// todo: go messageHandler有协程泄漏问题，需要使用channel来解决
// 目前还有个bug,如果两方同时dial对方，可能会丢失一个datachannel，如果存在较远的先后关系，不会有这个问题

package sender

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v3"
	"github.com/spark4862/sender/pkg/common"
	"github.com/spark4862/sender/pkg/utils"
	"log"
	"sync"
	"time"
)

var (
	signalingServer string = "ws://localhost:28080/ws"
	roomID          string = "default"
	//	destination     string = "grpc-client"
	//	source          string = "grpc-server"
	//	// todo 把client和server的代码统一
	//	isClient bool = true
	config webrtc.Configuration = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs:       []string{"turn:8.153.200.135:3479"},
				Username:   "test1",
				Credential: "123456",
			},
		},
	}
	connInitOnce  sync.Once
	settingEngine webrtc.SettingEngine = newSettingEngine(logging.LogLevelWarn)
)

type NatSender struct {
	// income and dial connections
	dst2dc map[string]*webrtc.DataChannel
	// income and dial connections
	dst2pc map[string]*webrtc.PeerConnection
	// current Listening pc
	listeningPc         *webrtc.PeerConnection
	canAccept           chan struct{}
	signalingServerConn *websocket.Conn
	source              string
}

func newNatSender(source string) *NatSender {
	return &NatSender{
		dst2dc:              make(map[string]*webrtc.DataChannel),
		dst2pc:              make(map[string]*webrtc.PeerConnection),
		listeningPc:         nil,
		canAccept:           make(chan struct{}, 1),
		signalingServerConn: nil,
		source:              source,
	}
}

var _ Sender = &NatSender{}

func connectSignalingServer(pSignalingServer string, rID string) *websocket.Conn {
	url := fmt.Sprintf("%s?room=%s", pSignalingServer, rID)
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
		log.Fatal(err)
	}
	return peerConnection
}

