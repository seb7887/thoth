package broker

import (
	"strings"

	"github.com/seb7887/thoth/utils"

	"github.com/eclipse/paho.mqtt.golang/packets"
	log "github.com/sirupsen/logrus"
)

func (c *client) ProcessSubscribe(packet *packets.SubscribePacket) {
	switch c.typ {
	case CLIENT:
		c.processClientSubscribe(packet)
	case ROUTER:
		fallthrough
	case REMOTE:
		c.processRouterSubscribe(packet)
	}
}

func (c *client) processClientSubscribe(packet *packets.SubscribePacket) {
	if c.status == Disconnected {
		return
	}

	b := c.broker
	if b == nil {
		return
	}

	topics := packet.Topics
	qoss := packet.Qoss

	suback := packets.NewControlPacket(packets.Suback).(*packets.SubackPacket)
	suback.MessageID = packet.MessageID
	var retcodes []byte

	for i, topic := range topics {
		t := topic

		groupName := ""
		share := false
		if strings.HasPrefix(topic, "$share/") {
			substr := groupCompile.FindStringSubmatch(topic)
			if len(substr) != 3 {
				retcodes = append(retcodes, QosFailure)
				continue
			}
			share = true
			groupName = substr[1]
			topic = substr[2]
		}

		if oldSub, exist := c.subMap[t]; exist {
			c.topicsMgr.Unsubscribe([]byte(oldSub.topic), oldSub)
			delete(c.subMap, t)
		}

		sub := &subscription{
			topic: topic,
			qos: qoss[i],
			client: c,
			share: share,
			groupName: groupName,
		}

		rqos, err := c.topicsMgr.Subscribe([]byte(topic), qoss[i], sub)
		if err != nil {
			log.Error("subscribe error")
			retcodes = append(retcodes, QosFailure)
			continue
		}

		c.subMap[t] = sub

		c.session.AddTopic(t, qoss[i])
		retcodes = append(retcodes, rqos)
		c.topicsMgr.Retained([]byte(topic), &c.rmsgs)
	}

	suback.ReturnCodes = retcodes

	err := c.WritePacket(suback)
	if err != nil {
		log.Error("send suback error")
		return
	}

	// process retain message
	for _, rm := range c.rmsgs {
		if err := c.WritePacket(rm); err != nil {
			log.Errorf("error publishing retained message: %s", err.Error())
		} else {
			log.Info("process retained message")
		}
	}
}

func (c *client) processRouterSubscribe(packet *packets.SubscribePacket) {
	if c.status == Disconnected {
		return
	}

	b := c.broker
	if b == nil {
		return
	}
	topics := packet.Topics
	qoss := packet.Qoss

	suback := packets.NewControlPacket(packets.Suback).(*packets.SubackPacket)
	suback.MessageID = packet.MessageID
	var retcodes []byte

	for i, topic := range topics {
		t := topic
		groupName := ""
		share := false
		if strings.HasPrefix(topic, "$share/") {
			substr := groupCompile.FindStringSubmatch(topic)
			if len(substr) != 3 {
				retcodes = append(retcodes, QosFailure)
				continue
			}
			share = true
			groupName = substr[1]
			topic = substr[2]
		}

		sub := &subscription{
			topic:     topic,
			qos:       qoss[i],
			client:    c,
			share:     share,
			groupName: groupName,
		}

		rqos, err := c.topicsMgr.Subscribe([]byte(topic), qoss[i], sub)
		if err != nil {
			log.Error("subscribe error")
			retcodes = append(retcodes, QosFailure)
			continue
		}

		c.subMap[t] = sub
		utils.AddSubMap(c.routeSubMap, topic)
		retcodes = append(retcodes, rqos)
	}

	suback.ReturnCodes = retcodes

	err := c.WritePacket(suback)
	if err != nil {
		log.Error("send suback error")
		return
	}
}