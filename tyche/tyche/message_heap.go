package tyche

import (
	"ethgo/model/orders"
)

// A messageQueue implements heap.Interface and holds orders.Messages.
type messageHeap []*orders.Message

func (mq messageHeap) Len() int { return len(mq) }

func (mq messageHeap) Less(i, j int) bool {
	a := mq[i].Nonce()
	b := mq[j].Nonce()
	if a == b {
		return mq[i].CreatedAt() > mq[j].CreatedAt()
	}
	return a < b
}

func (mq messageHeap) Swap(i, j int) {
	mq[i], mq[j] = mq[j], mq[i]
}

func (mq *messageHeap) Push(x interface{}) {
	*mq = append(*mq, x.(*orders.Message))
}

func (mq *messageHeap) Pop() interface{} {
	old := *mq
	n := len(old)
	msg := old[n-1]
	old[n-1] = nil // avoid memory leak
	*mq = old[0 : n-1]
	return msg
}
