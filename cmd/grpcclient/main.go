package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"
	"github.com/spark4862/net-sctp/pkg/common"
	"log"
	"time"
)

var (
	signalingServer string = "ws://localhost:28080/ws"
	roomID          string = "default"
	Dst             string = "grpc-client"
	Src             string = "grpc-server"
)

func parse() {
	flag.StringVar(&signalingServer, "server", "ws://localhost:28080/ws", "Signaling server WebSocket URL")
	flag.StringVar(&roomID, "room", "default", "Room ID")
	flag.StringVar(&Src, "src", "grpc-client", "Src")
	flag.StringVar(&Dst, "dst", "grpc-server", "Dst")
	flag.Parse()
}

//func check_direct_connection(connectedSignalCh chan bool) bool {
//	signalingURL := fmt.Sprintf("%s?room=%s", signalingServer, "cdc")
//	conn, _, err := websocket.DefaultDialer.Dial(signalingURL, nil)
//	if err != nil {
//		log.Fatal("Error connecting to signaling server:", err)
//	}
//	defer func(conn *websocket.Conn) {
//		err := conn.Close()
//		if err != nil {
//			log.Println(err)
//		}
//	}(conn)
//
//	config := webrtc.Configuration{
//		ICEServers: []webrtc.ICEServer{
//			{
//				URLs: []string{"stun:8.153.200.135:3479"},
//			},
//		},
//	}
//
//	loggerFactory := logging.NewDefaultLoggerFactory()
//	loggerFactory.DefaultLogLevel = logging.LogLevelTrace
//
//	s := webrtc.SettingEngine{}
//	s.LoggerFactory = loggerFactory
//	s.SetICETimeouts(5*time.Second, 5*time.Second, 5*time.Second)
//
//	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))
//	peerConnection, err := api.NewPeerConnection(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
//		log.Printf("OnICEConnectionStateChange: %s\n", connectionState.String())
//		if connectionState == webrtc.ICEConnectionStateConnected {
//			connectedSignalCh <- true
//		}
//		if connectionState == webrtc.ICEConnectionStateFailed {
//			connectedSignalCh <- false
//		}
//	})
//
//	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
//		log.Printf("OnConnectionStateChange: %s\n", s.String())
//	})
//
//	peerConnection.OnSignalingStateChange(func(s webrtc.SignalingState) {
//		log.Printf("OnSignalingStateChange: %s\n", s.String())
//	})
//
//	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
//		if i == nil {
//			return
//		}
//
//		candidateString, err := json.Marshal(i.ToJSON())
//		if err != nil {
//			log.Println(err)
//			return
//		}
//		targetedMsg := common.TargetedMsg{
//			Src:  Src,
//			Dst:  Dst,
//			Data: string(candidateString),
//		}
//		targetedString, err := json.Marshal(targetedMsg)
//		if err != nil {
//			log.Println(err)
//			return
//		}
//		if writeErr := conn.WriteJSON(&common.SignalMsg{
//			Type: common.Candidate,
//			Data: string(targetedString),
//		}); writeErr != nil {
//			log.Println(writeErr)
//		}
//	})
//
//	go func() {
//		for {
//			_, rawMsg, err := conn.ReadMessage()
//			if err != nil {
//				log.Println("Error reading message:", err)
//				return
//			}
//			log.Println("recv msg from signaling server")
//
//			var msg common.SignalMsg
//			if err := json.Unmarshal(rawMsg, &msg); err != nil {
//				log.Println("Error parsing message:", err)
//				continue
//			}
//			log.Println("msg is", msg)
//
//			switch msg.Type {
//			case common.Answer:
//				log.Println("recv a answer msg")
//				fromTargetedMsg := common.TargetedMsg{}
//				if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); err != nil {
//					log.Println("Error parsing offer:", err)
//					continue
//				}
//				answer := webrtc.SessionDescription{}
//				if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &answer); err != nil {
//					log.Println("Error parsing offer:", err)
//					continue
//				}
//
//				if err := peerConnection.SetRemoteDescription(answer); err != nil {
//					log.Println("Error setting remote description:", err)
//					continue
//				}
//
//				answer, err := peerConnection.CreateAnswer(nil)
//				if err != nil {
//					log.Println("Error creating answer:", err)
//					continue
//				}
//
//				if err := peerConnection.SetRemoteDescription(answer); err != nil {
//					log.Println("Error setting remote description:", err)
//				}
//				log.Println("set remote desc for answer ok")
//			case common.Offer:
//				log.Println("[error] grpc server should never receive ice answer")
//				continue
//			case common.Candidate:
//				fromTargetedMsg := common.TargetedMsg{}
//				if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); err != nil {
//					log.Println("Error parsing candidate:", err)
//					continue
//				}
//
//				candidate := webrtc.ICECandidateInit{}
//				if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &candidate); err != nil {
//					log.Println("Error parsing candidate:", err)
//					continue
//				}
//
//				if err := peerConnection.AddICECandidate(candidate); err != nil {
//					log.Println("Error adding ICE candidate:", err)
//					continue
//				}
//
//				log.Println("add ice candidate:", candidate)
//			default:
//				panic("unhandled default case")
//			}
//		}
//	}()
//
//	registerString, err := json.Marshal(common.RegisterMsg{Id: Src})
//	if err != nil {
//		log.Fatal(err)
//	}
//	if err := conn.WriteJSON(&common.SignalMsg{
//		Type: common.Register,
//		Data: string(registerString),
//	}); err != nil {
//		log.Fatal(err)
//	}
//
//	offer, err := peerConnection.CreateOffer(nil)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	if err := peerConnection.SetLocalDescription(offer); err != nil {
//		log.Fatal(err)
//	}
//
//	offerString, err := json.Marshal(offer)
//	if err != nil {
//		log.Fatal(err)
//	}
//	targetedMsg := common.TargetedMsg{
//		Src:  Src,
//		Dst:  Dst,
//		Data: string(offerString),
//	}
//	targetedString, err := json.Marshal(targetedMsg)
//	if err != nil {
//		log.Fatal(err)
//	}
//	if err := conn.WriteJSON(&common.SignalMsg{
//		Type: common.Offer,
//		Data: string(targetedString),
//	}); err != nil {
//		log.Fatal(err)
//	}
//
//	log.Println("send offer to signaling server ok")
//	tmp := <-connectedSignalCh
//	if tmp {
//		return true
//	} else {
//		return false
//	}
//}

