package chatroom

import (
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// modeled on a Client
// represents a server on the network
type Server struct {
	Name             string
	Uuid             uuid.UUID
	Conn             *websocket.Conn
	AvailableRooms   map[*Room]bool   // true if the room exists
	InboundMessages  chan Message     // messages coming in from the remote server
	OutboundMessages chan Message     // messages going out to the remote server
	ParentServer     *Server          // the server this server is a child of; mainly for finding rooms
	ChildServers     map[*Server]bool // true if the server exists
}

func (s Server) RoomExistsHere(roomName string) (bool, *Room) {
	for room, exists := range s.AvailableRooms {
		if room.RoomName == roomName && exists {
			return true, room
		}
	}
	return false, nil
}

// send an outgoing message from the local server to this remote server
// this function assumes that messages coming from OutboundMessages are meant for this server
func (s Server) sendMessages() {
	ticker := time.NewTicker(pingPeriod) // tick every so often
	defer func() {
		log.Println(s.Name, "closing sendMessage")
		s.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-s.OutboundMessages:
			s.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// remote server closed the channel
				log.Println(s.Name, "server closed the channel")
				s.Conn.WriteMessage(websocket.CloseMessage, []byte{})
			}
			err := s.Conn.WriteJSON(message)
			if err != nil {
				log.Println(s.Name, "cannot write to connection")
				return
			}
			log.Println("sent message to", s.Name)
		case <-ticker.C:
			// when on tick
			s.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// ping to the remote server failed
				log.Println(s.Name, "failed to ping")
				return
			}
		}
	}
}

// listen for messages from the remote server and send them to the correct local rooms
// this will attempt to look for the desired room locally first; if it does not exist, it relays it to the parent server
func (s Server) receiveMessages() {
	defer func() {
		log.Println(s.Name, "closing receiveMessage")
		s.Conn.Close()
	}()

	s.Conn.SetReadLimit(maxMessageSize)
	s.Conn.SetReadDeadline(time.Now().Add(pongWait))
	s.Conn.SetPongHandler(func(string) error { s.Conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		var message Message
		err := s.Conn.ReadJSON(&message)
		// failed to get a message
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		// look for the desired room in this server
		roomExists, room := s.RoomExistsHere(message.RoomName)
		if roomExists && room != nil {
			// send message to that room
			room.Broadcast <- message
		}
		// room does not exist, send up to the parent server
		s.ParentServer.InboundMessages <- message
	}
}
