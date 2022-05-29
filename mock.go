package amqp

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

type MessageBrokerMock struct {
	PublishFunc     func(msg *RabbitMQMessage, topic, routingKey, correlationId string) error
	ConsumeFunc     func(queueName, routingKey string) (<-chan amqp.Delivery, error)
	IsConnectedFunc func() bool
	ShutdownFunc    func()
}

func (mock *MessageBrokerMock) Publish(msg *RabbitMQMessage, topic, routingKey, correlationId string) error {
	if mock.PublishFunc != nil {
		return mock.PublishFunc(msg, topic, routingKey, correlationId)
	}

	log.Warn().Msgf("No mock provided for Publish function")
	return nil
}

func (mock *MessageBrokerMock) Consume(queueName, routingKey string) (<-chan amqp.Delivery, error) {
	if mock.ConsumeFunc != nil {
		return mock.ConsumeFunc(queueName, routingKey)
	}

	log.Warn().Msgf("No mock provided for Consume function")
	return nil, nil
}

func (mock *MessageBrokerMock) IsConnected() bool {
	if mock.IsConnectedFunc != nil {
		return mock.IsConnectedFunc()
	}

	log.Warn().Msgf("No mock provided for IsConnected function")
	return false
}

func (mock *MessageBrokerMock) Shutdown() {
	if mock.ShutdownFunc != nil {
		mock.ShutdownFunc()
		return
	}

	log.Warn().Msgf("No mock provided for Shutdown function")
}
