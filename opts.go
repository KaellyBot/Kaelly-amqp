package amqp

import "time"

type config struct {
	bindings            []Binding
	retryDelay          time.Duration
	isConnectedCallback func(bool)
}

type Option func(*config)

// WithRetryDelay configures queue bindings to create
// Empty routingKey is set to unique queue name by default.
func WithBindings(bindings ...Binding) Option {
	return func(cfg *config) {
		cfg.bindings = append(cfg.bindings, bindings...)
	}
}

// WithRetryDelay configures the retry delay.
func WithRetryDelay(d time.Duration) Option {
	return func(cfg *config) {
		cfg.retryDelay = d
	}
}

// WithIsConnectedCallback sets the callback for connection status.
func WithIsConnectedCallback(callback func(bool)) Option {
	return func(cfg *config) {
		cfg.isConnectedCallback = callback
	}
}
