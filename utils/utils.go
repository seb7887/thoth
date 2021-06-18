package utils

import (
	uuid "github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

func GenUniqueId() string {
	id, err := uuid.NewRandom()
	if err != nil {
		log.Error("uuid.NewRandom() returned an error")
	}
	return id.String()
}

func AddSubMap(m map[string]uint64, topic string) {
	subNum, exist := m[topic]
	if exist {
		m[topic] = subNum + 1
	} else {
		m[topic] = 1
	}
}

func DelSubMap(m map[string]uint64, topic string) uint64 {
	subNum, exist := m[topic]
	if exist {
		if subNum > 1 {
			m[topic] = subNum - 1
			return subNum - 1
		}
	} else {
		m[topic] = 0
	}
	return 0
}
