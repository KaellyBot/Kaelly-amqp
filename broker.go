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
New initializes a new AMQP client instance with the specified client ID, address, and optional configurations.

Parameters:
  - clientID: A string identifier used to distinguish the client as a publisher or consumer within the AMQP broker.
  - address: A string representing the AMQP broker address in the following format:
    `amqp://username:password@hostname:port/vhost`
  - `username`: The username for authentication.
  - `password`: The password for authentication.
  - `hostname`: The hostname or IP address of the AMQP server.
  - `port`: The port number on which the AMQP server is listening.
  - `vhost`: The virtual host on the AMQP server to connect to.

Options:
- opts: A variadic list of functional options that allow optional configuration of the client
instance, such as retry delay and connection status callbacks. Use provided helper functions
like `WithRetryDelay` and `WithIsConnectedCallback` to set these options.

Returns:
- A pointer to an `Impl` struct, representing the configured AMQP client instance,
ready for publishing or consuming messages.

Example usage:

	broker := New(
		"myClientID",
		"amqp://user:pass@localhost:5672",
		WithRetryDelay(10*time.Second),
		WithIsConnectedCallback(myCallback),
	)
*/
func New(clientID, address string, opts ...Option) MessageBroker {
	broker := messageBroker{
		clientID: clientID,
		address:  address,
		cfg: config{
			bindings:            make([]Binding, 0),
			retryDelay:          defaultRetryDelay,
			isConnectedCallback: func(_ bool) {},
		},
		eventsChannels:      make(map[string](chan connectionEvent)),
		mainEventsChan:      make(chan connectionEvent, 1),
		eventsChannelsMutex: &sync.Mutex{},
		connectionMutex:     &sync.Mutex{},
		callbackMutex:       &sync.Mutex{},
		isConnected:         false,
	}

	for _, opt := range opts {
		opt(&broker.cfg)
	}

	return &broker
}

// Tries to connect to RabbitMQ server and provides the result of the first connection trial
// No matters if it successfully connects or not, it starts a go routine
// to watch out the connection and to make sure that it stays up
// User is expected to defer call Shutdown() to stop the connection in the end.
func (broker *messageBroker) Run() error {
	broker.connectionMutex.Lock()
	defer broker.connectionMutex.Unlock()

	if broker.shouldBeConnected {
		log.Error().Err(ErrAlreadyStarted).Msgf("Already started, abort to start")
		return ErrAlreadyStarted
	}

	broker.shouldBeConnected = true

	firstConnectionResult := make(chan error, 1)
	go broker.handleAMQPConnection(firstConnectionResult)
	return <-firstConnectionResult
}

func (broker *messageBroker) Emit(msg *RabbitMQMessage, exchange Exchange, routingKey,
	correlationID string) error {
	return broker.publish(msg, exchange, routingKey, correlationID, "")
}

func (broker *messageBroker) Request(msg *RabbitMQMessage, exchange Exchange, routingKey,
	correlationID, replyTo string) error {
	return broker.publish(msg, exchange, routingKey, correlationID, replyTo)
}

func (broker *messageBroker) Reply(msg *RabbitMQMessage, correlationID, replyTo string) error {
	return broker.publish(msg, Exchange(""), replyTo, correlationID, "")
}

func (broker *messageBroker) Consume(queueName string, consumer MessageConsumer) {
	go func() {
		eventChanID, eventChan := broker.declareEventChannel()
		defer broker.deleteEventChannel(eventChanID)

		for {
			consumerGoChannel, err := broker.tryingToSetUpConsume(eventChan, queueName)
			if err != nil {
				return
			}
			action := broker.listenToMessages(eventChan, consumer, consumerGoChannel)
			switch action {
			case exit:
				return
			case reconnect:
				continue
			default:
				log.Error().Err(err).Msgf("Unexpected consume end. Abort.")
				return
			}
		}
	}()
}

func (broker *messageBroker) IsConnected() bool {
	return broker.isConnected
}

func (broker *messageBroker) Shutdown() {
	broker.connectionMutex.Lock()
	defer broker.connectionMutex.Unlock()

	if !broker.shouldBeConnected {
		log.Warn().Msgf("Already shutting down, shutdown action not taken into account")
		return
	}

	log.Info().Msgf("Shutdown the connection to RabbitMQ server")
	broker.shouldBeConnected = false
	broker.emitEvent(shutdown)
	broker.callIsConnectedCallback(false)
}

func (broker *messageBroker) getIdentifiedQueue(queue string) string {
	return fmt.Sprintf("%s.%s", broker.clientID, queue)
}

