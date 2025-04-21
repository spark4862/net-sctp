package main

import (
	"flag"
	"github.com/spark4862/sender/pkg/sender"
	"time"
)

var (
	destination *string = new(string)
	source      *string = new(string)
	isSender    bool
)

func parse() {
	flag.StringVar(source, "src", "c1", "name to register")
	flag.StringVar(destination, "dst", "c2", "name to call")
	flag.BoolVar(&isSender, "isSender", false, "is sender")
	flag.Parse()
}

func init() {
	parse()
}

func main() {
	natSender := sender.NewNatSender(*source)

	if !isSender {
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
				natSender.Send(*destination, "hello")
				select {
				case <-natSender.Ctx.Done():
					return
				default:
				}
			}
		}()
	}
	var t time.Duration
	if isSender {
		t = 20
	} else {
		t = 10
	}
	<-time.After(t * time.Second)
	//if !isSender {
	//	a, _ := natSender.Dst2pc.Get(*destination)
	//	a.Close()
	//}
	natSender.Close()
	return
	//select {}
}

// go run . -src c1
// ./testnatsender -src c2 -dst c1 -isSender
