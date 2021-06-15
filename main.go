package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"

	"github.com/seb7887/thoth/broker"
	"github.com/seb7887/thoth/config"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	config := config.GetConfig()
	b, err := broker.NewBroker(config)
	log.Println(b)
	if err != nil {
		log.Fatal("New broker error")
	}
	b.Start()

	waitForSignal()
	fmt.Println("Signal received, broker closed")
}

func waitForSignal() os.Signal {
	signalChan := make(chan os.Signal, 1)
	defer close(signalChan)
	signal.Notify(signalChan, os.Kill, os.Interrupt)
	s := <- signalChan
	signal.Stop(signalChan)
	return s
}
