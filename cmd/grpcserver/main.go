// Deprecated: use grpcclient without -isClient

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"
	"github.com/spark4862/sender/pkg/common"
	"log"
	"time"
)

var (
	signalingServer string
	roomID          string
	destination     string
	source          string
	isClient        bool = false
)

func parse() {
	flag.StringVar(&signalingServer, "server", "ws://localhost:28080/ws", "Signaling server WebSocket URL")
	flag.StringVar(&roomID, "room", "default", "Room ID (leave empty to create a new room)")
	flag.StringVar(&source, "src", "grpc-server", "Src")
	flag.Parse()
}

func connectSignalingServer(pSignalingServer string, pRoomID string) *websocket.Conn {
	url := fmt.Sprintf("%s?room=%s", pSignalingServer, pRoomID)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		err = fmt.Errorf("connectSignalingServer err when Dial: %w", err)
		log.Fatal(err)
	}
	log.Println("connected to signaling server")
	return conn
}

func closeConnection(c *websocket.Conn) {
	err := c.Close()
	if err != nil {
		err = fmt.Errorf("closeConnection err when Close: %w", err)
		log.Println(err)
	}
}

func newSettingEngine(l logging.LogLevel) webrtc.SettingEngine {
	s := webrtc.SettingEngine{}

	loggerFactory := logging.NewDefaultLoggerFactory()
	loggerFactory.DefaultLogLevel = l
	s.LoggerFactory = loggerFactory

	s.SetICETimeouts(5*time.Second, 5*time.Second, 5*time.Second)
	return s
}

func newPeerConnection(s webrtc.SettingEngine, cfg webrtc.Configuration) *webrtc.PeerConnection {
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))
	peerConnection, err := api.NewPeerConnection(cfg)
	if err != nil {
		err = fmt.Errorf("newPeerConnection err when NewPeerConnection: %w", err)
		log.Println(err)
	}
	return peerConnection
}

func newDataChannelOnOpen(dc *webrtc.DataChannel, pRoomID string) func() {
	return func() {
		log.Println("OnOpen" + dc.Label())
		go func() {
			for {
				err := dc.SendText(dc.Label() + " send Hello from " + pRoomID)
				if err != nil {
					err = fmt.Errorf("newDataChannelOnOpen err when SendText: %w", err)
					log.Println(err)
				}
				time.Sleep(5 * time.Second)
			}
		}()
	}
}

func newPeerConnectionOnICEConnectionStateChange() func(connectionState webrtc.ICEConnectionState) {
	return func(connectionState webrtc.ICEConnectionState) {
		log.Printf("OnICEConnectionStateChange: %s\n", connectionState.String())
	}
}

func newPeerConnectionOnConnectionStateChange() func(s webrtc.PeerConnectionState) {
	return func(s webrtc.PeerConnectionState) {
		log.Printf("OnConnectionStateChange: %s\n", s.String())
	}
}

func newPeerConnectionOnSignalingStateChange() func(s webrtc.SignalingState) {
	return func(s webrtc.SignalingState) {
		log.Printf("OnSignalingStateChange: %s\n", s.String())
	}
}

func newPeerConnectionOnDataChannel() func(*webrtc.DataChannel) {
	return func(dc *webrtc.DataChannel) {
		log.Printf("OnDataChannel %s %d\n", dc.Label(), dc.ID())

		dc.OnOpen(func() {
			log.Printf("OnOpen '%s'-'%d'\n", dc.Label(), dc.ID())
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			log.Printf("OnMessage '%s': '%s'\n", dc.Label(), string(msg.Data))
		})
	}
}

func newPeerConnectionOnICECandidate(c *websocket.Conn, src string, dst string) func(i *webrtc.ICECandidate) {
	return func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			err = fmt.Errorf("newPeerConnectionOnICECandidate err when Marshal0: %w", err)
			log.Println(err)
			return
		}
		targetedMsg := common.TargetedMsg{
			Src:  src,
			Dst:  dst,
			Data: string(candidateString),
		}
		targetedString, err := json.Marshal(targetedMsg)
		if err != nil {
			err = fmt.Errorf("newPeerConnectionOnICECandidate err when Marshal1: %w", err)
			log.Println(err)
			return
		}
		if writeErr := c.WriteJSON(&common.SignalMsg{
			Type: common.Candidate,
			Data: string(targetedString),
		}); writeErr != nil {
			err = fmt.Errorf("newPeerConnectionOnICECandidate err when WriteJSON: %w", err)
			log.Println(err)
		}
	}
}

//	func isConnectionFinished(p *webrtc.PeerConnection) bool {
//		status := p.ConnectionState()
//		if status >= webrtc.PeerConnectionStateConnected {
//			return false
//		}
//		return true
//	}

