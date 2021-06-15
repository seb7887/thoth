package broker

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"net"
	"regexp"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/eapache/queue"
	"github.com/eclipse/paho.mqtt.golang/packets"
	"github.com/seb7887/thoth/broker/persistence/sessions"
	"github.com/seb7887/thoth/broker/persistence/topics"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
)


const (
	BrokerInfoTopic = "broker000100101info" // special pub topic for cluster info BrokerInfoTopic
	CLIENT = 0 // end-user
	ROUTER = 1 // another router in the cluster
	REMOTE = 2 // the router connect to other router
	CLUSTER = 3
)

const (
	Connected = 1
	Disconnected = 2
)

const (
	awaitRelTimeout int64 = 20
	retryInterval int64 = 20
)

const (
	_GroupTopicRegexp = `^\$share/([0-9a-zA-Z_-]+)/(.*)$`
)

var (
	groupCompile = regexp.MustCompile(_GroupTopicRegexp)
)

type InflightStatus uint8

const (
	Publish InflightStatus = 0
	Pubrel InflightStatus = 1
)

var (
	DisconnectedPacket = packets.NewControlPacket(packets.Disconnect).(*packets.DisconnectPacket)
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
)

type info struct {
	clientId string
	username string
	password string
	keepalive uint16
	willMsg *packets.PublishPacket
	localIP string
	remoteIP string
}

type inflightElement struct {
	status InflightStatus
	packet *packets.PublishPacket
	timestamp int64
}

type route struct {
	remoteID string
	remoteUrl string
}

type client struct {
	typ int
	mu sync.Mutex
	broker *Broker
	conn net.Conn
	info info
	status int
	qoss []byte
	subs []interface{}
	subMap map[string]*subscription
	rmsgs []*packets.PublishPacket
	route route
	routeSubMap map[string]uint64
	awaitingRel map[uint16]int64
	maxAwaitingRel int
	ctx context.Context
	cancelFunc context.CancelFunc
	inflightMu sync.RWMutex
	inflight map[uint16]*inflightElement
	retryTimer *time.Timer
	retryTimerLock sync.Mutex
	mqueue *queue.Queue
	topicsMgr *topics.Manager
	sessionMgr *sessions.Manager
	session *sessions.Session
}

type subscription struct {
	client *client
	topic string
	qos byte
	share bool
	groupName string
}

func (c *client) init() {
	c.status = Connected
	c.info.localIP, _, _ = net.SplitHostPort(c.conn.LocalAddr().String())
	remoteAddr := c.conn.RemoteAddr()
	remoteNetwork := remoteAddr.Network()
	c.info.remoteIP = ""
	if remoteNetwork != "websocket" {
		c.info.remoteIP, _, _ = net.SplitHostPort(remoteAddr.String())
	} else {
		ws := c.conn.(*websocket.Conn)
		c.info.remoteIP, _, _ = net.SplitHostPort(ws.Request().RemoteAddr)
	}
	c.ctx, c.cancelFunc = context.WithCancel(context.Background())
}

func (c *client) readLoop() {
	nc := c.conn
	b := c.broker
	if nc == nil || b == nil {
		return
	}

	keepAlive := time.Second * time.Duration(c.info.keepalive)
	timeOut := keepAlive + (keepAlive / 2)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// add read timeout
			if keepAlive > 0 {
				if err := nc.SetReadDeadline(time.Now().Add(timeOut)); err != nil {
					log.Panic("set read timeout error")
					msg := &Message{
						client: c,
						packet: DisconnectedPacket,
					}
					b.SubmitWork(c.info.clientId, msg)
					return
				}
			}

			packet, err := packets.ReadPacket(nc)
			if err != nil {
				log.Error("read packet error")
				msg := &Message{
					client: c,
					packet: DisconnectedPacket,
				}
				b.SubmitWork(c.info.clientId, msg)
				return
			}

			// if packet is disconnect from client, then need to break read packet loop and clear will msg
			if _, isDisconnect := packet.(*packets.DisconnectPacket); isDisconnect {
				c.info.willMsg = nil
				c.cancelFunc()
			}

			msg := &Message{
				client: c,
				packet: packet,
			}
			b.SubmitWork(c.info.clientId, msg)
		}
	}
}

// extractPacketFields function reads a control packet and extracts only the fields
// that needs to pass on UTF-8 validation
func extractPacketFields(msgPacket packets.ControlPacket) []string {
	var fields [] string

	// Get packet type
	switch msgPacket.(type) {
	case *packets.ConnackPacket:
	case *packets.ConnectPacket:
	case *packets.PublishPacket:
		packet := msgPacket.(*packets.PublishPacket)
		fields = append(fields, packet.TopicName)
		break
	case *packets.SubscribePacket:
	case *packets.SubackPacket:
	case *packets.UnsubscribePacket:
		packet := msgPacket.(*packets.UnsubscribePacket)
		fields = append(fields, packet.Topics...)
		break
	}

	return fields
}

// validatePacketFields function checks if any of control packets fields has ill-formed
// UTF-8 string
func validatePacketFields(msgPacket packets.ControlPacket) (validFields bool) {
	// extract just fields that need validation
	fields := extractPacketFields(msgPacket)

	for _, field := range fields {
		// perform the basic UTF-8 validation
		if !utf8.ValidString(field) {
			validFields = false
			return
		}

		// A UTF-8 encoded string MUST NOT include an encoding of the null
		// character U+0000
		// If a receiver (Server or Client) receives a Control Packet containing U+0000
		// it MUST close the Network Connection
		// http://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.pdf page 14
		if bytes.ContainsAny([]byte(field), "\u0000") {
			validFields = false
			return
		}
	}

	// all fields have been validated successfully
	validFields = true

	return
}

