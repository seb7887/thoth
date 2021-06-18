package broker

import (
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
	log "github.com/sirupsen/logrus"
)

func publish(sub *subscription, packet *packets.PublishPacket) {
	switch packet.Qos {
	case QosAtMostOnce:
		err := sub.client.WritePacket(packet)
		if err != nil {
			log.Error("process message for psub error")
		}
	case QosAtLeastOnce:
		sub.client.inflightMu.Lock()
		sub.client.inflight[packet.MessageID] = &inflightElement{status: Publish, packet: packet, timestamp: time.Now().Unix()}
		sub.client.inflightMu.Unlock()
		err := sub.client.WritePacket(packet)
		if err != nil {
			log.Error("process message for psub error")
		}
		sub.client.ensureRetryTimer()
	default:
		log.Error("publish with unknown QoS")
		return
	}
}

// timer for retry delivery
func (c *client) ensureRetryTimer(interval ...int64) {
	if c.retryTimer != nil {
		return
	}

	if len(interval) > 1 {
		return
	}

	timerInterval := retryInterval
	if len(interval) == 1 {
		timerInterval = interval[0]
	}

	c.retryTimerLock.Lock()
	c.retryTimer = time.AfterFunc(time.Duration(timerInterval)*time.Second, c.retryDelivery)
	c.retryTimerLock.Unlock()
	return
}

func (c *client) resetRetryTimer() {
	if c.retryTimer == nil {
		return
	}

	// reset timer
	c.retryTimerLock.Lock()
	c.retryTimer = nil
	c.retryTimerLock.Unlock()
}

func (c *client) retryDelivery() {
	c.resetRetryTimer()
	c.inflightMu.RLock()
	ilen := len(c.inflight)
	// reset timer when client offline or inflight is empty
	if c.conn == nil || ilen == 0 {
		c.inflightMu.RUnlock()
		return
	}

	// copy the retried elements out of the map to only hold the lock for a short time
	// and use the new slice later to iterate through them
	toRetryElem := make([]*inflightElement, 0, ilen)
	for _, infElem := range c.inflight {
		toRetryElem = append(toRetryElem, infElem)
	}
	c.inflightMu.RUnlock()
	now := time.Now().Unix()

	for _, infElem := range toRetryElem {
		age := now - infElem.timestamp
		if age >= retryInterval {
			if infElem.status == Publish {
				c.WritePacket(infElem.packet)
				infElem.timestamp = now
			} else if infElem.status == Pubrel {
				pubrel := packets.NewControlPacket(packets.Pubrel).(*packets.PubrelPacket)
				pubrel.MessageID = infElem.packet.MessageID
				c.WritePacket(pubrel)
				infElem.timestamp = now
			}
		} else {
			if age < 0 {
				age = 0
			}
			c.ensureRetryTimer(retryInterval - age)
		}
	}
	c.ensureRetryTimer()
}
