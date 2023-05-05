package amqp

import (
	"github.com/rs/zerolog/log"
)

type Mock struct {
	PublishFunc     func(msg *RabbitMQMessage, exchange Exchange, routingKey, correlationID string) error
	ConsumeFunc     func(queueName, routingKey string, consumer MessageConsumer) error
	IsConnectedFunc func() bool
	ShutdownFunc    func()
}

func (mock *Mock) Publish(msg *RabbitMQMessage, exchange Exchange, routingKey, correlationID string) error {
	if mock.PublishFunc != nil {
		return mock.PublishFunc(msg, exchange, routingKey, correlationID)
	}

	log.Warn().Msgf("No mock provided for Publish function")
	return nil
}

func (mock *Mock) Consume(queueName, routingKey string, consumer MessageConsumer) error {
	if mock.ConsumeFunc != nil {
		return mock.ConsumeFunc(queueName, routingKey, consumer)
	}

	log.Warn().Msgf("No mock provided for Consume function")
	return nil
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
