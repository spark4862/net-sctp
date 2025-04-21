// todo: go messageHandler有协程泄漏问题，需要使用channel?来解决
// todo: 还未解决资源回收问题
// todo: 目前还有个bug,如果两方同时dial对方，可能会丢失一个datachannel，如果存在较远的先后关系，不会有这个问题
// todo: webrtc的datachannel在connected可能变为disconnecting,应该在disconnecting后回收资源，防止再次connected后造成连接状态问题

package sender

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pion/logging"
	webrtc "github.com/pion/webrtc/v3"
	"github.com/spark4862/sender/pkg/common"
	eh "github.com/spark4862/sender/pkg/utils/errorhandler"
	sm "github.com/spark4862/sender/pkg/utils/safemap"
	"log"
	"sync"
	"time"
)

var (
	signalingServer string               = "ws://8.153.200.135:28080/ws"
	roomID          string               = "default"
	config          webrtc.Configuration = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs:       []string{"turn:8.153.200.135:3479"},
				Username:   "test1",
				Credential: "123456",
			},
		},
	}
	settingEngine webrtc.SettingEngine = newSettingEngine(logging.LogLevelWarn)
)

// 注意，由于包含了writeMu, 只能以指针传递，不能以值传递，因为sync.Mutex 是一个值类型，但它不应该被复制
type connWithMu struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (c *connWithMu) writeJSON(v interface{}) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	err := c.conn.WriteJSON(v)
	eh.ErrorHandler(err, 2, eh.LogAndPanic)
}

type NatSender struct {
	dst2dc   map[string]*webrtc.DataChannel
	dst2dcMu sync.RWMutex
	// income and dial connections
	dst2pc   map[string]*webrtc.PeerConnection
	dst2pcMu sync.RWMutex
	// messageHandler应该只接受自己的消息，而不是从与signalingServer的连接中接受所有消息，需要实现消息分派机制
	dst2SdpCh      map[string]chan []byte
	dst2SdpChMu    sync.RWMutex
	listeningSdpCh chan []byte
	// current Listening pc
	listeningPc *webrtc.PeerConnection
	canAccept   chan struct{}
	// 用锁来保护多协程写，用chan确保单协程读
	signalingServerConn *connWithMu
	source              string
}

func NewNatSender(source string) *NatSender {
	ns := &NatSender{
		dst2dc:         make(map[string]*webrtc.DataChannel),
		dst2pc:         make(map[string]*webrtc.PeerConnection),
		dst2SdpCh:      make(map[string]chan []byte),
		listeningSdpCh: make(chan []byte),
		listeningPc:    nil,
		canAccept:      make(chan struct{}),
		signalingServerConn: &connWithMu{
			conn: connectSignalingServer(signalingServer, roomID),
		},
		source: source,
	}
	sendRegister(ns.signalingServerConn.conn, source)
	go ns.sdpDispatcher()
	return ns
}

var _ Sender = &NatSender{}

func (natSender *NatSender) sdpDispatcher() {
	for {
		_, rawMsg, err := natSender.signalingServerConn.conn.ReadMessage()
		if eh.ErrorHandler(err, 1, eh.LogOnly) {
			return
		}
		var msg common.SignalMsg
		var targetedMsg common.TargetedMsg
		err = json.Unmarshal(rawMsg, &msg)
		if eh.ErrorHandler(err, 1, eh.LogOnly) {
			continue
		}
		err = json.Unmarshal([]byte(msg.Data), &targetedMsg)
		if eh.ErrorHandler(err, 1, eh.LogOnly) {
			continue
		}

		dst := targetedMsg.Src
		ch, ok := sm.GetSafeMap(natSender.dst2SdpCh, &natSender.dst2SdpChMu, dst)
		if !ok {
			natSender.listeningSdpCh <- rawMsg
		} else {
			ch <- rawMsg
		}
	}
}

func connectSignalingServer(pSignalingServer string, rID string) *websocket.Conn {
	url := fmt.Sprintf("%s?room=%s", pSignalingServer, rID)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	eh.ErrorHandler(err, 1, eh.LogAndPanic)

	log.Println("connected to signaling server")
	return conn
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
	eh.ErrorHandler(err, 1, eh.LogAndFatal)
	return peerConnection
}

