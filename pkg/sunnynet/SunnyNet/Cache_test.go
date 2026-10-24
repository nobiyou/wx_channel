package SunnyNet

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 并发调用同一个 key 时，fn 只应执行一次，且所有调用者都拿到同一个非 nil 结果。
func TestGroupDo_ConcurrentWaitersGetResult(t *testing.T) {
	g := &Group{}
	var calls int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := g.Do("k", func() (interface{}, error) {
				atomic.AddInt32(&calls, 1)
				time.Sleep(5 * time.Millisecond)
				return "cert", nil
			})
			if err != nil || v != "cert" {
				t.Errorf("got v=%v err=%v, want cert", v, err)
			}
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1", calls)
	}
}