func (broker *messageBroker) declareBindings() error {
	for _, binding := range broker.cfg.bindings {
		uniqueQueue := broker.getIdentifiedQueue(binding.Queue)
		routingKey := binding.RoutingKey
		if routingKey == "" {
			routingKey = uniqueQueue
		}

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
			uniqueQueue,              // name
			routingKey,               // key
			string(binding.Exchange), // exchange
			false,                    // noWait
			nil,                      // arguments
		)
		if err != nil {
			log.Error().Err(err).
				Str(logExchange, string(binding.Exchange)).
				Str(logQueue, uniqueQueue).
				Str(logRoutingKey, routingKey).
				Msg("Failed to bind")
			return ErrCannotBeConnected
		}
	}

	return nil
}

func (broker *messageBroker) getConsumerTag(queue string) string {
	return fmt.Sprintf("%s.%s", broker.clientID, queue)
}

func (broker *messageBroker) handleAMQPConnection(firstConnectionResult chan<- error) {
	init := true
	for {
		log.Info().Msgf("Trying to connect to RabbitMQ Server...")

		select {
		case event, ok := <-broker.mainEventsChan:
			if event == shutdown || !ok {
				log.Info().Msg("Application shutdown has been requested. Stop to maintain AMQP connection")
				return
			}
		default:
		}

		err := broker.connect()
		if err != nil {
			if init {
				init = false
				firstConnectionResult <- err
			}
			time.Sleep(broker.cfg.retryDelay)
			continue
		} else if init {
			init = false
			close(firstConnectionResult)
		}
		broker.emitEvent(connected)
		broker.callIsConnectedCallback(true)
		log.Info().Msgf("Connected to RabbitMQ Server")

		select {
		case event, ok := <-broker.mainEventsChan:
			if event == shutdown || !ok {
				log.Info().Msg("Application shutdown has been requested. Stop to maintain AMQP connection")
				return
			}
		case connectionError := <-broker.notifyConClose:
			log.Error().Err(connectionError).
				Msg("Detected an abnormal disconnection. Trying to reconnect...")
			broker.emitEvent(issue)
			broker.callIsConnectedCallback(false)
		case channelError := <-broker.notifyPubChanClose:
			log.Error().Err(channelError).
				Msg("Detected an issue in the publisher channel. Trying to reinit communication...")
			broker.emitEvent(issue)
			broker.callIsConnectedCallback(false)
			broker.killConnectionsAndChannels()
		case channelError := <-broker.notifySubChanClose:
			log.Error().Err(channelError).
				Msg("Detected an issue in the consumer channel. Trying to reinit communication...")
			broker.emitEvent(issue)
			broker.callIsConnectedCallback(false)
			broker.killConnectionsAndChannels()
		}
	}
}

func (broker *messageBroker) connect() error {
	var err error
	if broker.connection, err = amqp091.Dial(broker.address); err != nil {
		log.Error().Err(err).Msgf("Failed to connect to AMQP server")
		return ErrCannotBeConnected
	}
	broker.notifyConClose = broker.connection.NotifyClose(make(chan *amqp091.Error, 1))

	if broker.publisherChannel, err = broker.connection.Channel(); err != nil {
		log.Error().Err(err).Msgf("Failed to retrieve channel from AMQP server connection")
		broker.killConnectionsAndChannels()
		return ErrCannotBeConnected
	}
	broker.notifyPubChanClose = broker.publisherChannel.NotifyClose(make(chan *amqp091.Error, 1))

	if broker.consumerChannel, err = broker.connection.Channel(); err != nil {
		log.Error().Err(err).Msgf("Failed to retrieve channel from AMQP server connection")
		broker.killConnectionsAndChannels()
		return ErrCannotBeConnected
	}
	broker.notifySubChanClose = broker.consumerChannel.NotifyClose(make(chan *amqp091.Error, 1))

	if err = broker.declareBindings(); err != nil {
		log.Error().Err(err).Msgf("Failed to declare topics, queues and bindings")
		broker.killConnectionsAndChannels()
		return ErrCannotBeConnected
	}

	return nil
}

func (broker *messageBroker) emitEvent(event connectionEvent) {
	broker.eventsChannelsMutex.Lock()
	defer broker.eventsChannelsMutex.Unlock()

	// Start by flushing all channels to be sure that everyone will have an uptodate information
	for _, eventHandler := range broker.eventsChannels {
		select {
		case <-eventHandler:
		default:
		}
	}

	// No need to send that client is connected. The only purpose of calling emitEvent(connected) is to flush the issues
	if event == connected {
		return
	}

	if event == shutdown {
		broker.mainEventsChan <- shutdown
	}

	for _, eventHandler := range broker.eventsChannels {
		eventHandler <- event
	}
}

