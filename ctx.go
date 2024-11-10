package amqp

import (
	"context"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type Context struct {
	context.Context
	CorrelationID string
	RoutingKey    string
	ReplyTo       string
	Timestamp     time.Time
}

func withData(data amqp091.Delivery) Context {
	return Context{
		Context:       context.Background(),
		CorrelationID: data.CorrelationId,
		RoutingKey:    data.RoutingKey,
		ReplyTo:       data.ReplyTo,
		Timestamp:     data.Timestamp,
	}
}
