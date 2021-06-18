package rabbitmq

import (
	"encoding/json"
	"fmt"

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

func PublishToStream(msg *QueueMessage) error {
	conn, err := amqp.Dial(config.GetConfig().AMQPUrl)
	if err != nil {
		return fmt.Errorf("error connecting to the message broker")
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open a channel")
	}
	defer ch.Close()

	topicName := "mqtt"
	queueName := "messages"
	err = ch.ExchangeDeclare(topicName, "topic", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed creating the exchange")
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("error formatting message")
	}

	message := amqp.Publishing{Body: body}

	err = ch.Publish(topicName, "random-key", false, false, message)
	if err != nil {
		return fmt.Errorf("failed publishing a message to the queue")
	}

	_, err = ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("error declaring the queue")
	}

	err = ch.QueueBind(queueName, "#", topicName, false, nil)
	if err != nil {
		return fmt.Errorf("error binding the queue")
	}

	log.Debug("a new message has been published to the queue")

	return nil
}
