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
	signalingServer string
	roomID          string
	Dst             string
	Src             string
)

func parse() {
	flag.StringVar(&signalingServer, "server", "ws://localhost:28080/ws", "Signaling server WebSocket URL")
	flag.StringVar(&roomID, "room", "default", "Room ID (leave empty to create a new room)")
	flag.StringVar(&Src, "src", "grpc-server", "Src")
	flag.Parse()
}

func main() {
	parse()

	signalingURL := fmt.Sprintf("%s?room=%s", signalingServer, roomID)
	conn, _, err := websocket.DefaultDialer.Dial(signalingURL, nil)
	if err != nil {
		log.Fatal("Error connecting to signaling server:", err)
	}
	defer func(conn *websocket.Conn) {
		err := conn.Close()
		if err != nil {
			log.Println(err)
		}
	}(conn)
	log.Println("connected to signaling server")

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs:       []string{"turn:8.153.200.135:3479"},
				Username:   "test1",
				Credential: "123456",
			},
		},
	}

	loggerFactory := logging.NewDefaultLoggerFactory()
	loggerFactory.DefaultLogLevel = logging.LogLevelTrace

	s := webrtc.SettingEngine{}
	s.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeTCP4,
		webrtc.NetworkTypeTCP6,
	})
	s.LoggerFactory = loggerFactory
	s.SetICETimeouts(5*time.Second, 5*time.Second, 5*time.Second)

	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		log.Fatal(err)
	}

	dataChannel, err := peerConnection.CreateDataChannel("test", nil)
	if err != nil {
		log.Fatal(err)
	}

	dataChannel.OnOpen(func() {
		log.Println("Data channel is open")
		go func() {
			for {
				err := dataChannel.SendText("Hello from " + roomID)
				if err != nil {
					log.Println(err)
				}
				time.Sleep(5 * time.Second)
			}
		}()
	})

	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("Received message: %s\n", string(msg.Data))
	})

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})

	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("Peer Connection State has changed: %s\n", s.String())
	})

	peerConnection.OnSignalingStateChange(func(s webrtc.SignalingState) {
		log.Printf("Signaling State has changed: %s\n", s.String())
	})

	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		log.Printf("New DataChannel %s %d\n", d.Label(), d.ID())

		d.OnOpen(func() {
			log.Printf("Data channel '%s'-'%d' open.\n", d.Label(), d.ID())
		})

		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			log.Printf("Message from DataChannel '%s': '%s'\n", d.Label(), string(msg.Data))
		})
	})

	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println(err)
			return
		}
		targetedMsg := common.TargetedMsg{
			Src:  Src,
			Dst:  Dst,
			Data: string(candidateString),
		}
		targetedString, err := json.Marshal(targetedMsg)
		if err != nil {
			log.Println(err)
			return
		}
		if writeErr := conn.WriteJSON(&common.SignalMsg{
			Type: common.Candidate,
			Data: string(targetedString),
		}); writeErr != nil {
			log.Println(writeErr)
		}
	})

	go func() {
		for {
			_, rawMsg, err := conn.ReadMessage()
			if err != nil {
				log.Println("Error reading message:", err)
				return
			}
			log.Println("recv msg from signaling server")

			var msg common.SignalMsg
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				log.Println("Error parsing message:", err)
				continue
			}
			log.Println("msg is", msg)

			switch msg.Type {
			case common.Offer:
				log.Println("recv a offer msg")
				fromTargetedMsg := common.TargetedMsg{}
				if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); err != nil {
					log.Println("Error parsing offer:", err)
					continue
				}
				Dst = fromTargetedMsg.Src

				offer := webrtc.SessionDescription{}
				if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &offer); err != nil {
					log.Println("Error parsing offer:", err)
					continue
				}

				if err := peerConnection.SetRemoteDescription(offer); err != nil {
					log.Println("Error setting remote description:", err)
					continue
				}

				answer, err := peerConnection.CreateAnswer(nil)
				if err != nil {
					log.Println("Error creating answer:", err)
					continue
				}

				if err := peerConnection.SetLocalDescription(answer); err != nil {
					log.Println("Error setting local description:", err)
					continue
				}

				answerString, err := json.Marshal(answer)
				if err != nil {
					log.Println("Error encoding answer:", err)
					continue
				}

				toTargetedMsg := common.TargetedMsg{
					Src:  Src,
					Dst:  Dst,
					Data: string(answerString),
				}
				targetedString, err := json.Marshal(toTargetedMsg)
				if err != nil {
					log.Println("Error encoding answer:", err)
					continue
				}

				if err := conn.WriteJSON(&common.SignalMsg{
					Type: common.Answer,
					Data: string(targetedString),
				}); err != nil {
					log.Println("Error sending answer:", err)
				}
				log.Println("send answer ok")
			case common.Answer:
				log.Println("[error] grpc server should never receive ice answer")
				continue
			case common.Candidate:
				fromTargetedMsg := common.TargetedMsg{}
				if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); err != nil {
					log.Println("Error parsing candidate:", err)
					continue
				}

				candidate := webrtc.ICECandidateInit{}
				if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &candidate); err != nil {
					log.Println("Error parsing candidate:", err)
					continue
				}

				if err := peerConnection.AddICECandidate(candidate); err != nil {
					log.Println("Error adding ICE candidate:", err)
					continue
				}

				log.Println("add ice candidate:", candidate)
			default:
				panic("unhandled default case")
			}
		}
	}()
	select {}
}
