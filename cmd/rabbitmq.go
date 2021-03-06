package cmd

import (
	"fmt"
	"github.com/streadway/amqp"
	"github.com/valicm/rabbinator/cmd/utility"
	"log"
	"log/syslog"
)

// Defines statuses upon we decide what we are doing
// with that message.
const (
	queueSuccess = "success"
	queueReject  = "reject"
	queueRetry   = "retry"
	queueUnknown = "unknown"
)

// Stored configuration for processing queue.
var config utility.Config

// Initialize all task necessarily for establishing connection.
func Initialize(consumer string, configDir string) {

	// Initialize and set config.
	config = utility.ConfigSetup(consumer, configDir)

	// Set syslog.
	initializeLogger()

	// Make connection to RabbitMQ.
	connectRabbitMQ()
}

// Set syslog for later log writes.
func initializeLogger() {
	tag := "rabbitmq_log" + config.Type
	logwriter, e := syslog.New(syslog.LOG_ERR, tag)
	if e == nil {
		log.SetOutput(logwriter)
	}
}

// Rabbit connection handler and processing items.
func connectRabbitMQ() {

	// Start connection.
	conn, err := amqp.Dial(config.Client.Uri)
	utility.InitErrorHandler("Failed to connect to RabbitMQ", err)
	defer conn.Close()

	ch, err := conn.Channel()
	utility.InitErrorHandler("Failed to open a channel", err)
	defer ch.Close()

	// Declare queue.
	_, err = ch.QueueDeclare(
		config.QueueName,
		config.Client.Queue.Durable,
		config.Client.Queue.AutoDelete,
		config.Client.Queue.Exclusive,
		config.Client.Queue.NoWait,
		nil,
	)
	utility.InitErrorHandler("Failed to declare a queue", err)

	err = ch.Qos(
		config.Client.Prefetch.Count,
		config.Client.Prefetch.Size,
		config.Client.Prefetch.Global,
	)
	utility.InitErrorHandler("Failed to set QoS", err)

	msgs, err := ch.Consume(
		config.QueueName,
		config.Consumer,
		config.Client.Consume.AutoAck,
		config.Client.Consume.Exclusive,
		config.Client.Consume.NoLocal,
		config.Client.Consume.NoWait,
		nil,
	)
	utility.InitErrorHandler("Failed to register a consumer", err)

	forever := make(chan bool)

	go func() {
		for d := range msgs {
			// Process queue items.
			processQueueItem(d)
		}
	}()

	fmt.Println(" [*] Waiting for messages. To exit press CTRL+C")
	<-forever
}

// Process queue item.
// TODO: make it dynamic?
func processQueueItem(Delivery amqp.Delivery) {
	// Set defaults.
	result := queueUnknown

	// Ignore default case. If type is not mapped
	// item would be discarded from RabbitMQ.
	switch config.Type {
	case "mandrill":
		result = processMandrillItem(Delivery.Body, config.ApiKey, config.Templates.Default, config.Templates.Modules)
	case "mailchimp":
		result = processMailchimpItem(Delivery.Body, config.ApiKey)
	}

	// Use reject for rejecting and requeue of items.
	switch result {
	case queueSuccess:
		Delivery.Ack(true)
	case queueReject:
		Delivery.Reject(false)
	case queueRetry:
		Delivery.Reject(true)
	default:
		Delivery.Nack(true, false)
	}

}