func newDataChannelOnOpen(dc *webrtc.DataChannel, pc *webrtc.PeerConnection, natSender *NatSender, dst *string) func() {
	return func() {
		natSender.dst2dc[*dst] = dc
		natSender.dst2pc[*dst] = pc
		natSender.canAccept <- struct{}{}
		log.Println("OnOpen" + dc.Label())
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

func newPeerConnectionOnICECandidate(c *websocket.Conn, src *string, dst *string) func(i *webrtc.ICECandidate) {
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
			Src:  *src,
			Dst:  *dst,
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

// 该函数用于创建goroutine后没有退出，是为了可能存在的连接建立后交换消息的情况，目前不清楚需不需要在连接建立后退出
// 搞了一个停止信号，但不知道对不对，因为有个disconnected状态，会不会用指数等待的sleep会更好
func messageHandler(c *websocket.Conn, p *webrtc.PeerConnection, src *string, dst *string, isClient bool) {
	for {
		// 阻塞，所以不需要停止go messageHandler
		// 但会有协程泄漏问题
		_, rawMsg, err := c.ReadMessage()
		if utils.ErrorHandler(err) {
			return
		}
		log.Println("recv msg from signaling server")

		var msg common.SignalMsg
		err = json.Unmarshal(rawMsg, &msg)
		if utils.ErrorHandler(err) {
			continue
		}

		switch msg.Type {
		case common.Answer:
			if isClient {
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
			} else {
				err := fmt.Errorf("messageHandler err: should never receive answer")
				log.Println(err)
				continue
			}
		case common.Offer:
			if isClient {
				err := fmt.Errorf("messageHandler err: should never receive offer")
				log.Println(err)
				continue
			} else {
				log.Println("recv a offer msg")
				fromTargetedMsg := common.TargetedMsg{}
				if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); err != nil {
					err = fmt.Errorf("messageHandler err when Unmarshal1: %w", err)
					log.Println(err)
					continue
				}
				*dst = fromTargetedMsg.Src

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
					Src:  *src,
					Dst:  *dst,
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
			}
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

	log.Println("send offer to signaling server ok")
}

func setPeerConnection(p *webrtc.PeerConnection, c *websocket.Conn, src *string, dst *string) {
	p.OnICEConnectionStateChange(newPeerConnectionOnICEConnectionStateChange())
	p.OnConnectionStateChange(newPeerConnectionOnConnectionStateChange())
	p.OnSignalingStateChange(newPeerConnectionOnSignalingStateChange())
	p.OnDataChannel(newPeerConnectionOnDataChannel())
	p.OnICECandidate(newPeerConnectionOnICECandidate(c, src, dst))
}

func initConn(conn *websocket.Conn, src string) *websocket.Conn {
	if conn == nil {
		connInitOnce.Do(func() {
			conn = connectSignalingServer(signalingServer, roomID)
			sendRegister(conn, src)
		})
	}
	return conn
}

func (natSender *NatSender) dial(dst string) *webrtc.DataChannel {
	dc, ok := natSender.dst2dc[dst]
	if ok {
		return dc
	} else {
		source := &(natSender.source)
		destination := &dst
		natSender.signalingServerConn = initConn(natSender.signalingServerConn, natSender.source)

		peerConnection := newPeerConnection(settingEngine, config)
		setPeerConnection(peerConnection, natSender.signalingServerConn, source, destination)
		dataChannel, err := peerConnection.CreateDataChannel(*source, nil)
		if err != nil {
			log.Fatal(err)
		}
		dataChannel.OnOpen(newDataChannelOnOpen(dataChannel, peerConnection, natSender, destination))
		go messageHandler(natSender.signalingServerConn, peerConnection, source, destination, false)
		offer := newAndSetOffer(peerConnection)
		sendOffer(offer, natSender.signalingServerConn, *source, *destination)
	}
	return dc
}

func (natSender *NatSender) Send(dst string, data string) {
	dc := natSender.dial(dst)
	if dc == nil {
		log.Println("destination is not online")
		return
	}
	if err := dc.SendText(data); err != nil {
		err = fmt.Errorf("NatSender.Send err when SendText: %w", err)
		log.Println(err)
	}
}

func (natSender *NatSender) Listen() {
	//这个可以先尝试直连，不行再用server转发，反正打洞的方式不行也是要转发的，但是打洞方式有一个缺点，就是就算要turn转发，它还是p2p的
	if natSender.listeningPc != nil {
		panic("Listen is called more than once")
	}
	source := &(natSender.source)
	destination := new(string)
	natSender.signalingServerConn = initConn(natSender.signalingServerConn, natSender.source)

	natSender.listeningPc = newPeerConnection(settingEngine, config)
	setPeerConnection(natSender.listeningPc, natSender.signalingServerConn, source, destination)
	dataChannel, err := natSender.listeningPc.CreateDataChannel(*source, nil)
	if err != nil {
		log.Fatal(err)
	}
	dataChannel.OnOpen(newDataChannelOnOpen(dataChannel, natSender.listeningPc, natSender, destination))
	go messageHandler(natSender.signalingServerConn, natSender.listeningPc, source, destination, false)
}

func (natSender *NatSender) Accept() {
	<-natSender.canAccept
	natSender.listeningPc = nil
	destination := new(string)
	source := &natSender.source

	natSender.listeningPc = newPeerConnection(settingEngine, config)
	setPeerConnection(natSender.listeningPc, natSender.signalingServerConn, source, destination)
	dataChannel, err := natSender.listeningPc.CreateDataChannel(*source, nil)
	if err != nil {
		log.Fatal(err)
	}
	dataChannel.OnOpen(newDataChannelOnOpen(dataChannel, natSender.listeningPc, natSender, destination))
	go messageHandler(natSender.signalingServerConn, natSender.listeningPc, source, destination, false)
}
