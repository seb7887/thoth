package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"

	"github.com/seb7887/thoth/broker"
	"github.com/seb7887/thoth/config"
	log "github.com/sirupsen/logrus"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	config := config.GetConfig()
	b, err := broker.NewBroker(config)
	if err != nil {
		log.Fatal("New broker error")
	}
	b.Start()

	defer b.Stop()
	waitForSignal()
	fmt.Println("Signal received, broker closed")
}

func waitForSignal() os.Signal {
	signalChan := make(chan os.Signal, 1)
	defer close(signalChan)
	signal.Notify(signalChan, os.Kill, os.Interrupt)
	s := <-signalChan
	signal.Stop(signalChan)
	return s
}
