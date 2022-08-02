package chatroom

import (
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type ServerInfo struct {
	Name string
	Uuid uuid.UUID
	Conn *websocket.Conn // actual connection to this remote server
}

// modeled on a Client
// represents a server on the network
type Server struct {
	Name                string
	Uuid                uuid.UUID
	LocalRooms          map[RoomInfo]*Room           // not-nil if the room exists for this server only
	ServerboundMessages chan Message                 // messages coming in from the local rooms
	RoomboundMessages   chan Message                 // messages for this server's rooms
	ParentServer        *RemoteServer                // the server this server is a child of; mainly for finding rooms
	ChildServers        map[ServerInfo]*RemoteServer // not-nil if the server exists
}

func (s Server) RoomWithNameExistsLocally(roomName string) *Room {
	for roomInfo, room := range s.LocalRooms {
		if roomInfo.Name == roomName && room != nil {
			return room
		}
	}
	return nil
}
func (s Server) RoomWithUuidExistsLocally(roomUuid uuid.UUID) *Room {
	for roomInfo, room := range s.LocalRooms {
		if roomInfo.Uuid == roomUuid && room != nil {
			return room
		}
	}
	return nil
}
func (s Server) RoomWithInfoExistsLocally(roomInfo RoomInfo) *Room {
	return s.LocalRooms[roomInfo]
}
func (s Server) ChildContainsRoom(roomInfo RoomInfo) *RemoteServer {
	for _, childServer := range s.ChildServers {
		if childServer.ContainsRoom(roomInfo) {
			return childServer
		}
	}
	return nil
}

// listen for messages from the remote hub parent server and relay them to either:
//	- the local rooms (highest priority)
//	- the child servers
//	- the parent server (lowest priority)
// in that order
func (s Server) RelayMessages() {
	defer func() {
		log.Println(s.Name, "closing RelayMessages")
		s.ParentServer.Conn.Close()
	}()
	var message Message
	var ok bool
	// main message-reading loop
	for {
		select {
		case message, ok := <-s.RoomboundMessages:
			// message just came in from some source
			if !ok {
				// remote server closed the channel
				log.Println(s.Name, "server closed the roombound channel")
				break
			}
			s.LocalRooms[message.RoomInfo].Broadcast <- message
			log.Println("sent message to room", message.RoomName)
		case message, ok = <-s.ServerboundMessages:
			// got a message from a local room
			if !ok {
				// server closed the channel
				log.Println(s.Name, "closed the serverbound channel")
				return
			}
			// look for the desired room in this server first
			room := s.RoomWithInfoExistsLocally(message.RoomInfo)
			if room != nil {
				// send message to this local room
				room.Broadcast <- message
				continue
			}
			// room does not exist
			// check for the room in a child server next
			child := s.ChildContainsRoom(message.RoomInfo)
			if child != nil {
				// child with the desired room exists, send to that child
				child.OutboundMessages <- message
				continue
			}
			// otherwise, send up to the parent server
			s.ParentServer.OutboundMessages <- message
		case message = <-s.ParentServer.LocalboundMessages:
			// message coming from the parent server
			// just send it to here
			s.ServerboundMessages <- message
		default:
			// loop through all the child servers
			for _, childServer := range s.ChildServers {
				select {
				case message = <-childServer.LocalboundMessages:
					// relay to the local server
					s.ServerboundMessages <- message
				default:
					continue
				}
			}
		}
	}
}

// creates a new local room for this server
func (s Server) NewRoom(roomName string) *Room {
	room := NewRoom(roomName)
	room.ParentServer = &s
	s.LocalRooms[room.RoomInfo()] = room
	return room
}

type AvailableRooms map[RoomInfo]bool

// creates an AvailableRooms map for setting up the parent server
func (s Server) AvailableRooms() AvailableRooms {
	rooms := make(AvailableRooms)
	for roomInfo := range s.LocalRooms {
		rooms[roomInfo] = true
	}
	return rooms
}

type RemoteServer struct {
	ServerInfo
	AvailableRooms     AvailableRooms // true if the room exists on this remote server
	InboundMessages    chan Message   // messages coming in from this remote server
	OutboundMessages   chan Message   // messages going out to this remote server
	LocalboundMessages chan Message   // messages meant for the local server
}

func NewRemoteServer(si ServerInfo) *RemoteServer {
	var rs RemoteServer
	rs.ServerInfo = si
	rs.AvailableRooms = make(AvailableRooms)
	rs.InboundMessages = make(chan Message)
	rs.OutboundMessages = make(chan Message)
	rs.LocalboundMessages = make(chan Message)
	return &rs
}

func (rs RemoteServer) ContainsRoom(roomInfo RoomInfo) bool {
	return rs.AvailableRooms[roomInfo]
}

// get a message from the remote server's socket
// and relay it to the local server
func (rs RemoteServer) ReceiveMessages() {
	defer func() {
		log.Println(rs.Name, "closing ReceiveMessages")
		rs.Conn.Close()
	}()

	rs.Conn.SetReadLimit(maxMessageSize)
	rs.Conn.SetReadDeadline(time.Now().Add(pongWait))
	rs.Conn.SetPongHandler(func(string) error { rs.Conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	// see if we have something from remote server
	var message Message
	for {
		rs.Conn.SetWriteDeadline(time.Now().Add(writeWait))
		err := rs.Conn.ReadJSON(&message)
		// failed to get a message
		if err != nil {
			log.Println(rs.Name, "failed to receive message from remote server")
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		// got a message
		// send to the local server
		rs.InboundMessages <- message
	}
}

// send messages to the remote server
func (rs RemoteServer) RelayMessages() {
	defer func() {
		log.Println(rs.Name, "closing RelayMessages")
		rs.Conn.Close()
	}()
	ticker := time.NewTicker(pingPeriod) // tick every so often
	for {
		select {
		case message, ok := <-rs.InboundMessages:
			// a message coming in from the remote
			if !ok {
				// somehow failed to get a message
				log.Println(rs.Name, "failed to get inbound message")
				return
			}
			// relay this to message to the local server for further routing
			rs.LocalboundMessages <- message
		case message, ok := <-rs.OutboundMessages:
			rs.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			// message is from the local meant for the remote
			if !ok {
				// somehow failed to get a message
				log.Println(rs.Name, "failed to get outbound message")
				return
			}
			// send via the connection to the remote
			err := rs.Conn.WriteJSON(message)
			if err != nil {
				// failed to write to the remote server
				log.Println(rs.Name, "failed to send message to remote server")
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("error: %v", err)
				}
				return
			}
		case <-ticker.C:
			// when on tick
			rs.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := rs.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// ping to the remote server failed
				log.Println(rs.Name, "failed to ping")
				return
			}
		}
	}
}
