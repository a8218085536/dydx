package dydxtrading

import (
	"context"
	"log"
	"time"
)

type IndexerConsumerPoller interface {
	PollOnce(context.Context) error
}

func RunIndexerConsumerPollLoop(ctx context.Context, worker IndexerConsumerPoller, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	for {
		if err := worker.PollOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("dYdX indexer consumer poll failed: %v", err)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