func (c *client) WritePacket(packet packets.ControlPacket) error {
	defer func() {
		if err := recover(); err != nil {
			log.Error("recover error")
		}
	}()

	if c.status == Disconnected {
		return nil
	}

	if packet == nil {
		return nil
	}

	if c.conn == nil {
		c.Close()
		return errors.New("connect lost...")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return packet.Write(c.conn)
}

func (c *client) Close() {
	if c.status == Disconnected {
		return
	}

	c.cancelFunc()

	c.status = Disconnected

	// b := c.broker
	// publish bridge

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	// subs := c.subMap
}

func ProcessMessage(msg *Message) {
	c := msg.client
	ca := msg.packet
	if ca == nil {
		return
	}

	if c.typ == CLIENT {
		log.Debug("Recv message from clientId: %s", c.info.clientId)
	}

	// Perform field validation
	if !validatePacketFields(ca) {
		// http://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.pdf
		// Page 14
		//
		// If a Server or Client receives a Control Packet
		// containing ill-formed UTF-8 it MUST close the Network Connection
		c.conn.Close()

		log.Error("Client %s disconnected due to malformed packet", c.info.clientId)
		return
	}

	switch ca.(type) {
	case *packets.ConnackPacket:
	case *packets.ConnectPacket:
	case *packets.PublishPacket:
		packet := ca.(*packets.PublishPacket)
		c.ProcessPublish(packet)
	case *packets.PubackPacket:
		packet := ca.(*packets.PubackPacket)
		c.inflightMu.Lock()
		if _, found := c.inflight[packet.MessageID]; found {
			delete(c.inflight, packet.MessageID)
		} else {
			log.Error("Duplicated PUBACK PacketId")
		}
		c.inflightMu.Unlock()
	case *packets.PubrecPacket:
		packet := ca.(*packets.PubrecPacket)
		c.inflightMu.RLock()
		ielem, found := c.inflight[packet.MessageID]
		c.inflightMu.RUnlock()
		if found {
			if ielem.status == Publish {
				ielem.status = Pubrel
				ielem.timestamp = time.Now().Unix()
			} else if ielem.status == Pubrel {
				log.Error("Duplicated PUBREC PacketId")
			}
		} else {
			log.Error("The PUBREC PacketId is not found")
		}

		pubrel := packets.NewControlPacket(packets.Pubrel).(*packets.PubrelPacket)
		pubrel.MessageID = packet.MessageID
		if err := c.WritePacket(pubrel); err != nil {
			log.Error("send pubrel error")
			return
		}
	case *packets.PubrelPacket:
		packet := ca.(*packets.PubrelPacket)
		c.pubRel(packet.MessageID)
		pubcomp := packets.NewControlPacket(packets.Pubcomp).(*packets.PubcompPacket)
		pubcomp.MessageID = packet.MessageID
		if err := c.WritePacket(pubcomp); err != nil {
			log.Error("send pubcomp error")
			return
		}
	case *packets.PubcompPacket:
		packet := ca.(*packets.PubcompPacket)
		c.inflightMu.Lock()
		delete(c.inflight, packet.MessageID)
		c.inflightMu.Unlock()
	case *packets.SubscribePacket:
		packet := ca.(*packets.SubscribePacket)
		c.ProcessSubscribe(packet)
	case *packets.SubackPacket:
	case *packets.UnsubscribePacket:
		packet := ca.(*packets.UnsubscribePacket)
		c.ProcessUnsubscribe(packet)
	case *packets.UnsubackPacket:
	case *packets.PingreqPacket:
		c.ProcessPing()
	case *packets.PingrespPacket:
	case *packets.DisconnectPacket:
		c.Close()
	default:
		log.Info("Recv Unknown message...")
	}
}

func (c *client) ProcessPing() {
	if c.status == Disconnected {
		return
	}

	resp := packets.NewControlPacket(packets.Pingresp).(*packets.PingrespPacket)
	err := c.WritePacket(resp)
	if err != nil {
		log.Error("send PingResponse error")
		return
	}
}

func (c *client) isAwaitingFull() bool {
	if c.maxAwaitingRel == 0 {
		return false
	}
	if len(c.awaitingRel) < c.maxAwaitingRel {
		return false
	}
	return true
}

func (c *client) registerPublishPacketId(packetId uint16) error {
	if c.isAwaitingFull() {
		log.Error("Dropped qos2 packet for too many awaiting_rel")
		return errors.New("DROPPED_QOS2_PACKET_FOR_TOO_MANY_AWAITING_REL")
	}

	if _, found := c.awaitingRel[packetId]; found {
		return errors.New("RC_PACKET_IDENTIFIER_IN_USE")
	}
	c.awaitingRel[packetId] = time.Now().Unix()
	time.AfterFunc(time.Duration(awaitRelTimeout)*time.Second, c.expireAwaitingRel)
	return nil
}

func (c *client) expireAwaitingRel() {
	if len(c.awaitingRel) == 0 {
		return
	}

	now := time.Now().Unix()
	for packetId, Timestamp := range c.awaitingRel {
		if now - Timestamp >= awaitRelTimeout {
			log.Error("Dropped qos2 packet for await_rel_timeout")
			delete(c.awaitingRel, packetId)
		}
	}
}

func (c *client) pubRel(packetId uint16) error {
	if _, found := c.awaitingRel[packetId]; found {
		delete(c.awaitingRel, packetId)
	} else {
		log.Error("The PUBREL PacketId is not found")
		return errors.New("RC_PACKET_IDENTIFIER_NOT_FOUND")
	}
	return nil
}