package main

import (
	"flag"
	"github.com/pion/logging"
	"github.com/spark4862/sender/pkg/sender"
	"log"
	"strings"
	"time"
)

var (
	destination     *string = new(string)
	source          *string = new(string)
	isSender        bool
	isListener      bool
	liveTime        int
	signalingServer *string = new(string)
)

func parse() {
	flag.StringVar(source, "src", "c1", "name to register")
	flag.StringVar(destination, "dst", "c2", "name to call")
	flag.BoolVar(&isSender, "isSender", false, "is sender")
	flag.BoolVar(&isListener, "isListener", false, "is listener")
	flag.IntVar(&liveTime, "t", 500000, "time to live")
	flag.StringVar(signalingServer, "server", "ws://8.153.200.135:28080/ws", "Signaling server WebSocket URL")
	flag.Parse()
}

func init() {
	parse()
}

func main() {
	dsts := strings.Split(*destination, ":")
	natSender := sender.NewNatSender(*source, *signalingServer, func(msg []byte) { log.Print(string(msg)) }, logging.LogLevelWarn)

	if isListener {
		natSender.Listen()
		go func() {
			for {
				select {
				case <-natSender.Ctx.Done():
					return
				default:
				}
				natSender.Accept()
			}

		}()
	}

	if isSender {
		go func() {
			for {
				for _, dst := range dsts {
					natSender.Send(dst, []byte("hello"))
				}
				select {
				case <-natSender.Ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}()
	}
	<-time.After(time.Duration(liveTime) * time.Second)
	natSender.Close()
	return
	//select {}
}

// go run . -src c1 -isListener -t 20
// ./testnatsender -src c2 -dst c1 -isSender -isListener -t 20
