package broker

import (
	"fmt"
	"time"

	"github.com/bitly/go-simplejson"
	"github.com/eclipse/paho.mqtt.golang/packets"
	log "github.com/sirupsen/logrus"
)

func (c *client) SendInfo() {
	if c.status == Disconnected {
		return
	}

	url := c.info.localIP + ":" + c.broker.config.Cluster.Port

	infoMsg := NewInfo(c.broker.id, url, false)
	err := c.WritePacket(infoMsg)
	if err != nil {
		log.Error("send info message error")
		return
	}
}

func (c *client) StartPing() {
	timeTicker := time.NewTicker(time.Second * 50) // 50 seconds
	ping := packets.NewControlPacket(packets.Pingreq).(*packets.PingreqPacket)
	for {
		select {
		case <-timeTicker.C:
			err := c.WritePacket(ping)
			if err != nil {
				log.Error("ping error")
				c.Close()
			}
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *client) SendConnect() {
	if c.status != Connected {
		return
	}

	m := packets.NewControlPacket(packets.Connect).(*packets.ConnectPacket)
	m.ProtocolName = "MQIsdp"
	m.ProtocolVersion = 3

	m.CleanSession = true
	m.ClientIdentifier = c.info.clientId
	m.Keepalive = uint16(60)
	err := c.WritePacket(m)
	if err != nil {
		log.Error("send connect message error")
		return
	}
	log.Info("send connect success")
}

func NewInfo(sid, url string, isforword bool) *packets.PublishPacket {
	pub := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	pub.Qos = 0
	pub.TopicName = BrokerInfoTopic
	pub.Retain = false
	info := fmt.Sprintf(`{"brokerID": "%s", "brokerUrl": "%s"}`, sid, url)
	pub.Payload = []byte(info)
	return pub
}

func (c *client) ProcessInfo(packet *packets.PublishPacket) {
	nc := c.conn
	b := c.broker
	if nc == nil {
		return
	}

	log.Info("recv remoteInfo")

	js, err := simplejson.NewJson(packet.Payload)
	if err != nil {
		log.Warn("parse json info message err")
		return
	}

	routes, err := js.Get("data").Map()
	if routes == nil {
		log.Error("receive info message error")
		return
	}

	b.nodes = routes
}
