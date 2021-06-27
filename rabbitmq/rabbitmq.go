package rabbitmq

import (
	"encoding/json"

	"github.com/seb7887/thoth/config"
	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
)

type QueueMessage struct {
	ClientId  string
	Topic     string
	Payload   string
	Timestamp int64
}

const (
	consumer = "consumer"
)

// channel to publish RabbitMQ messages
var pchan = make(chan QueueMessage, 10)

func InitProducer() {
	// connect to RabbitMQ service
	conn, err := amqp.Dial(config.GetConfig().AMQPUrl)
	if err != nil {
		log.Fatal("ERROR: fail init RabbitMQ producer")
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal("failed to open a channel")
	}

	for {
		select {
		case msg := <-pchan:
			// format message
			body, err := json.Marshal(msg)
			if err != nil {
				log.Error("error formatting message")
			}
			message := amqp.Publishing{Body: body}

			// publish message
			err = ch.Publish(
				"",       // exchange
				consumer, // routing key
				false,    // mandatory
				false,    // immediate
				message,  // message to be published
			)
			if err != nil {
				log.Error("failed publishing a message to the queue")
			}
		}
	}

}

func PublishToStream(msg *QueueMessage) {
	pchan <- *msg
}
