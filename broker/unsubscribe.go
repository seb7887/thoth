package broker

import (
	"github.com/eclipse/paho.mqtt.golang/packets"
	"github.com/seb7887/thoth/utils"
	log "github.com/sirupsen/logrus"
)

func (c *client) ProcessUnsubscribe(packet *packets.UnsubscribePacket) {
	switch c.typ {
	case CLIENT:
		c.processClientUnsubscribe(packet)
	case ROUTER:
		c.processRouterUnsubscribe(packet)
	}
}

func (c *client) processRouterUnsubscribe(packet *packets.UnsubscribePacket) {
	if c.status == Disconnected {
		return
	}

	b := c.broker
	if b == nil {
		return
	}

	topics := packet.Topics

	for _, topic := range topics {
		sub, exist := c.subMap[topic]
		if exist {
			retainNum := utils.DelSubMap(c.routeSubMap, topic)
			if retainNum > 0 {
				continue
			}
			c.topicsMgr.Unsubscribe([]byte(sub.topic), sub)
			delete(c.subMap, topic)
		}
	}

	unsuback := packets.NewControlPacket(packets.Unsuback).(*packets.UnsubackPacket)
	unsuback.MessageID = packet.MessageID

	err := c.WritePacket(unsuback)
	if err != nil {
		log.Error("send suback error")
		return
	}
}

func (c *client) processClientUnsubscribe(packet *packets.UnsubscribePacket) {
	if c.status == Disconnected {
		return
	}

	b := c.broker
	if b == nil {
		return
	}

	topics := packet.Topics

	for _, topic := range topics {
		sub, exist := c.subMap[topic]
		if exist {
			c.topicsMgr.Unsubscribe([]byte(sub.topic), sub)
			c.session.RemoveTopic(topic)
			delete(c.subMap, topic)
		}
	}

	unsuback := packets.NewControlPacket(packets.Unsuback).(*packets.UnsubackPacket)
	unsuback.MessageID = packet.MessageID

	err := c.WritePacket(unsuback)
	if err != nil {
		log.Error("send unsuback error")
		return
	}
	// //process ubsubscribe message
	b.BroadcastSubOrUnsubMessage(packet)
}
