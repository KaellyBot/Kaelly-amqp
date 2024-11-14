package amqp

import (
	"github.com/rs/zerolog/log"
)

type Mock struct {
	RunFunc  func() error
	EmitFunc func(msg *RabbitMQMessage, exchange Exchange, routingKey,
		correlationID string) error
	RequestFunc func(msg *RabbitMQMessage, exchange Exchange, routingKey,
		correlationID, replyTo string) error
	ReplyFunc       func(msg *RabbitMQMessage, correlationID, replyTo string) error
	ConsumeFunc     func(queueName, routingKey string, consumer MessageConsumer)
	IsConnectedFunc func() bool
	ShutdownFunc    func()
}

func (mock *Mock) Run() error {
	if mock.RunFunc != nil {
		return mock.RunFunc()
	}

	log.Warn().Msgf("No mock provided for Run function")
	return nil
}

func (mock *Mock) Emit(msg *RabbitMQMessage, exchange Exchange, routingKey,
	correlationID string) error {
	if mock.EmitFunc != nil {
		return mock.EmitFunc(msg, exchange, routingKey, correlationID)
	}

	log.Warn().Msgf("No mock provided for Emit function")
	return nil
}

func (mock *Mock) Request(msg *RabbitMQMessage, exchange Exchange, routingKey,
	correlationID, replyTo string) error {
	if mock.RequestFunc != nil {
		return mock.RequestFunc(msg, exchange, routingKey, correlationID, replyTo)
	}

	log.Warn().Msgf("No mock provided for Request function")
	return nil
}

func (mock *Mock) Reply(msg *RabbitMQMessage, correlationID, replyTo string) error {
	if mock.ReplyFunc != nil {
		return mock.ReplyFunc(msg, correlationID, replyTo)
	}

	log.Warn().Msgf("No mock provided for Reply function")
	return nil
}

func (mock *Mock) Consume(queueName, routingKey string, consumer MessageConsumer) {
	if mock.ConsumeFunc != nil {
		mock.ConsumeFunc(queueName, routingKey, consumer)
		return
	}

	log.Warn().Msgf("No mock provided for Consume function")
}

func (mock *Mock) IsConnected() bool {
	if mock.IsConnectedFunc != nil {
		return mock.IsConnectedFunc()
	}

	log.Warn().Msgf("No mock provided for IsConnected function")
	return false
}

func (mock *Mock) Shutdown() {
	if mock.ShutdownFunc != nil {
		mock.ShutdownFunc()
		return
	}

	log.Warn().Msgf("No mock provided for Shutdown function")
}
