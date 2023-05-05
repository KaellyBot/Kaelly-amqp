package amqp

import (
	"context"
	"errors"
	"sync"

	"github.com/rabbitmq/amqp091-go"
)

const (
	ContextCorrelationID ContextKey = "correlationID"

	ExchangeRequest Exchange = "requests"
	ExchangeAnswer  Exchange = "answers"
	ExchangeNews    Exchange = "news"
)

var (
	ErrCannotBeConnected = errors.New("cannot be connected, please check AMQP server or address")
	ErrMustBeConnected   = errors.New("this function requires to be connected to AMQP server")
)

type ContextKey string

type Exchange string

type MessageConsumer func(ctx context.Context, message *RabbitMQMessage, correlationID string)

type MessageBroker interface {
	Publish(msg *RabbitMQMessage, exchange Exchange, routingKey, correlationID string) error
	Consume(queueName string, consumer MessageConsumer) error
	IsConnected() bool
	Shutdown()
}

type Impl struct {
	connection       *amqp091.Connection
	publisherChannel *amqp091.Channel
	consumerChannel  *amqp091.Channel
	mutex            *sync.Mutex
	clientID         string
	address          string
	bindings         []Binding
	isConnected      bool
}

type Binding struct {
	Exchange   Exchange
	RoutingKey string
	Queue      string
}
