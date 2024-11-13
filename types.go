package amqp

import (
	"errors"
	"sync"

	"github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeRequest Exchange = "requests"
	ExchangeNews    Exchange = "news"
)

var (
	ErrCannotBeConnected = errors.New("cannot be connected, please check AMQP server or address")
	ErrMustBeConnected   = errors.New("this function requires to be connected to AMQP server")
)

type Exchange string

type MessageConsumer func(ctx Context, message *RabbitMQMessage)

type MessageBroker interface {
	Run(bindings []Binding) error
	Emit(msg *RabbitMQMessage, exchange Exchange, routingKey, correlationID string) error
	Request(msg *RabbitMQMessage, exchange Exchange, routingKey, correlationID, replyTo string) error
	Reply(msg *RabbitMQMessage, correlationID, replyTo string) error
	Consume(queueName string, consumer MessageConsumer) error
	GetIdentifiedQueue(queue string) string
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