func (broker *messageBroker) killConnectionsAndChannels() {
	if broker.publisherChannel != nil && !broker.publisherChannel.IsClosed() {
		err := broker.publisherChannel.Close()
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to close publisher channel during AMQP Shutdown. Continuing shutdown...")
		}
	}

	if broker.consumerChannel != nil && !broker.consumerChannel.IsClosed() {
		err := broker.consumerChannel.Close()
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to close consumer channel during AMQP Shutdown. Continuing shutdown...")
		}
	}

	if broker.connection != nil {
		err := broker.connection.Close()
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to close connection during AMQP Shutdown. Continuing shutdown...")
		}
	}
}

func (broker *messageBroker) callIsConnectedCallback(status bool) {
	broker.callbackMutex.Lock()
	defer broker.callbackMutex.Unlock()
	defer func() {
		if err := recover(); err != nil {
			log.Error().Interface(logPanic, err).Msgf("External callback function panicked. Continuing...")
		}
	}()
	if broker.isConnected != status {
		broker.isConnected = status
		broker.cfg.isConnectedCallback(status)
	}
}

func (broker *messageBroker) publish(msg *RabbitMQMessage, exchange Exchange,
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
			ReplyTo:       broker.getIdentifiedQueue(replyTo),
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

func (broker *messageBroker) tryingToSetUpConsume(eventChan chan connectionEvent, queue string,
) (<-chan amqp091.Delivery, error) {
	for {
		select {
		case event := <-eventChan:
			if event == shutdown {
				return nil, ErrActionCanceledByUser
			}
			if event == issue {
				time.Sleep(broker.cfg.retryDelay)
				continue
			}
		default:
		}

		consumerGoChannel, err := broker.consumerChannel.Consume(
			broker.getIdentifiedQueue(queue), // queue
			broker.getConsumerTag(queue),     // consumer
			true,                             // auto ack
			false,                            // exclusive
			false,                            // no local
			false,                            // no wait
			nil,                              // args
		)
		if err != nil {
			log.Debug().Err(err).Msgf("Impossible to start consuming messages. Retrying...")
			time.Sleep(broker.cfg.retryDelay)
			continue
		}
		log.Debug().Msgf("Starting to consume")
		return consumerGoChannel, nil
	}
}

func (broker *messageBroker) listenToMessages(eventChan chan connectionEvent,
	consumer MessageConsumer, consumerGoChannel <-chan amqp091.Delivery) interruptionAction {
	for {
		select {
		case event := <-eventChan:
			if event == shutdown {
				log.Info().Msgf("Application shutdown has been requested. Stopping to consume")
				return exit
			} else if event == issue {
				log.Debug().Err(ErrCannotBeConnected).Msgf("Retrying to consume...")
				return reconnect
			}
		case delivery, ok := <-consumerGoChannel:
			if !ok {
				break
			}

			ctx := withData(delivery)
			var message RabbitMQMessage
			if err := proto.Unmarshal(delivery.Body, &message); err != nil {
				log.Error().Err(err).Msgf("Protobuf unmarshal failed, message ignored. Continuing...")
				break
			}

			go broker.callConsumer(ctx, consumer, &message)
		}
	}
}

func (broker *messageBroker) callConsumer(ctx Context, consumer MessageConsumer, message *RabbitMQMessage) {
	defer func() {
		if err := recover(); err != nil {
			log.Error().Interface(logPanic, err).Msgf("Message consumer panicked. Continuing...")
		}
	}()
	consumer(ctx, message)
}

func (broker *messageBroker) declareEventChannel() (string, chan connectionEvent) {
	broker.eventsChannelsMutex.Lock()
	defer broker.eventsChannelsMutex.Unlock()

	eventChan := make(chan connectionEvent, 1)
	var eventChanID string
	for success := false; !success; {
		eventChanID = GenerateUUID()
		if _, valueAlreadyExists := broker.eventsChannels[eventChanID]; !valueAlreadyExists {
			broker.eventsChannels[eventChanID] = eventChan
			success = true
		}
	}
	return eventChanID, eventChan
}

func (broker *messageBroker) deleteEventChannel(eventChanID string) {
	broker.eventsChannelsMutex.Lock()
	defer broker.eventsChannelsMutex.Unlock()

	delete(broker.eventsChannels, eventChanID)
}
