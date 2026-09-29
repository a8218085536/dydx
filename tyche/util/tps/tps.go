package tps

import (
	"context"
	"sync"
	"time"
)

type ReleaseFunc func()

type Ctrl interface {
	Acquire() ReleaseFunc
	Close()
	Size() int
}

type tpsCtrl struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	queue  chan int
	res    chan ReleaseFunc
}

func New(size int) Ctrl {
	var ctx, cancel = context.WithCancel(context.Background())
	var tpsc = &tpsCtrl{
		cancel: cancel,
		queue:  make(chan int, size),
		res:    make(chan ReleaseFunc, size),
	}
	tpsc.run(ctx, size)
	return tpsc
}

func (tpsc *tpsCtrl) Acquire() ReleaseFunc {
	tpsc.queue <- 1
	return <-tpsc.res
}

func (tpsc *tpsCtrl) Close() {
	tpsc.cancel()
	tpsc.wg.Wait()
}

func (tpsc *tpsCtrl) Size() int {
	return cap(tpsc.queue)
}

func (tpsc *tpsCtrl) run(ctx context.Context, size int) {
	var cond = sync.NewCond(new(sync.Mutex))
	go func() {
		for {
			select {
			case <-ctx.Done():
				// quit
				defer close(tpsc.queue)
				return
			case <-time.After(time.Second):
				// reset per seconds
				cond.Broadcast()
			}
		}
	}()

	tpsc.wg.Add(1)
	go func() {
		defer tpsc.wg.Done()
		var sem = make(chan int, size)
		for range tpsc.queue {
			sem <- 1
			tpsc.res <- func() {
				cond.L.Lock()
				cond.Wait()
				cond.L.Unlock()
				<-sem
			}
		}
	}()
}
