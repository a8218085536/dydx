package tps

import (
	"sync"
	"testing"
	"time"
)

func TestCTRL(t *testing.T) {
	var tpsc = New(2)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(id int) {
			var release = tpsc.Acquire()
			defer release()

			t.Logf("  -> Start: %v", id)
			time.Sleep(3 * time.Second)
			t.Logf("  -> Finish: %v", id)

			wg.Done()
		}(i)

	}

	t.Log("====== BEGIN ======")
	wg.Wait()
	t.Log("====== END ======")

	tpsc.Close()
	t.Log("====== CLOSED ======")
}
