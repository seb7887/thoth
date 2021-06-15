package broker

import (
	"time"
)

const (
	ACCEPT_MIN_SLEEP = 100 * time.Millisecond // min acceptable sleep time on temporary erros
	ACCEPT_MAX_SLEEP = 10 * time.Second // max acceptable sleep time on temporary errors
	DEFAULT_ROUTE_CONNECT = 5 * time.Second // route solicitation intervals
	DEFAULT_TLS_TIMEOUT = 5 * time.Second
)

const (
	CONNECT = uint8(iota + 1)
	CONNACK
	PUBLISH
	PUBACK
	PUBREC
	PUBREL
	PUBCOMP
	SUBSCRIBE
	SUBACK
	UNSUBSCRIBE
	UNSUBACK
	PINGREQ
	PINGRESP
	DISCONNECT
)

const (
	QosAtMostOnce byte = iota
	QosAtLeastOnce
	QosExactlyOnce
	QosFailure = 0x80
)