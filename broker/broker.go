package broker

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
	"github.com/seb7887/thoth/auth"
	"github.com/seb7887/thoth/broker/persistence/sessions"
	"github.com/seb7887/thoth/broker/persistence/topics"
	"github.com/seb7887/thoth/config"
	"github.com/seb7887/thoth/pool"
	"github.com/seb7887/thoth/utils"
	log "github.com/sirupsen/logrus"
)

const (
	MessagePoolNum = 1024
	MessagePoolMessageNum = 1024
)

type Message struct {
	client *client
	packet packets.ControlPacket
}

type Broker struct {
	id string
	mu sync.Mutex
	config *config.Configuration
	wpool *pool.WorkerPool
	clients sync.Map
	routes sync.Map
	remotes sync.Map
	nodes map[string]interface{}
	clusterPool chan *Message
	sessionMgr *sessions.Manager
	topicsMgr *topics.Manager
	auth auth.Auth
}

func newMessagePool() []chan *Message {
	pool := make([]chan *Message, 0)
	for i := 0; i < MessagePoolNum; i++ {
		ch := make(chan *Message, MessagePoolMessageNum)
		pool = append(pool, ch)
	}
	return pool
}

func NewBroker(config *config.Configuration) (*Broker, error) {
	b := &Broker{
		id: utils.GenUniqueId(),
		clusterPool: make(chan *Message),
		config: config,
		wpool: pool.New(config.Worker),
		nodes: make(map[string]interface{}),
	}

	var err error
	b.topicsMgr, err = topics.NewManager("mem")
	if err != nil {
		log.Error("new topic manager error")
		return nil, err
	}

	b.sessionMgr, err = sessions.NewManager("mem")
	if err != nil {
		log.Error("new session manager error")
		return nil, err
	}

	bAuth, err := auth.NewAuth()
	if err != nil {
		log.Error("auth config error")
		return nil, err
	}
	b.auth = bAuth

	return b, nil
}

func (b *Broker) SubmitWork(clientId string, msg *Message) {
	if b.wpool == nil {
		b.wpool = pool.New(b.config.Worker)
	}

	if msg.client.typ == CLUSTER {
		b.clusterPool <- msg
	} else {
		b.wpool.Submit(clientId, func() {
			ProcessMessage(msg)
		})
	}
}

func (b *Broker) Start() {
	if b == nil {
		log.Fatal("No Broker")
		return
	}
	go b.StartClientListening()
}

func (b *Broker) Stop() {
	b.wpool.Stop()
}

func (b *Broker) StartClientListening() {
	var l net.Listener
	var err error
	for {
		address := fmt.Sprintf(":%d", b.config.Port)
		l, err = net.Listen("tcp", address)
		if err != nil {
			log.Fatal("Error initializating broker")
			return
		}
		log.Printf("Broker listening on port %d", b.config.Port)
		break
	}
	tmpDelay := 10 * ACCEPT_MIN_SLEEP
	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				log.Error("Temporary client accept error, sleeping")
				time.Sleep(tmpDelay)
				tmpDelay *= 2
				if tmpDelay > ACCEPT_MAX_SLEEP {
					tmpDelay = ACCEPT_MAX_SLEEP
				}
			} else {
				log.Error("accept error")
			}
			continue
		}
		tmpDelay = ACCEPT_MIN_SLEEP
		go b.handleConnection(CLIENT, conn)
	}
}

func (b *Broker) handleConnection(typ int, conn net.Conn) {
	// Process connect packet
	packet, err := packets.ReadPacket(conn)
	if err != nil {
		log.Fatal("read connect packet error")
		return
	}
	if packet == nil {
		log.Println("received nil packet")
		return
	}
	msg, ok := packet.(*packets.ConnectPacket)
	if !ok {
		log.Fatal("received message that was not connect")
		return
	}

	connack := packets.NewControlPacket(packets.Connack).(*packets.ConnackPacket)
	connack.SessionPresent = msg.CleanSession
	connack.ReturnCode = msg.Validate()

	if connack.ReturnCode != packets.Accepted {
		err = connack.Write(conn)
		if err != nil {
			log.Fatal("send connack error")
			return
		}
		return
	}

	// Authenticate
	if typ == CLIENT && !b.auth.CheckConnect(string(msg.Username), string(msg.Password)) {
		connack.ReturnCode = packets.ErrRefusedBadUsernameOrPassword
		err = connack.Write(conn)
		if err != nil {
			log.Error("send connack error")
			return
		}
		return
	}

	err = connack.Write(conn)
	if err != nil {
		log.Fatal("send connack error")
		return
	}

	log.Printf("clientID: '%s' connected", msg.ClientIdentifier)

	willMsg := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	if msg.WillFlag {
		willMsg.Qos = msg.WillQos
		willMsg.TopicName = msg.WillTopic
		willMsg.Retain = msg.WillRetain
		willMsg.Payload = msg.WillMessage
		willMsg.Dup = msg.Dup
	} else {
		willMsg = nil
	}
	
	info := info{
		clientId: msg.ClientIdentifier,
		username: msg.Username,
		password: string(msg.Password),
		keepalive: msg.Keepalive,
		willMsg: willMsg,
	}

	c := &client{
		typ: typ,
		broker: b,
		conn: conn,
		info: info,
	}

	c.init()

	err = b.getSession(c, msg, connack)
	if err != nil {
		log.Error("get session error")
		return
	}

	cid := c.info.clientId

	var exist bool
	var old interface{}

	switch typ {
	case CLIENT:
		old, exist = b.clients.Load(cid)
		if exist {
			log.Warn("client exists, close old...")
			ol, ok := old.(*client)
			if ok {
				ol.Close()
			}
		}
		b.OnlineOfflineNotification(cid, true)
	case ROUTER:
		old, exist = b.routes.Load(cid)
		if exist {
			log.Warn("router exist, close old...")
			ol, ok := old.(*client)
			if ok {
				ol.Close()
			}
		}
		b.routes.Store(cid, c)
	}

	c.readLoop()
}

func (b *Broker) PublishMessage(packet *packets.PublishPacket) {
	var subs []interface{}
	var qoss []byte
	b.mu.Lock()
	err := b.topicsMgr.Subscribers([]byte(packet.TopicName), packet.Qos, &subs, &qoss)
	b.mu.Unlock()
	if err != nil {
		log.Error("search sub client error")
		return
	}
	for _, sub := range subs {
		s, ok := sub.(*subscription)
		if ok {
			err := s.client.WritePacket(packet)
			if err != nil {
				log.Error("write message error...")
			}
		}
	}
}

func (b *Broker) OnlineOfflineNotification(clientId string, online bool) {
	packet := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	packet.TopicName = "/" + clientId
	packet.Qos = 0
	packet.Payload = []byte(fmt.Sprintf(`{"clientId": "%s", "online": %v, "timestamp": "%s"`, clientId, online, time.Now().UTC().Format(time.RFC3339)))

	b.PublishMessage(packet)
}

func (b *Broker) BroadcastSubOrUnsubMessage(packet packets.ControlPacket) {
	b.routes.Range(func(key, value interface{}) bool {
		r, ok := value.(*client)
		if ok {
			r.WritePacket(packet)
		}
		return true
	})
}

func (b *Broker) CheckRemoteExist(remoteID, url string) bool {
	exist := false
	b.remotes.Range(func(key, value interface{}) bool {
		v, ok := value.(*client)
		if ok {
			if v.route.remoteUrl == url {
				v.route.remoteID = remoteID
				exist = true
				return false
			}
		}
		return true
	})
	return exist
}