package amqp

import (
	"errors"
	"sync"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
)

const (
	logTopic      = "amqpTopic"
	logRoutingKey = "amqpRoutingKey"
	logQueue      = "amqpQueue"
	logProto      = "amqpProto"
	logContent    = "amqpContent"
	logPanic      = "amqpPanic"
)

var (
	ErrCannotBeConnected = errors.New("Cannot be connected, please check AMQP server or address")
	ErrMustBeConnected   = errors.New("This function requires to be connected to AMQP server")
)

type MessageConsumer func(message *RabbitMQMessage, correlationId string)

type MessageBrokerInterface interface {
	Publish(msg *RabbitMQMessage, topic, routingKey, correlationId string) error
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
	Topic      string
	Queue      string
}

/*
	- clientId: ID used to be identified as a publisher/consumer
	- address: string with the following format: `amqp://username:password@hostname:port/vhost`
	- topics: topics used to publish and consume messages
	- bindings: topic, routingKey and queue used to consume messages
*/
func New(clientId, address string, bindings []Binding) (*MessageBroker, error) {
	broker := MessageBroker{
		clientId:    clientId,
		address:     address,
		bindings:    bindings,
		isConnected: false,
		mutex:       &sync.Mutex{},
	}

	return &broker, broker.dial()
}

func (broker *MessageBroker) dial() error {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()

	var err error
	if broker.connection, err = amqp091.Dial(broker.address); err != nil {
		log.Error().Err(err).Msgf("Failed to connect to AMQP server")
		return ErrCannotBeConnected
	}

	if broker.publisherChannel, err = broker.connection.Channel(); err != nil {
		log.Error().Err(err).Msgf("Failed to retrieve channel from AMQP server connection")
		broker.Shutdown()
		return ErrCannotBeConnected
	}

	if broker.consumerChannel, err = broker.connection.Channel(); err != nil {
		log.Error().Err(err).Msgf("Failed to retrieve channel from AMQP server connection")
		broker.Shutdown()
		return ErrCannotBeConnected
	}

	if err = broker.declareBindings(); err != nil {
		log.Error().Err(err).Msgf("Failed to declare topics, queues and bindings")
		broker.Shutdown()
		return ErrCannotBeConnected
	}

	broker.isConnected = true

	go broker.handleAMQPConnection()
	return nil
}

func (broker *MessageBroker) declareBindings() error {

	for _, binding := range broker.bindings {
		uniqueQueue := broker.getIdentifiedQueue(binding.Queue)
		_, err := broker.consumerChannel.QueueDeclare(
			uniqueQueue, // name
			true,        // durable
			false,       // delete when unused
			false,       // exclusive
			false,       // no-wait
			nil,         // arguments
		)
		if err != nil {
			log.Error().Err(err).Str(logQueue, uniqueQueue).Msgf("Failed to declare queue")
			return ErrCannotBeConnected
		}

		err = broker.consumerChannel.QueueBind(
			uniqueQueue,        // queue name
			binding.RoutingKey, // routing key
			binding.Topic,      // exchange
			false,
			nil)
		if err != nil {
			log.Error().Err(err).Str(logTopic, binding.Topic).Str(logQueue, uniqueQueue).Str(logRoutingKey, binding.RoutingKey).Msg("Failed to bind")
			return ErrCannotBeConnected
		}
	}

	return nil
}

func (broker *MessageBroker) getIdentifiedQueue(queue string) string {
	return broker.clientId + queue
}

func (broker *MessageBroker) handleAMQPConnection() {
	amqpClosed := broker.connection.NotifyClose(make(chan *amqp091.Error))
	for err := range amqpClosed {
		broker.mutex.Lock()
		defer broker.mutex.Unlock()
		broker.isConnected = false

		if err != nil {
			log.Error().Err(err).Msgf("AMQP connection closed")
		}

		//TODO handle reconnection
	}
}

func (broker *MessageBroker) Publish(msg *RabbitMQMessage, topic, routingKey, correlationId string) error {
	if !broker.IsConnected() {
		return ErrMustBeConnected
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Interface(logProto, msg).Msgf("Publication ignored since marshal failed")
		return err
	}

	log.Debug().Str(logTopic, topic).Str(logRoutingKey, routingKey).Str(logContent, string(data)).Msgf("Sending message...")
	err = broker.publisherChannel.Publish(
		topic,
		routingKey,
		false,
		false,
		amqp091.Publishing{
			ContentType:   "application/protobuf",
			CorrelationId: correlationId,
			Body:          data,
		},
	)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to publish message")
		return err
	}

	return nil
}

func (broker *MessageBroker) Consume(queueName, routingKey string, consumer MessageConsumer) error {
	if !broker.IsConnected() {
		return ErrMustBeConnected
	}

	delivery, err := broker.consumerChannel.Consume(
		broker.getIdentifiedQueue(queueName), // queue
		broker.clientId,                      // consumer
		false,                                // auto ack
		false,                                // exclusive
		false,                                // no local
		false,                                // no wait
		nil,                                  // args
	)
	if err != nil {
		return err
	}

	go broker.dispatch(delivery, consumer)
	return nil
}

func (broker *MessageBroker) dispatch(delivery <-chan amqp091.Delivery, consumer MessageConsumer) {
	for data := range delivery {
		var message RabbitMQMessage
		if err := proto.Unmarshal(data.Body, &message); err != nil {
			log.Error().Err(err).Msgf("Protobuf unmarshal failed, message ignored. Continuing...")
		} else {
			broker.callConsumer(consumer, &message, data.CorrelationId)
		}
	}
}

func (broker *MessageBroker) callConsumer(consumer MessageConsumer, message *RabbitMQMessage, correlationId string) {
	defer func() {
		if err := recover(); err != nil {
			log.Error().Interface(logPanic, err).Msgf("Message consumer panicked. Continuing...")
		}
	}()
	consumer(message, correlationId)
}

func (broker *MessageBroker) IsConnected() bool {
	return broker.isConnected
}

func (broker *MessageBroker) Shutdown() {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	broker.isConnected = false

	if broker.publisherChannel != nil && !broker.publisherChannel.IsClosed() {
		err := broker.publisherChannel.Close()
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to close channel during AMQP Shutdown")
		}
	}

	if broker.consumerChannel != nil && !broker.consumerChannel.IsClosed() {
		err := broker.consumerChannel.Close()
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to close channel during AMQP Shutdown")
		}
	}

	if broker.connection != nil {
		err := broker.connection.Close()
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to close connection during AMQP Shutdown")
		}
	}
}
