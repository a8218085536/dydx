package tyche

import (
	"container/heap"
	"context"
	"ethgo/model/orders"
	"ethgo/util/timer"
	"sort"
	"sync"
	"time"
)

type AfterFunc func(context.Context, *MessageDispatcher)
type HandlerFunc func(context.Context, *orders.Message) AfterFunc

type MessageDispatcher struct {
	numberOfConcurrent int64
	reader             *orders.MessageReader
	handler            HandlerFunc
	receiver           chan chan *orders.Message
	queue              chan *orders.Message
}

var shareTimer = timer.New(time.Second, 60*60)

func After(timeout int64, message *orders.Message) AfterFunc {
	return func(ctx context.Context, t *MessageDispatcher) {
		select {
		case <-ctx.Done():
			return
		case <-shareTimer.After(time.Duration(timeout) * time.Second):
		}

		select {
		case <-ctx.Done():
			return
		case t.queue <- message:
		}
	}
}

func NewMessageDispatcher(reader *orders.MessageReader, handler HandlerFunc, numberOfConcurrent int64) *MessageDispatcher {
	return &MessageDispatcher{
		reader:             reader,
		handler:            handler,
		numberOfConcurrent: numberOfConcurrent,
		queue:              make(chan *orders.Message),
		receiver:           make(chan chan *orders.Message),
	}
}

func (t *MessageDispatcher) Run(ctx context.Context) {
	go func() {
		for {
			// 清理 15 天前处理过的消息
			t.reader.Trim(15 * 24 * 60 * 60)

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour):
			}
		}
	}()

	go func() {
		// 优先处理尚未处理完毕的消息
		t.handleNoAck()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				// 接收新消息
				t.handlePending()
			}
		}
	}()

	go func() {
		defer close(t.queue)
		defer close(t.receiver)
		<-ctx.Done()
	}()

	t.run(ctx)
}

func (t *MessageDispatcher) dispatchMessage(res []*orders.Message) {
	defer func() {
		if err := recover(); err != nil {
			// panic: this queue is closed
			_ = err
		}
	}()
	sort.Slice(res, func(i, j int) bool {
		return res[i].Nonce() < res[j].Nonce() // 调用方法获取 Nonce 值进行比较
	})
	for _, msg := range res {
		t.queue <- msg
	}
}

func (t *MessageDispatcher) handleNoAck() {
	res, err := t.reader.Read(orders.WithNoLimit(), orders.WithNoBlock(), orders.WithPending())
	if err != nil {
		panic(err)
	}

	t.dispatchMessage(res)
}

func (t *MessageDispatcher) handlePending() {
	res, err := t.reader.Read(orders.WithLimit(128), orders.WithBlock(0))
	if err != nil {
		log.Errorf("侦听 %v 失败: %v", t.reader.StreamName(), err)
		time.Sleep(time.Second)
	}

	t.dispatchMessage(res)
}

func (t *MessageDispatcher) work(ctx context.Context) {
	wg := sync.WaitGroup{}
	for i := 0; i < int(t.numberOfConcurrent); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			c := make(chan *orders.Message)
			t.receiver <- c
			for msg := range c {
				after := t.handler(ctx, msg)
				if after != nil {
					after(ctx, t)
				}

				select {
				case <-ctx.Done():
					return
				case t.receiver <- c:
				}
			}

		}()
	}
	wg.Wait()
}

func (t *MessageDispatcher) run(ctx context.Context) {
	go t.work(ctx)

	workers := make([]chan *orders.Message, 0)
	messageQueue := make(messageHeap, 0)
	dispatchTicker := time.NewTicker(50 * time.Millisecond)
	defer dispatchTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-t.queue:
			if ok {
				heap.Push(&messageQueue, msg)
			}
		case c, ok := <-t.receiver:
			if ok {
				workers = append(workers, c)
			}
		case <-dispatchTicker.C:
			n := len(workers)
			if n > messageQueue.Len() {
				n = messageQueue.Len()
			}
			if n > 0 {
				var wakeup []chan *orders.Message
				for i := 0; i < n; i++ {
					wakeup = append(wakeup, workers[i])
				}
				workers = workers[n:]

				for _, c := range wakeup {
					c <- heap.Pop(&messageQueue).(*orders.Message)
				}
			}
		}
	}
}