func messageHandler(c *websocket.Conn, p *webrtc.PeerConnection) {
	for {

		_, rawMsg, err := c.ReadMessage()
		if err != nil {
			err = fmt.Errorf("messageHandler err when ReadMessage: %w", err)
			log.Println(err)
			return
		}
		log.Println("recv msg from signaling server")

		var msg common.SignalMsg
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			err = fmt.Errorf("messageHandler err when Unmarshal0: %w", err)
			log.Println(err)
			continue
		}
		//log.Println("msg is", msg)

		switch msg.Type {
		case common.Offer:
			log.Println("recv a offer msg")
			fromTargetedMsg := common.TargetedMsg{}
			if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); err != nil {
				err = fmt.Errorf("messageHandler err when Unmarshal1: %w", err)
				log.Println(err)
				continue
			}
			destination = fromTargetedMsg.Src

			offer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &offer); err != nil {
				err = fmt.Errorf("messageHandler err when Unmarshal2: %w", err)
				log.Println(err)
				continue
			}

			if err := p.SetRemoteDescription(offer); err != nil {
				err = fmt.Errorf("messageHandler err when SetRemoteDescription: %w", err)
				log.Println(err)
				continue
			}

			answer, err := p.CreateAnswer(nil)
			if err != nil {
				err = fmt.Errorf("messageHandler err when CreateAnswer: %w", err)
				log.Println(err)
				continue
			}

			if err := p.SetLocalDescription(answer); err != nil {
				err = fmt.Errorf("messageHandler err when SetLocalDescription: %w", err)
				log.Println(err)
				continue
			}

			answerString, err := json.Marshal(answer)
			if err != nil {
				err = fmt.Errorf("messageHandler err when Marshal0: %w", err)
				log.Println(err)
				continue
			}

			toTargetedMsg := common.TargetedMsg{
				Src:  source,
				Dst:  destination,
				Data: string(answerString),
			}
			targetedString, err := json.Marshal(toTargetedMsg)
			if err != nil {
				err = fmt.Errorf("messageHandler err when Marshal1: %w", err)
				log.Println(err)
				continue
			}

			if err := c.WriteJSON(&common.SignalMsg{
				Type: common.Answer,
				Data: string(targetedString),
			}); err != nil {
				err = fmt.Errorf("messageHandler err when WriteJSON: %w", err)
				log.Println(err)
			}
			log.Println("send answer ok")
		case common.Answer:
			err := fmt.Errorf("messageHandler err: should never receive answer")
			log.Println(err)
			continue
		case common.Candidate:
			fromTargetedMsg := common.TargetedMsg{}
			if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); err != nil {
				err = fmt.Errorf("messageHandler err when Unmarshal3: %w", err)
				log.Println(err)
				continue
			}

			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &candidate); err != nil {
				err = fmt.Errorf("messageHandler err when Unmarshal4: %w", err)
				log.Println(err)
				continue
			}

			if err := p.AddICECandidate(candidate); err != nil {
				err = fmt.Errorf("messageHandler err when AddICECandidate: %w", err)
				log.Println(err)
				continue
			}

			log.Println("add ice candidate:", candidate)
		default:
			panic("unhandled default case")
		}
	}
}

func sendRegister(c *websocket.Conn, id string) {
	registerString, err := json.Marshal(common.RegisterMsg{Id: id})
	if err != nil {
		err = fmt.Errorf("sendRegister err when Marshal: %w", err)
		log.Fatal(err)
	}
	if err := c.WriteJSON(&common.SignalMsg{
		Type: common.Register,
		Data: string(registerString),
	}); err != nil {
		err = fmt.Errorf("sendRegister err when WriteJSON: %w", err)
		log.Fatal(err)
	}
}
func setPeerConnection(p *webrtc.PeerConnection, c *websocket.Conn, src string, dst string) {
	p.OnICEConnectionStateChange(newPeerConnectionOnICEConnectionStateChange())
	p.OnConnectionStateChange(newPeerConnectionOnConnectionStateChange())
	p.OnSignalingStateChange(newPeerConnectionOnSignalingStateChange())
	p.OnDataChannel(newPeerConnectionOnDataChannel())
	p.OnICECandidate(newPeerConnectionOnICECandidate(c, src, dst))
}

func checkDirectConnection() bool {
	rID := "checkDirectConnection"
	connectedSignalCh := make(chan bool)

	conn := connectSignalingServer(signalingServer, rID)
	defer closeConnection(conn)

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:8.153.200.135:3479"},
			},
		},
	}

	s := newSettingEngine(logging.LogLevelInfo)

	peerConnection := newPeerConnection(s, config)
	setPeerConnection(peerConnection, conn, source, destination)
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("OnICEConnectionStateChange: %s\n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			connectedSignalCh <- true
		}
		if connectionState == webrtc.ICEConnectionStateFailed {
			connectedSignalCh <- false
		}
	})

	dataChannel, err := peerConnection.CreateDataChannel(source, nil)
	if err != nil {
		log.Fatal(err)
	}
	dataChannel.OnOpen(newDataChannelOnOpen(dataChannel, rID))

	go messageHandler(conn, peerConnection)

	sendRegister(conn, rID)

	var tmp bool
	select {
	case tmp = <-connectedSignalCh:
		//case <-time.After(time.Second * 20):
		//	log.Println("checkDirectConnection timeout")
		//	return false
	}

	if tmp {
		return true
	} else {
		return false
	}
}

func init() {
	parse()
}

func main() {
	conn := connectSignalingServer(signalingServer, roomID)
	defer closeConnection(conn)

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			//{
			//	URLs: []string{"stun:8.153.200.135:3479"},
			//},
			//{
			//	URLs: []string{"stun:stun.l.google.com:19302"},
			//},
			{
				URLs:       []string{"turn:8.153.200.135:3479"},
				Username:   "test1",
				Credential: "123456",
			},
		},
	}

	s := newSettingEngine(logging.LogLevelInfo)
	//s.SetNetworkTypes([]webrtc.NetworkType{
	//	webrtc.NetworkTypeTCP4,
	//	webrtc.NetworkTypeTCP6,
	//})

	peerConnection := newPeerConnection(s, config)
	setPeerConnection(peerConnection, conn, source, destination)

	dataChannel, err := peerConnection.CreateDataChannel(source, nil)
	if err != nil {
		log.Fatal(err)
	}
	dataChannel.OnOpen(newDataChannelOnOpen(dataChannel, roomID))

	go messageHandler(conn, peerConnection)

	sendRegister(conn, source)

	select {}
}
