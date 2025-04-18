// signaling-server
// api: ws://server-ip:port/ws?room=room_id
// parameter room is necessary
// record all clients in room
// when a client join a room, sent id in the room to it

// grpc通信逻辑
// grpc-server与signaling server建立连接，发送RegisterMsg,仅在加入时发送一次
// grpc-client与signaling server建立连接，发送RegisterMsg,仅在加入时发送一次
// grpc-client根据自己的id和grpc-server的id构造OfferMsg,发送给signaling-server,signaling-server转发给grpc-server
// 然后就是ice连接建立
package main

import (
	"encoding/json"
	"flag"
	"github.com/gorilla/websocket"
	common "github.com/spark4862/sender/pkg/common"
	"log"
	"net/http"
	"strconv"
	"sync"
)

// Room use room to distinguish between clusters
type Room struct {
	clients map[string]*websocket.Conn
	mu      sync.Mutex
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	rooms  = make(map[string]*Room)
	roomMu sync.Mutex
)

func main() {
	port := flag.Int("port", 28080, "Port to listen on")
	flag.Parse()

	http.HandleFunc("/ws", handleWebSocket)
	log.Printf("Signaling server starting on :%d \n", *port)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading to WebSocket:", err)
		return
	}
	defer func(conn *websocket.Conn) {
		err := conn.Close()
		if err != nil {
			log.Println(err)
		}
	}(conn)

	remoteAddr := conn.RemoteAddr().String()
	log.Println("New WebSocket connection from:", remoteAddr)

	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		//roomID = fmt.Sprintf("room_%d", len(rooms)+1)
		//log.Printf("Created new room: %s\n", roomID)
		log.Println("roomID is empty, return")
		return
	}

	roomMu.Lock()
	room, exists := rooms[roomID]
	if !exists {
		room = &Room{clients: make(map[string]*websocket.Conn)}
		rooms[roomID] = room
	}
	roomMu.Unlock()

	var clientId string

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message:", err)
			break
		}

		var signalMsg common.SignalMsg
		if err := json.Unmarshal(message, &signalMsg); err != nil {
			log.Println("Error unmarshalling message:", err)
			continue
		}

		switch signalMsg.Type {
		case common.Register:
			registerMsg := common.RegisterMsg{}
			if err := json.Unmarshal([]byte(signalMsg.Data), &registerMsg); err != nil {
				log.Println("Error unmarshalling message:", err)
				continue
			}
			clientId = registerMsg.Id
			room.mu.Lock()
			room.clients[clientId] = conn
			room.mu.Unlock()
			defer func() {
				room.mu.Lock()
				delete(room.clients, registerMsg.Id)
				room.mu.Unlock()
			}()
			log.Printf("Client[%v] joined room [%s]\n", clientId, roomID)

		case common.Candidate, common.Offer, common.Answer:
			targetedMsg := common.TargetedMsg{}
			if err := json.Unmarshal([]byte(signalMsg.Data), &targetedMsg); err != nil {
				log.Println("Error unmarshalling message:", err)
				continue
			}
			room.mu.Lock()
			targetClient := room.clients[targetedMsg.Dst]
			room.mu.Unlock()
			if targetClient != nil && targetClient != conn {
				if err := targetClient.WriteMessage(messageType, message); err != nil {
					log.Println("Error sending message:", err)
				} else {
					log.Printf("send message to target[%v]\n", targetedMsg.Dst)
				}
			} else {
				log.Printf("Target not found %s\n", targetedMsg.Dst)
				continue
			}

		default:
			log.Println("wrong message type")
			continue
		}
	}

	log.Printf("Client[%v] left room [%s]\n", clientId, roomID)
	defer cleanRooms(room, roomID)
}

func cleanRooms(room *Room, roomID string) {
	roomMu.Lock()
	defer roomMu.Unlock()
	if len(room.clients) == 0 {
		delete(rooms, roomID)
	}
}
