package amqp

import (
	"context"
	"errors"
	"sync"

	"github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeAnswer Exchange = "answers"
	ExchangeNews            = "news"

	ContextCorrelationId = "correlationId"
)

var (
	ErrCannotBeConnected = errors.New("Cannot be connected, please check AMQP server or address")
	ErrMustBeConnected   = errors.New("This function requires to be connected to AMQP server")
)

type Exchange string

type MessageConsumer func(ctx context.Context, message *RabbitMQMessage, correlationId string)

type MessageBrokerInterface interface {
	Publish(msg *RabbitMQMessage, exchange Exchange, routingKey, correlationId string) error
	Consume(queueName, routingKey string, consumer MessageConsumer) error
	IsConnected() bool
	Shutdown()
}

type MessageBroker struct {
	connection       *amqp091.Connection
	publisherChannel *amqp091.Channel
	consumerChannel  *amqp091.Channel
	mutex            *sync.Mutex
	clientId         string
	address          string
	bindings         []Binding
	isConnected      bool
}

type Binding struct {
	RoutingKey string
	Exchange   string
	Queue      string
}
