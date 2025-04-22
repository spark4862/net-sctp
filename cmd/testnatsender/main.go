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
	isListener  bool
	liveTime    int
)

func parse() {
	flag.StringVar(source, "src", "c1", "name to register")
	flag.StringVar(destination, "dst", "c2", "name to call")
	flag.BoolVar(&isSender, "isSender", false, "is sender")
	flag.BoolVar(&isListener, "isListener", false, "is listener")
	flag.IntVar(&liveTime, "t", 500000, "live time")
	flag.Parse()
}

func init() {
	parse()
}

func main() {
	natSender := sender.NewNatSender(*source)

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
				natSender.Send(*destination, []byte("hello"))
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
// ./testnatsender -src c2 -dst c1 -isSender -t 20
