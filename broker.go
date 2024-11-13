package amqp

import (
	"fmt"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
)

/*
- clientID: ID used to be identified as a publisher/consumer
- address: string with the following format: `amqp://username:password@hostname:port/vhost`
- topics: topics used to publish and consume messages
- bindings: topic, routingKey and queue used to consume messages.
*/
func New(clientID, address string, bindings []Binding) (*Impl, error) {
	broker := Impl{
		clientID:    clientID,
		address:     address,
		bindings:    bindings,
		isConnected: false,
		mutex:       &sync.Mutex{},
	}

	return &broker, broker.dial()
}

func (broker *Impl) dial() error {
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

func (broker *Impl) declareBindings() error {
	for _, binding := range broker.bindings {
		uniqueQueue := broker.GetIdentifiedQueue(binding.Queue)
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
			uniqueQueue,
			binding.RoutingKey,
			string(binding.Exchange),
			false,
			nil)
		if err != nil {
			log.Error().Err(err).
				Str(logExchange, string(binding.Exchange)).
				Str(logQueue, uniqueQueue).
				Str(logRoutingKey, binding.RoutingKey).
				Msg("Failed to bind")
			return ErrCannotBeConnected
		}
	}

	return nil
}

func (broker *Impl) getConsumerTag(queue string) string {
	return fmt.Sprintf("%s.%s", broker.clientID, queue)
}

func (broker *Impl) GetIdentifiedQueue(queue string) string {
	return fmt.Sprintf("%s.%s", broker.clientID, queue)
}

func (broker *Impl) handleAMQPConnection() {
	amqpClosed := broker.connection.NotifyClose(make(chan *amqp091.Error))
	for err := range amqpClosed {
		if err != nil {
			log.Error().Err(err).Msgf("AMQP connection closed, trying to reconnect...")
			broker.reconnect()
		}
	}
}

func (broker *Impl) reconnect() {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	broker.isConnected = false

	// TODO handle reconnection
}

func (broker *Impl) Emit(msg *RabbitMQMessage, exchange Exchange, routingKey,
	correlationID string) error {
	return broker.publish(msg, exchange, routingKey, correlationID, "")
}

func (broker *Impl) Request(msg *RabbitMQMessage, exchange Exchange, routingKey,
	correlationID, replyTo string) error {
	return broker.publish(msg, exchange, routingKey, correlationID, replyTo)
}

func (broker *Impl) Reply(msg *RabbitMQMessage, correlationID, replyTo string) error {
	return broker.publish(msg, Exchange(""), replyTo, correlationID, "")
}

func (broker *Impl) publish(msg *RabbitMQMessage, exchange Exchange,
	routingKey, correlationID, replyTo string) error {
	if !broker.IsConnected() {
		return ErrMustBeConnected
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Interface(logProto, msg).Msgf("Publication ignored since marshal failed")
		return err
	}

	log.Debug().
		Str(logExchange, string(exchange)).
		Str(logCorrelationID, correlationID).
		Str(logRoutingKey, routingKey).
		Str(logReplyTo, replyTo).
		Str(logContent, string(data)).
		Msgf("Publishing message...")
	err = broker.publisherChannel.Publish(
		string(exchange),
		routingKey,
		false,
		false,
		amqp091.Publishing{
			ContentType:   "application/protobuf",
			CorrelationId: correlationID,
			ReplyTo:       broker.GetIdentifiedQueue(replyTo),
			Timestamp:     time.Now(),
			Body:          data,
		},
	)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to publish message")
		return err
	}

	return nil
}

func (broker *Impl) Consume(queueName string, consumer MessageConsumer) error {
	if !broker.IsConnected() {
		return ErrMustBeConnected
	}

	delivery, err := broker.consumerChannel.Consume(
		broker.GetIdentifiedQueue(queueName), // queue
		broker.getConsumerTag(queueName),     // consumer
		true,                                 // auto ack
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

func (broker *Impl) dispatch(delivery <-chan amqp091.Delivery, consumer MessageConsumer) {
	for data := range delivery {
		ctx := withData(data)
		var message RabbitMQMessage
		if err := proto.Unmarshal(data.Body, &message); err != nil {
			log.Error().Err(err).Msgf("Protobuf unmarshal failed, message ignored. Continuing...")
		} else {
			broker.callConsumer(ctx, consumer, &message)
		}
	}
}

func (broker *Impl) callConsumer(ctx Context, consumer MessageConsumer, message *RabbitMQMessage) {
	defer func() {
		if err := recover(); err != nil {
			log.Error().Interface(logPanic, err).Msgf("Message consumer panicked. Continuing...")
		}
	}()
	consumer(ctx, message)
}

func (broker *Impl) IsConnected() bool {
	return broker.isConnected
}

func (broker *Impl) Shutdown() {
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