func newDataChannelOnOpen(dc *webrtc.DataChannel, pc *webrtc.PeerConnection, natSender *NatSender, dst *string, succeeded chan bool) func() {
	return func() {
		sm.SetSafeMap(natSender.dst2dc, &natSender.dst2SdpChMu, *dst, dc)
		sm.SetSafeMap(natSender.dst2pc, &natSender.dst2SdpChMu, *dst, pc)

		log.Println("OnOpen" + dc.Label())
		if succeeded != nil {
			succeeded <- true
		}
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

func newPeerConnectionOnDataChannel(natSender *NatSender, dst *string) func(*webrtc.DataChannel) {
	return func(dc *webrtc.DataChannel) {
		log.Printf("OnDataChannel %s %d\n", dc.Label(), dc.ID())

		dc.OnOpen(func() {
			sm.SetSafeMap(natSender.dst2dc, &natSender.dst2SdpChMu, *dst, dc)
			sm.SetSafeMap(natSender.dst2pc, &natSender.dst2SdpChMu, *dst, natSender.listeningPc)
			sm.SetSafeMap(natSender.dst2SdpCh, &natSender.dst2SdpChMu, *dst, natSender.listeningSdpCh)
			natSender.canAccept <- struct{}{}
			err := dc.SendText("hello from server")
			eh.ErrorHandler(err, 1, eh.LogAndPanic)
			log.Printf("OnOpen '%s'-'%d'\n", dc.Label(), dc.ID())
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			log.Printf("OnMessage '%s': '%s'\n", dc.Label(), string(msg.Data))
		})
	}
}

func newPeerConnectionOnICECandidate(c *connWithMu, src *string, dst *string) func(i *webrtc.ICECandidate) {
	return func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		eh.ErrorHandler(err, 1, eh.LogAndPanic)
		targetedMsg := common.TargetedMsg{
			Src:  *src,
			Dst:  *dst,
			Data: string(candidateString),
		}
		targetedString, err := json.Marshal(targetedMsg)
		eh.ErrorHandler(err, 1, eh.LogAndPanic)
		c.writeJSON(&common.SignalMsg{
			Type: common.Candidate,
			Data: string(targetedString),
		})
	}
}

// 该函数用于创建goroutine后没有退出，是为了可能存在的连接建立后交换消息的情况，目前不清楚需不需要在连接建立后退出
// 搞了一个停止信号，但不知道对不对，因为有个disconnected状态，会不会用指数等待的sleep会更好
func sdpHandler(c *connWithMu, ch chan []byte, p *webrtc.PeerConnection, src *string, dst *string, isClient bool) {
	for {
		// 阻塞，所以不需要停止go sdpHandler
		// 但会有协程泄漏问题
		rawMsg := <-ch

		var msg common.SignalMsg
		err := json.Unmarshal(rawMsg, &msg)
		if eh.ErrorHandler(err, 1, eh.LogOnly) {
			continue
		}

		switch msg.Type {
		case common.Answer:
			if isClient {
				log.Println("recv a answer msg")
				fromTargetedMsg := common.TargetedMsg{}
				if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}
				answer := webrtc.SessionDescription{}
				if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &answer); eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}

				if err := p.SetRemoteDescription(answer); eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}

				log.Println("set remote desc for answer ok")
			} else {
				log.Println(fmt.Errorf("sdpHandler err: should never receive answer"))
				continue
			}
		case common.Offer:
			if isClient {
				log.Println(fmt.Errorf("sdpHandler err: should never receive offer"))
				continue
			} else {
				log.Println("recv a offer msg")
				fromTargetedMsg := common.TargetedMsg{}
				if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}
				*dst = fromTargetedMsg.Src

				offer := webrtc.SessionDescription{}
				if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &offer); eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}

				if err := p.SetRemoteDescription(offer); eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}

				answer, err := p.CreateAnswer(nil)
				if eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}

				if err := p.SetLocalDescription(answer); eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}

				answerString, err := json.Marshal(answer)
				if eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}

				toTargetedMsg := common.TargetedMsg{
					Src:  *src,
					Dst:  *dst,
					Data: string(answerString),
				}
				targetedString, err := json.Marshal(toTargetedMsg)
				if eh.ErrorHandler(err, 1, eh.LogOnly) {
					continue
				}

				c.writeJSON(&common.SignalMsg{
					Type: common.Answer,
					Data: string(targetedString),
				})
				log.Println("send answer ok")
			}
		case common.Candidate:
			fromTargetedMsg := common.TargetedMsg{}
			if err := json.Unmarshal([]byte(msg.Data), &fromTargetedMsg); eh.ErrorHandler(err, 1, eh.LogOnly) {
				continue
			}

			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(fromTargetedMsg.Data), &candidate); eh.ErrorHandler(err, 1, eh.LogOnly) {
				continue
			}

			if err := p.AddICECandidate(candidate); eh.ErrorHandler(err, 1, eh.LogOnly) {
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
	eh.ErrorHandler(err, 1, eh.LogAndFatal)

	err = c.WriteJSON(&common.SignalMsg{
		Type: common.Register,
		Data: string(registerString),
	})
	eh.ErrorHandler(err, 1, eh.LogAndFatal)
}

