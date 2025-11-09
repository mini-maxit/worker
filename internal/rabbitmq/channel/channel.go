package channel

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

// Channel abstracts the subset of *amqp.Channel used by the project.
type Channel interface {
	Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	Consume(queue, consumer string,
		autoAck, exclusive, noLocal, noWait bool,
		args amqp.Table) (<-chan amqp.Delivery, error)
}

// AmqpChannel wraps a real *amqp.Channel and implements Channel.
type AmqpChannel struct {
	ch *amqp.Channel
}

func NewAmqpChannel(ch *amqp.Channel) *AmqpChannel { return &AmqpChannel{ch: ch} }

func (a *AmqpChannel) Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	return a.ch.Publish(exchange, key, mandatory, immediate, msg)
}

func (a *AmqpChannel) QueueDeclare(name string,
	durable, autoDelete, exclusive, noWait bool,
	args amqp.Table) (amqp.Queue, error) {
	return a.ch.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
}

func (a *AmqpChannel) Consume(queue, consumer string,
	autoAck, exclusive, noLocal, noWait bool,
	args amqp.Table) (<-chan amqp.Delivery, error) {
	return a.ch.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}