func connectSignalingServer(pSignalingServer string, pRoomID string) *websocket.Conn {
	url := fmt.Sprintf("%s?room=%s", pSignalingServer, pRoomID)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		err = fmt.Errorf("connectSignalingServer err when Dial: %w", err)
		log.Fatal(err)
	}
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
		case common.Answer:
			log.Println("recv a answer msg")
			fromTargetedMsg := common.TargetedMsg{}
			if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); err != nil {
				err = fmt.Errorf("messageHandler err when Unmarshal1: %w", err)
				log.Println(err)
				continue
			}
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &answer); err != nil {
				err = fmt.Errorf("messageHandler err when Unmarshal2: %w", err)
				log.Println(err)
				continue
			}

			if err := p.SetRemoteDescription(answer); err != nil {
				err = fmt.Errorf("messageHandler err when SetRemoteDescription: %w", err)
				log.Println(err)
				continue
			}

			log.Println("set remote desc for answer ok")
		case common.Offer:
			err := fmt.Errorf("messageHandler err: should never receive offer")
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

func newAndSetOffer(peerConnection *webrtc.PeerConnection) webrtc.SessionDescription {
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		err = fmt.Errorf("newAndSetOffer err when CreateOffer: %w", err)
		log.Fatal(err)
	}

	if err := peerConnection.SetLocalDescription(offer); err != nil {
		err = fmt.Errorf("newAndSetOffer err when SetLocalDescription: %w", err)
		log.Fatal(err)
	}
	return offer
}

func sendOffer(o webrtc.SessionDescription, c *websocket.Conn, src string, dst string) {
	offerString, err := json.Marshal(o)
	if err != nil {
		err = fmt.Errorf("sendOffer err when Marshal0: %w", err)
		log.Fatal(err)
	}
	targetedMsg := common.TargetedMsg{
		Src:  src,
		Dst:  dst,
		Data: string(offerString),
	}
	targetedString, err := json.Marshal(targetedMsg)
	if err != nil {
		err = fmt.Errorf("sendOffer err when Marshal1: %w", err)
		log.Fatal(err)
	}
	if err := c.WriteJSON(&common.SignalMsg{
		Type: common.Offer,
		Data: string(targetedString),
	}); err != nil {
		err = fmt.Errorf("sendOffer err when WriteJSON: %w", err)
		log.Fatal(err)
	}
}

func init() {
	parse()
}
func main() {
	conn := connectSignalingServer(signalingServer, roomID)
	defer closeConnection(conn)
	log.Println("connected to signaling server")

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

	s := newSettingEngine(logging.LogLevelTrace)
	//s.SetNetworkTypes([]webrtc.NetworkType{
	//	webrtc.NetworkTypeTCP4,
	//	webrtc.NetworkTypeTCP6,
	//})

	peerConnection := newPeerConnection(s, config)

	dataChannel, err := peerConnection.CreateDataChannel(Src, nil)
	if err != nil {
		log.Fatal(err)
	}
	dataChannel.OnOpen(newDataChannelOnOpen(dataChannel, roomID))

	peerConnection.OnICEConnectionStateChange(newPeerConnectionOnICEConnectionStateChange())
	peerConnection.OnConnectionStateChange(newPeerConnectionOnConnectionStateChange())
	peerConnection.OnSignalingStateChange(newPeerConnectionOnSignalingStateChange())
	peerConnection.OnDataChannel(newPeerConnectionOnDataChannel())
	peerConnection.OnICECandidate(newPeerConnectionOnICECandidate(conn, Src, Dst))

	go messageHandler(conn, peerConnection)

	sendRegister(conn, Src)

	offer := newAndSetOffer(peerConnection)

	sendOffer(offer, conn, Src, Dst)
	log.Println("send offer to signaling server ok")
	select {}
}
