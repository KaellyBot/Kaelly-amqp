package amqp

import (
	"errors"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeRequest Exchange = "requests"
	ExchangeNews    Exchange = "news"

	defaultRetryDelay = 5 * time.Second
)

var (
	ErrAlreadyStarted        = errors.New("client is already started. Aborting new attempt")
	ErrCannotBeConnected     = errors.New("cannot be connected, please check AMQP server or address")
	ErrMustBeConnected       = errors.New("this function requires to be connected to AMQP server")
	ErrCommunicationIsBroken = errors.New("action cannot continue because communication is broken")
	ErrClientNotInitialized  = errors.New("client is not ready yet")
	ErrActionCanceledByUser  = errors.New("action has been canceled by user by a shutdown")
)

type MessageConsumer func(ctx Context, message *RabbitMQMessage)

type MessageBroker interface {
	Run() error
	Emit(msg *RabbitMQMessage, exchange Exchange, routingKey, correlationID string) error
	Request(msg *RabbitMQMessage, exchange Exchange, routingKey, correlationID, replyTo string) error
	Reply(msg *RabbitMQMessage, correlationID, replyTo string) error
	Consume(queueName string, consumer MessageConsumer)
	IsConnected() bool
	Shutdown()
}

type Exchange string

type Binding struct {
	Exchange   Exchange
	RoutingKey string
	Queue      string
}

type messageBroker struct {
	clientID            string
	address             string
	cfg                 config
	isConnected         bool
	shouldBeConnected   bool
	connection          *amqp091.Connection
	publisherChannel    *amqp091.Channel
	consumerChannel     *amqp091.Channel
	notifyConClose      chan *amqp091.Error
	notifyPubChanClose  chan *amqp091.Error
	notifySubChanClose  chan *amqp091.Error
	eventsChannels      map[string](chan connectionEvent)
	mainEventsChan      chan connectionEvent
	eventsChannelsMutex *sync.Mutex
	connectionMutex     *sync.Mutex
	callbackMutex       *sync.Mutex
}

type connectionEvent int

const (
	shutdown  connectionEvent = 1
	issue     connectionEvent = 2
	connected connectionEvent = 3
)

type interruptionAction int

const (
	reconnect interruptionAction = 1
	exit      interruptionAction = 2
)
