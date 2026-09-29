package tyche

import "context"

const (
	DYDXStatusSucceed = "succ"
	DYDXStatusFailed  = "fail"
)

type DYDXBroadcaster interface {
	Broadcast(context.Context, DYDXIntent) (string, error)
}

type DYDXBroadcasterFunc func(context.Context, DYDXIntent) (string, error)

func (f DYDXBroadcasterFunc) Broadcast(ctx context.Context, intent DYDXIntent) (string, error) {
	return f(ctx, intent)
}

type DYDXStatusStore interface {
	SetStatus(id, status, txHash, reason string) error
}

type DYDXStatusStoreFunc func(id, status, txHash, reason string) error

func (f DYDXStatusStoreFunc) SetStatus(id, status, txHash, reason string) error {
	return f(id, status, txHash, reason)
}

type DYDXProcessor struct {
	broadcaster DYDXBroadcaster
	statusStore DYDXStatusStore
}

func NewDYDXProcessor(broadcaster DYDXBroadcaster, statusStore DYDXStatusStore) *DYDXProcessor {
	return &DYDXProcessor{
		broadcaster: broadcaster,
		statusStore: statusStore,
	}
}

func (p *DYDXProcessor) Process(ctx context.Context, intent DYDXIntent) error {
	txHash, err := p.broadcaster.Broadcast(ctx, intent)
	if err != nil {
		return p.statusStore.SetStatus(intent.ID, DYDXStatusFailed, "", err.Error())
	}
	return p.statusStore.SetStatus(intent.ID, DYDXStatusSucceed, txHash, "")
}
