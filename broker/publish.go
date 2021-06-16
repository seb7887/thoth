package broker

import (
	"github.com/eclipse/paho.mqtt.golang/packets"
	log "github.com/sirupsen/logrus"
)

func (c *client) ProcessPublish(packet *packets.PublishPacket) {
	switch c.typ {
	case CLIENT:
		c.processClientPublish(packet)
	case ROUTER:
		c.processRouterPublish(packet)
	case CLUSTER:
		c.processRemotePublish(packet)
	}
}

func (c *client) processRemotePublish(packet *packets.PublishPacket) {
	if c.status == Disconnected {
		return
	}

	topic := packet.TopicName
	if topic == BrokerInfoTopic {
		c.ProcessInfo(packet)
		return
	}
}

func (c *client) processRouterPublish(packet *packets.PublishPacket) {
	if c.status == Disconnected {
		return
	}

	switch packet.Qos {
	case QosAtMostOnce:
		c.ProcessPublishMessage(packet)
	case QosAtLeastOnce:
		puback := packets.NewControlPacket(packets.Puback).(*packets.PubackPacket)
		puback.MessageID = packet.MessageID
		if err := c.WritePacket(puback); err != nil {
			log.Error("send puback error")
			return
		}
		c.ProcessPublishMessage(packet)
	case QosExactlyOnce:
		return
	default:
		log.Error("publish with unknown QoS")
		return
	}
}

func (c *client) processClientPublish(packet *packets.PublishPacket) {
	//topic := packet.TopicName

	// TODO: broker check auth topic

	// TODO: publish RabbitMQ

	switch packet.Qos {
	case QosAtMostOnce:
		c.ProcessPublishMessage(packet)
	case QosAtLeastOnce:
		puback := packets.NewControlPacket(packets.Puback).(*packets.PubackPacket)
		puback.MessageID = packet.MessageID
		if err := c.WritePacket(puback); err != nil {
			log.Error("send puback error")
			return
		}
		c.ProcessPublishMessage(packet)
	case QosExactlyOnce:
		if err := c.registerPublishPacketId(packet.MessageID); err != nil {
			return
		} else {
			pubrec := packets.NewControlPacket(packets.Pubrec).(*packets.PubrecPacket)
			pubrec.MessageID = packet.MessageID
			if err := c.WritePacket(pubrec); err != nil {
				log.Error("send pubrec error")
				return
			}
			c.ProcessPublishMessage(packet)
		}
		return
	default:
		log.Error("publish with unknown QoS")
		return
	}
}

func (c *client) ProcessPublishMessage(packet *packets.PublishPacket) {
	b := c.broker
	if b == nil {
		return
	}
	typ := c.typ

	if packet.Retain {
		if err := c.topicsMgr.Retain(packet); err != nil {
			log.Error("Error retaining message")
		}
	}

	err := c.topicsMgr.Subscribers([]byte(packet.TopicName), packet.Qos, &c.subs, &c.qoss)
	if err != nil {
		log.Error("Error retrieving subscribers list")
		return
	}

	if len(c.subs) == 0 {
		return
	}

	var qsub []int
	for i, sub := range c.subs {
		s, ok := sub.(*subscription)
		if ok {
			if s.client.typ == ROUTER {
				if typ != CLIENT {
					continue
				}
			}
			if s.share {
				qsub = append(qsub, i)
			} else {
				publish(s, packet)
			}
		}
	}

	if len(qsub) > 0 {
		idx := r.Intn(len(qsub))
		sub := c.subs[qsub[idx]].(*subscription)
		publish(sub, packet)
	}
}