func newAndSetOffer(peerConnection *webrtc.PeerConnection) webrtc.SessionDescription {
	offer, err := peerConnection.CreateOffer(nil)
	eh.ErrorHandler(err, 1, eh.LogAndFatal)

	err = peerConnection.SetLocalDescription(offer)
	eh.ErrorHandler(err, 1, eh.LogAndFatal)
	return offer
}

func sendOffer(o webrtc.SessionDescription, c *connWithMu, src string, dst string) {
	offerString, err := json.Marshal(o)
	eh.ErrorHandler(err, 1, eh.LogAndFatal)

	targetedMsg := common.TargetedMsg{
		Src:  src,
		Dst:  dst,
		Data: string(offerString),
	}
	targetedString, err := json.Marshal(targetedMsg)
	eh.ErrorHandler(err, 1, eh.LogAndFatal)

	c.writeJSON(&common.SignalMsg{
		Type: common.Offer,
		Data: string(targetedString),
	})

	log.Println("send offer to signaling server ok")
}

func setPeerConnection(p *webrtc.PeerConnection, natSender *NatSender, src *string, dst *string) {
	p.OnICEConnectionStateChange(newPeerConnectionOnICEConnectionStateChange())
	p.OnConnectionStateChange(newPeerConnectionOnConnectionStateChange())
	p.OnSignalingStateChange(newPeerConnectionOnSignalingStateChange())
	p.OnDataChannel(newPeerConnectionOnDataChannel(natSender, dst))
	p.OnICECandidate(newPeerConnectionOnICECandidate(natSender.signalingServerConn, src, dst))
}

func (natSender *NatSender) dial(dst string) *webrtc.DataChannel {
	dc, ok := sm.GetSafeMap(natSender.dst2dc, &natSender.dst2dcMu, dst)
	succeeded := make(chan bool)
	defer close(succeeded)
	if ok {
		return dc
	} else {
		source := &(natSender.source)
		destination := &dst

		peerConnection := newPeerConnection(settingEngine, config)
		setPeerConnection(peerConnection, natSender, source, destination)
		var err error
		dc, err = peerConnection.CreateDataChannel(*source, nil)
		eh.ErrorHandler(err, 1, eh.LogAndFatal)

		dc.OnOpen(newDataChannelOnOpen(dc, peerConnection, natSender, destination, succeeded))
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			log.Printf("OnMessage '%s': '%s'\n", dc.Label(), string(msg.Data))
		})
		ch := make(chan []byte)
		sm.SetSafeMap(natSender.dst2SdpCh, &natSender.dst2SdpChMu, *destination, ch)
		go sdpHandler(natSender.signalingServerConn, ch, peerConnection, source, destination, true)
		offer := newAndSetOffer(peerConnection)
		sendOffer(offer, natSender.signalingServerConn, *source, *destination)
		select {
		case <-time.After(time.Second * 5):
			log.Println("dial time out")
			eh.ErrorHandler(dc.Close(), 1, eh.LogAndPanic)
			eh.ErrorHandler(peerConnection.Close(), 1, eh.LogAndPanic)
			close(ch)
			sm.DeleteSafeMap(natSender.dst2SdpCh, &natSender.dst2SdpChMu, *destination)
			return nil
		case <-succeeded:
			return dc
		}
	}
}

func (natSender *NatSender) Send(dst string, data string) {
	dc := natSender.dial(dst)
	if dc == nil {
		log.Println("destination is not online")
		return
	}
	err := dc.SendText(data)
	eh.ErrorHandler(err, 1, eh.LogAndPanic)
}

func (natSender *NatSender) Listen() {
	//这个可以先尝试直连，不行再用server转发，反正打洞的方式不行也是要转发的，但是打洞方式有一个缺点，就是就算要turn转发，它还是p2p的
	if natSender.listeningPc != nil {
		panic("Listen is called more than once")
	}
	source := &(natSender.source)
	destination := new(string)

	natSender.listeningPc = newPeerConnection(settingEngine, config)
	setPeerConnection(natSender.listeningPc, natSender, source, destination)
	go sdpHandler(natSender.signalingServerConn, natSender.listeningSdpCh, natSender.listeningPc, source, destination, false)
}

func (natSender *NatSender) Accept() {
	<-natSender.canAccept
	log.Println("start accept")
	natSender.listeningPc = nil
	// set to update listener
	destination := new(string)
	source := &natSender.source
	natSender.listeningPc = newPeerConnection(settingEngine, config)
	setPeerConnection(natSender.listeningPc, natSender, source, destination)
	natSender.listeningSdpCh = make(chan []byte)
	go sdpHandler(natSender.signalingServerConn, natSender.listeningSdpCh, natSender.listeningPc, source, destination, false)
}

func (natSender *NatSender) Close() {

}
