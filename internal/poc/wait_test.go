package poc

import (
	"context"
	"sync"
	"testing"
	"time"
)

type manualTimer struct {
	deadline time.Time
	channel  chan time.Time
}

type manualClock struct {
	mu         sync.Mutex
	now        time.Time
	timers     []*manualTimer
	registered chan struct{}
	sleeps     []time.Duration
}

func newManualClock() *manualClock {
	return &manualClock{registered: make(chan struct{}, 16)}
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) Sleep(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.mu.Lock()
	c.sleeps = append(c.sleeps, duration)
	c.now = c.now.Add(duration)
	c.fireLocked()
	c.mu.Unlock()
	return nil
}

func (c *manualClock) After(duration time.Duration) <-chan time.Time {
	c.mu.Lock()
	timer := &manualTimer{deadline: c.now.Add(duration), channel: make(chan time.Time, 1)}
	c.timers = append(c.timers, timer)
	c.mu.Unlock()
	c.registered <- struct{}{}
	return timer.channel
}

func (c *manualClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.fireLocked()
	c.mu.Unlock()
}

func (c *manualClock) fireLocked() {
	remaining := c.timers[:0]
	for _, timer := range c.timers {
		if !timer.deadline.After(c.now) {
			timer.channel <- c.now
			close(timer.channel)
			continue
		}
		remaining = append(remaining, timer)
	}
	c.timers = remaining
}

func (c *manualClock) waitTimer(t *testing.T) {
	t.Helper()
	select {
	case <-c.registered:
	case <-time.After(time.Second):
		t.Fatal("manual timer was not registered")
	}
}

func TestWaitAllowsOne300SecondExtension(t *testing.T) {
	clock := newManualClock()
	controls := make(chan OperatorCommand, 2)
	ready := make(chan struct{})
	waiter := NewWaitController(clock, HumanWaitPolicy{Timeout: 300 * time.Second, Extension: 300 * time.Second, MaxExtensions: 1}, controls)
	done := make(chan WaitResult, 1)
	go func() { done <- waiter.Wait(context.Background(), WaitLogin, 0, ready) }()
	clock.waitTimer(t)
	clock.Advance(299 * time.Second)
	controls <- OperatorExtend
	clock.waitTimer(t)
	clock.Advance(299 * time.Second)
	select {
	case result := <-done:
		t.Fatalf("wait ended early: %s", result)
	default:
	}
	clock.Advance(time.Second)
	if got := <-done; got != WaitTimedOut {
		t.Fatalf("got=%s", got)
	}
}

func TestSecondExtensionIsRejected(t *testing.T) {
	clock := newManualClock()
	controls := make(chan OperatorCommand, 2)
	waiter := NewWaitController(clock, HumanWaitPolicy{Timeout: 300 * time.Second, Extension: 300 * time.Second, MaxExtensions: 1}, controls)
	done := make(chan WaitResult, 1)
	go func() { done <- waiter.Wait(context.Background(), WaitVerification, 2, nil) }()
	clock.waitTimer(t)
	controls <- OperatorExtend
	clock.waitTimer(t)
	controls <- OperatorExtend
	for {
		event := <-waiter.Events()
		if event.Kind == "extension_rejected" {
			if event.Reason != WaitVerification || event.WorkRank != 2 {
				t.Fatalf("unsafe or incorrect event=%+v", event)
			}
			break
		}
	}
	clock.Advance(300 * time.Second)
	if got := <-done; got != WaitTimedOut {
		t.Fatalf("got=%s", got)
	}
}

func TestReadySignalResolvesWaitWithoutRequest(t *testing.T) {
	clock := newManualClock()
	ready := make(chan struct{})
	waiter := NewWaitController(clock, HumanWaitPolicy{Timeout: 300 * time.Second}, nil)
	done := make(chan WaitResult, 1)
	go func() { done <- waiter.Wait(context.Background(), WaitTargetContext, 3, ready) }()
	clock.waitTimer(t)
	close(ready)
	if got := <-done; got != WaitResolved {
		t.Fatalf("got=%s", got)
	}
}

type scriptedCall struct {
	raw []byte
	err error
}

type scriptedPageAPI struct {
	calls  int
	script []scriptedCall
	bodies []any
}

func (a *scriptedPageAPI) Call(_ context.Context, _ string, body any) ([]byte, error) {
	a.bodies = append(a.bodies, body)
	call := a.script[a.calls]
	a.calls++
	return call.raw, call.err
}

func TestTransientRetryUsesTwoAndFiveSecondBackoff(t *testing.T) {
	api := &scriptedPageAPI{script: []scriptedCall{
		{err: NewCategorizedError(ErrorTransient)},
		{err: NewCategorizedError(ErrorTransient)},
		{raw: []byte(`{"data":{"objectList":[],"lastBuffer":""}}`)},
	}}
	clock := newManualClock()
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "retry-job"), clock)
	if _, _, err := collector.call(context.Background(), "finderSearch", map[string]any{"keyword": "fixture-keyword"}); err != nil {
		t.Fatal(err)
	}
	if api.calls != 3 || len(clock.sleeps) != 2 || clock.sleeps[0] != 2*time.Second || clock.sleeps[1] != 5*time.Second {
		t.Fatalf("calls=%d sleeps=%v", api.calls, clock.sleeps)
	}
}

func TestRateLimitAndAccessDeniedDoNotRetry(t *testing.T) {
	for _, category := range []ErrorCategory{ErrorRateLimited, ErrorAccessDenied, ErrorMethodMissing, ErrorSafety, ErrorStructure} {
		t.Run(string(category), func(t *testing.T) {
			api := &scriptedPageAPI{script: []scriptedCall{{err: NewCategorizedError(category)}}}
			collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "no-retry-"+string(category)), newManualClock())
			if _, _, err := collector.call(context.Background(), "finderSearch", nil); err == nil || ClassifyError(err) != category {
				t.Fatalf("err=%v", err)
			}
			if api.calls != 1 {
				t.Fatalf("calls=%d", api.calls)
			}
		})
	}
}

func TestTargetContextWaitRetriesReadOnlyRequestOnce(t *testing.T) {
	api := &scriptedPageAPI{script: []scriptedCall{
		{err: NewCategorizedError(ErrorTargetContext)},
		{raw: []byte(`{"data":{"objectList":[],"lastBuffer":""}}`)},
	}}
	clock := newManualClock()
	waiter := NewWaitController(clock, HumanWaitPolicy{Timeout: 300 * time.Second}, nil)
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "context-retry-job"), clock)
	collector.ConfigureHumanWait(waiter, func(reason WaitReason, rank int) <-chan struct{} {
		if reason != WaitTargetContext || rank != 0 {
			t.Fatalf("reason=%s rank=%d", reason, rank)
		}
		ready := make(chan struct{})
		close(ready)
		return ready
	})
	if _, _, err := collector.call(context.Background(), "finderSearch", nil); err != nil {
		t.Fatal(err)
	}
	if api.calls != 2 {
		t.Fatalf("calls=%d", api.calls)
	}
}
