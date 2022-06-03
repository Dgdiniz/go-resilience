package resilience

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
)

type CircuitBreaker interface {
	Execute(ctx context.Context, req func() (interface{}, error)) (interface{}, error)
}

type CircuitBreakerInstrumentation interface {
	RegisterCircuitBreakerStateGauge(name string, supplier func() string)
	RecordCircuitBreakerCall(name string, err error)
}

type CircuitBreakerLogger interface {
	Info(context.Context, ...interface{})
	CircuitBreakerOpen(context.Context, ...interface{})
}

type CircuitBreakerOptions struct {
	Instrumentation      CircuitBreakerInstrumentation
	Logger               CircuitBreakerLogger
	Name                 string
	FailureRateThreshold float64
	WaitOpen             time.Duration
}

type metrifiedCircuitBreaker struct {
	opts CircuitBreakerOptions
	cb   *gobreaker.CircuitBreaker
}

func NewCircuitBreaker(opts CircuitBreakerOptions) CircuitBreaker {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:     opts.Name,
		Timeout:  opts.WaitOpen,
		Interval: 1 * time.Minute,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			total := float64(counts.TotalSuccesses + counts.TotalFailures)
			failureRate := float64(counts.TotalFailures) / total
			return failureRate >= opts.FailureRateThreshold
		},
		OnStateChange: logCircuitBreakerStateTransition(opts.Logger),
	})

	if opts.Instrumentation != nil {
		opts.Instrumentation.RegisterCircuitBreakerStateGauge(opts.Name, func() string {
			return cb.State().String()
		})
	}

	return &metrifiedCircuitBreaker{opts, cb}
}

func (cb *metrifiedCircuitBreaker) Execute(ctx context.Context, req func() (interface{}, error)) (interface{}, error) {
	res, err := cb.cb.Execute(req)
	if cb.opts.Instrumentation != nil {
		cb.opts.Instrumentation.RecordCircuitBreakerCall(cb.opts.Name, err)
	}
	return res, err
}

func logCircuitBreakerStateTransition(logger CircuitBreakerLogger) func(name string, from gobreaker.State, to gobreaker.State) {
	if logger == nil {
		return func(string, gobreaker.State, gobreaker.State) {}
	}

	return func(name string, from gobreaker.State, to gobreaker.State) {
		ctx := context.TODO()

		logger.Info(ctx, "Circuit breaker state transition", map[string]interface{}{
			"circuit_breaker": name,
			"from_state":      from.String(),
			"to_state":        to.String(),
		})

		if from == gobreaker.StateClosed && to == gobreaker.StateOpen {
			logger.CircuitBreakerOpen(ctx, "Circuit breaker is open.",
				map[string]interface{}{"circuit_breaker": name})
		} else if to == gobreaker.StateClosed {
			logger.Info(ctx, "Circuit breaker is closed.", map[string]interface{}{"circuit_breaker": name})
		}
	}
}
