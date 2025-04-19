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
	// 实现有点问题，应该吧

	// 把dispatcher放到listen里面了，必须执行Listen
	natSender.Listen()
	go func() {
		for {
			natSender.Accept()
		}
	}()

	if isSender {
		for {
			natSender.Send(*destination, "hello")
			time.Sleep(3 * time.Second)
		}
	}
	select {}
}

// go run . -src c1
// ./testnatsender -src c2 -dst c1 -isSender
