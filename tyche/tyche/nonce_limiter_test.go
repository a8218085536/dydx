package tyche

import (
	"testing"
)

func TestNonceLimiter(t *testing.T) {
	queue := NewNonceLimiter(12)
	t.Log(queue.Distance(1))
	t.Log(queue.Distance(23))
	queue.Update(23)
	t.Log(queue.Distance(23))
}
