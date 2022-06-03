package resilience

import (
	"context"
	"errors"
	"time"
)

type TimeoutFunc = func(ctx context.Context) (interface{}, error)

type Timeout interface {
	Execute(ctx context.Context, req TimeoutFunc) (interface{}, error)
}

type TimeoutOutcome int

const (
	TimeoutSuccess TimeoutOutcome = iota
	TimeoutFailed
	TimeoutTimedOut
)

func (o TimeoutOutcome) String() string {
	switch o {
	case TimeoutSuccess:
		return "successful"
	case TimeoutFailed:
		return "failed"
	case TimeoutTimedOut:
		return "timed-out"
	}
	return "unknown"
}

type TimeoutInstrumentation interface {
	RecordTimeoutCall(name string, outcome TimeoutOutcome)
}

type TimeoutLogger interface {
	Error(context.Context, ...interface{})
}

type TimeoutOptions struct {
	Name            string
	Instrumentation TimeoutInstrumentation
	Logger          TimeoutLogger
	TimeLimit       time.Duration
}

type metrifiedTimeout struct {
	opts TimeoutOptions
}

func NewTimeout(opts TimeoutOptions) Timeout {
	return &metrifiedTimeout{opts}
}

func (t *metrifiedTimeout) Execute(ctx context.Context, req TimeoutFunc) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, t.opts.TimeLimit)
	defer cancel()

	r, err := req(ctx)
	if err == nil {
		t.recordSuccess()
	} else if errors.Is(err, context.DeadlineExceeded) {
		t.recordTimeout(ctx)
	} else {
		t.recordFailure(ctx, err)
	}

	return r, err
}

func (t *metrifiedTimeout) recordTimeout(ctx context.Context) {
	if t.opts.Logger != nil {
		t.opts.Logger.Error(ctx, "Request timed out.", map[string]interface{}{"timeout": t.opts.Name})
	}
	if t.opts.Instrumentation != nil {
		t.opts.Instrumentation.RecordTimeoutCall(t.opts.Name, TimeoutTimedOut)
	}
}

func (t *metrifiedTimeout) recordFailure(ctx context.Context, err error) {
	if t.opts.Logger != nil {
		t.opts.Logger.Error(ctx, "Timed request failed for non-timeout reasons.",
			map[string]interface{}{"timeout": t.opts.Name, "error": err})
	}
	if t.opts.Instrumentation != nil {
		t.opts.Instrumentation.RecordTimeoutCall(t.opts.Name, TimeoutFailed)
	}
}

func (t *metrifiedTimeout) recordSuccess() {
	if t.opts.Instrumentation != nil {
		t.opts.Instrumentation.RecordTimeoutCall(t.opts.Name, TimeoutSuccess)
	}
}
