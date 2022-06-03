package resilience

import (
	"context"
	"errors"
	"math"
	"time"
)

type BackOff interface {
	Next(i int) time.Duration
}

type Retry interface {
	Execute(ctx context.Context, req func() (interface{}, error)) (interface{}, error)
}

type RetryPredicateFunc = func(error) bool

type RetryOutcome int

const (
	RetrySuccess RetryOutcome = iota
	RetryFailedWithRetry
	RetryFailedWithoutRetry
)

func (o RetryOutcome) String() string {
	switch o {
	case RetrySuccess:
		return "successful"
	case RetryFailedWithRetry:
		return "failed-with-retry"
	case RetryFailedWithoutRetry:
		return "failed-without-retry"
	}
	return "unknown"
}

type RetryInstrumentation interface {
	RecordRetryCall(name string, attempts int, outcome RetryOutcome)
}

type RetryLogger interface {
	Warn(context.Context, ...interface{})
	Error(context.Context, ...interface{})
}

type RetryOptions struct {
	Name            string
	Instrumentation RetryInstrumentation
	Logger          RetryLogger
	MaxRetries      int
	BackOff         BackOff
	ErrorPredicate  RetryPredicateFunc
}

type metrifiedRetry struct {
	opts RetryOptions
}

func NewRetry(opts RetryOptions) Retry {
	return &metrifiedRetry{opts}
}

func (r *metrifiedRetry) Execute(ctx context.Context, req func() (interface{}, error)) (res interface{}, err error) {
	for i := 0; i <= r.opts.MaxRetries; i++ {
		if i > 0 {
			r.recordRetry(ctx, i)
			r.backOff(i)
		}

		if res, err = req(); err == nil {
			r.recordSuccess(ctx, i)
			return
		} else if !r.shouldRetry(err) {
			r.recordFailure(ctx, i, err)
			return
		}
	}
	r.recordExhausted(ctx, err)
	return
}

func (r *metrifiedRetry) backOff(i int) {
	if r.opts.BackOff != nil {
		<-time.After(r.opts.BackOff.Next(i))
	}
}

func (r *metrifiedRetry) shouldRetry(err error) bool {
	if r.opts.ErrorPredicate == nil {
		return !errors.Is(err, context.Canceled)
	} else {
		return r.opts.ErrorPredicate(err)
	}
}

func (r *metrifiedRetry) recordRetry(ctx context.Context, attempt int) {
	if r.opts.Logger != nil {
		r.opts.Logger.Warn(ctx, "Retrying request.", map[string]interface{}{"retry": r.opts.Name})
	}
}

func (r *metrifiedRetry) recordSuccess(ctx context.Context, attempt int) {
	if r.opts.Instrumentation != nil {
		r.opts.Instrumentation.RecordRetryCall(r.opts.Name, attempt+1, RetrySuccess)
	}
}

func (r *metrifiedRetry) recordFailure(ctx context.Context, attempt int, err error) {
	if r.opts.Instrumentation != nil {
		r.opts.Instrumentation.RecordRetryCall(r.opts.Name, attempt+1, RetryFailedWithoutRetry)
	}
	if r.opts.Logger != nil {
		r.opts.Logger.Error(ctx, "Request failed and will not be retried.",
			map[string]interface{}{"retry": r.opts.Name, "error": err})
	}
}

func (r *metrifiedRetry) recordExhausted(ctx context.Context, err error) {
	if r.opts.Logger != nil {
		r.opts.Logger.Error(ctx, "All retries failed.", map[string]interface{}{"retry": r.opts.Name, "error": err})
	}
	if r.opts.Instrumentation != nil {
		r.opts.Instrumentation.RecordRetryCall(r.opts.Name, r.opts.MaxRetries+1, RetryFailedWithRetry)
	}
}

type ConstantBackoff struct {
	t time.Duration
}

func NewConstantBackoff(t time.Duration) BackOff {
	return &ConstantBackoff{t}
}

func (b *ConstantBackoff) Next(i int) time.Duration {
	return b.t
}

type ExponentialBackoff struct {
	initial     time.Duration
	exponential time.Duration
}

func NewExponentialBackoff(initial time.Duration, exponential time.Duration) BackOff {
	return &ExponentialBackoff{initial, exponential}
}

func (b *ExponentialBackoff) Next(i int) time.Duration {
	t := b.initial
	if i > 1 {
		t += time.Duration(math.Pow(b.exponential.Seconds(), float64(i-1)))
	}
	return t
}
