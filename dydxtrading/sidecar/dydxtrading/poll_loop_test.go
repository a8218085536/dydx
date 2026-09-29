package dydxtrading

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunIndexerConsumerPollLoopStopsOnContextCancel(t *testing.T) {
	worker := &recordingPollWorker{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		RunIndexerConsumerPollLoop(ctx, worker, time.Millisecond)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return worker.calls() > 0
	}, time.Second, time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("poll loop did not stop after context cancellation")
	}
}

type recordingPollWorker struct {
	count int
}

func (w *recordingPollWorker) PollOnce(context.Context) error {
	w.count++
	return nil
}

func (w *recordingPollWorker) calls() int {
	return w.count
}
