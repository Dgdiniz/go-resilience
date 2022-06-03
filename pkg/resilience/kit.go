package resilience

import "sync"

type ResilienceKit interface {
	Retry() Retry
	CircuitBreaker() CircuitBreaker
	Timeout() Timeout
}

type ResilienceKitOptions struct {
	Retry          RetryOptions
	CircuitBreaker CircuitBreakerOptions
	Timeout        TimeoutOptions
}

type resilienceKit struct {
	opts ResilienceKitOptions

	// Retry
	retry     Retry
	lazyRetry sync.Once

	// Circuit breaker
	cb     CircuitBreaker
	lazyCb sync.Once

	// Timeout
	timeout     Timeout
	lazyTimeout sync.Once
}

func NewResilienceKit(opts ResilienceKitOptions) ResilienceKit {
	kit := &resilienceKit{}
	kit.opts = opts
	return kit
}

func (p *resilienceKit) Retry() Retry {
	p.lazyRetry.Do(func() {
		p.retry = NewRetry(p.opts.Retry)
	})
	return p.retry
}

func (p *resilienceKit) CircuitBreaker() CircuitBreaker {
	p.lazyCb.Do(func() {
		p.cb = NewCircuitBreaker(p.opts.CircuitBreaker)
	})
	return p.cb
}

func (p *resilienceKit) Timeout() Timeout {
	p.lazyTimeout.Do(func() {
		p.timeout = NewTimeout(p.opts.Timeout)
	})
	return p.timeout
}
