package poc

import (
	"context"
	"errors"
	"strings"
	"time"
)

type WaitReason string

const (
	WaitLogin         WaitReason = "login"
	WaitVerification  WaitReason = "verification"
	WaitTargetContext WaitReason = "target_context"
)

type OperatorCommand string

const (
	OperatorExtend OperatorCommand = "extend"
	OperatorCancel OperatorCommand = "cancel"
)

type WaitResult string

const (
	WaitResolved  WaitResult = "resolved"
	WaitTimedOut  WaitResult = "timed_out"
	WaitCancelled WaitResult = "cancelled"
)

type WaitEvent struct {
	Kind     string
	Reason   WaitReason
	WorkRank int
}

type WaitController struct {
	clock    Clock
	policy   HumanWaitPolicy
	commands <-chan OperatorCommand
	events   chan WaitEvent
}

func NewWaitController(clock Clock, policy HumanWaitPolicy, commands <-chan OperatorCommand) *WaitController {
	return &WaitController{clock: clock, policy: policy, commands: commands, events: make(chan WaitEvent, 8)}
}

func (w *WaitController) Events() <-chan WaitEvent {
	if w == nil {
		return nil
	}
	return w.events
}

func (w *WaitController) Wait(ctx context.Context, reason WaitReason, workRank int, ready <-chan struct{}) WaitResult {
	if w == nil || w.clock == nil || w.policy.Timeout <= 0 {
		return WaitTimedOut
	}
	extensions := 0
	deadline := clockAfter(w.clock, w.policy.Timeout)
	commands := w.commands
	for {
		select {
		case <-ctx.Done():
			w.emit(WaitEvent{Kind: "cancelled", Reason: reason, WorkRank: workRank})
			return WaitCancelled
		case <-ready:
			w.emit(WaitEvent{Kind: "resolved", Reason: reason, WorkRank: workRank})
			return WaitResolved
		case command, open := <-commands:
			if !open {
				commands = nil
				continue
			}
			switch command {
			case OperatorCancel:
				w.emit(WaitEvent{Kind: "cancelled", Reason: reason, WorkRank: workRank})
				return WaitCancelled
			case OperatorExtend:
				if extensions >= w.policy.MaxExtensions || w.policy.Extension <= 0 {
					w.emit(WaitEvent{Kind: "extension_rejected", Reason: reason, WorkRank: workRank})
					continue
				}
				extensions++
				deadline = clockAfter(w.clock, w.policy.Extension)
				w.emit(WaitEvent{Kind: "extension_accepted", Reason: reason, WorkRank: workRank})
			}
		case <-deadline:
			w.emit(WaitEvent{Kind: "timed_out", Reason: reason, WorkRank: workRank})
			return WaitTimedOut
		}
	}
}

func (w *WaitController) emit(event WaitEvent) {
	select {
	case w.events <- event:
	default:
	}
}

type afterClock interface {
	After(time.Duration) <-chan time.Time
}

func clockAfter(clock Clock, duration time.Duration) <-chan time.Time {
	if provider, ok := clock.(afterClock); ok {
		return provider.After(duration)
	}
	return time.After(duration)
}

type ErrorCategory string

const (
	ErrorTransient     ErrorCategory = "transient_network"
	ErrorRateLimited   ErrorCategory = "rate_limited"
	ErrorAccessDenied  ErrorCategory = "access_denied"
	ErrorTargetContext ErrorCategory = "target_context"
	ErrorMethodMissing ErrorCategory = "method_missing"
	ErrorSafety        ErrorCategory = "safety_failure"
	ErrorStructure     ErrorCategory = "structure_failure"
	ErrorUnknown       ErrorCategory = "unknown"
)

type CategorizedError struct {
	Category ErrorCategory
}

func (e CategorizedError) Error() string { return string(e.Category) }

func NewCategorizedError(category ErrorCategory) error {
	return CategorizedError{Category: category}
}

func ClassifyError(err error) ErrorCategory {
	if err == nil {
		return ErrorUnknown
	}
	var categorized CategorizedError
	if errors.As(err, &categorized) {
		return categorized.Category
	}
	message := err.Error()
	if strings.Contains(message, "-70003") || strings.Contains(message, "JSAPI_JSONPARSE_FAILED") {
		return ErrorTargetContext
	}
	return ErrorUnknown
}

type RetryPolicy struct {
	MaxRetries int
	Backoff    []time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxRetries: 2, Backoff: []time.Duration{2 * time.Second, 5 * time.Second}}
}

type HumanWaitError struct {
	Result WaitResult
}

func (e HumanWaitError) Error() string { return "human wait " + string(e.Result) }
