package chatroom

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

var ActiveRooms = make(map[uuid.UUID]*Room)       // the list of channels
var StringToRoomUUID = make(map[string]uuid.UUID) // convert a channel name to its uuid
var AllUsers = make(map[*Client]IsInRoom)

type IsInRoom bool

// manages active clients, and broadcasting to active clients
type Room struct {
	Uuid       uuid.UUID            // unique room identifier
	RoomName   string               // name of the room
	Clients    map[*Client]IsInRoom // the list of registered clients
	Broadcast  chan Message         // inbound messages from clients
	Register   chan *Client         // register requests from clients
	Unregister chan *Client         // unregister requests from clients
	SwitchRoom chan *RoomSwitch     // room switch requests from clients
	isRunning  bool
	Commands   CommandList // commands available to the server
}

type PrivateRoom struct {
	Room
	AllowedUsers map[uuid.UUID]bool
}

// holds a client and the target room they are switching to
type RoomSwitch struct {
	client     *Client
	targetRoom *Room
}

// create a new room with a given name
func NewRoom(roomName string) *Room {
	r := &Room{
		Uuid:       uuid.New(),
		RoomName:   roomName,
		Clients:    make(map[*Client]IsInRoom),
		Broadcast:  make(chan Message),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		SwitchRoom: make(chan *RoomSwitch),
		Commands:   NewCommandList(),
	}
	ActiveRooms[r.Uuid] = r
	StringToRoomUUID[roomName] = r.Uuid
	return r
}

// helper log functions
func (r Room) Logf(format string, v ...any) {
	log.Printf("[%v] %s", r.RoomName, fmt.Sprintf(format, v...))
}
func (r Room) Logln(v ...any) {
	log.Printf("[%v] %s", r.RoomName, fmt.Sprintln(v...))
}

// getting client by a criteria
func (r Room) GetClientByUuid(uuid uuid.UUID) *Client {
	for c := range r.Clients {
		if c.Uuid == uuid {
			return c
		}
	}
	return nil
}
func (r Room) GetClientByNickname(nickname string) *Client {
	for c, isInRoom := range AllUsers {
		if c.Nickname == nickname && isInRoom {
			return c
		}
	}
	return nil
}

// run the server
func (r *Room) Run() {
	r.isRunning = true

	r.Logln("Starting room")
	for {
		select {
		case client := <-r.Register:
			// register an incoming user
			r.Logf("Register %s\n", client.Nickname)
			// broadcast "joined" message
			go func() {
				r.Broadcast <- r.serverMessage(fmt.Sprintf("---- <%s> joined %s ----", client.Nickname, r.RoomName))
			}()
			r.Clients[client] = true
			AllUsers[client] = true
		case client := <-r.Unregister:
			// unregister an outgoing user
			// check if the user is actually in the room first
			if _, ok := r.Clients[client]; ok {
				// they are in, remove them
				r.Logf("Unregister %s\n", client.Nickname)
				// broadcast "left" message
				go func() {
					r.Broadcast <- r.serverMessage(fmt.Sprintf("---- <%s> left %s (disconnected) ----", client.Nickname, r.RoomName))
				}()
				// remove from the client list
				delete(r.Clients, client)
				AllUsers[client] = false
				// close the sending channel
				close(client.Send)
			}
		case message := <-r.Broadcast:
			// a message just came in from some client
			// check if it's a slash-command first
			if message.IsCommand() {
				// got a command
				r.Logf("Got command `%s` from %v\n", message.Content, message.FromNick)
				command := message.ToCommand()
				callingClient := r.GetClientByUuid(command.Uuid)
				// check if the commad is in the command list
				if r.Commands.InCommandList(command.Name) {
					// in the list, ok to run
					callingClient.ServerDirectMessage(r.serverMessage(message.Content))
					// go func() {
					// call command
					err := r.Commands[command.Name].Operation(r, callingClient, command.Args)
					if err != nil {
						callingClient.ServerDirectMessage(r.serverMessage(err.Error()))
					}
					// }()
				} else {
					// otherwise say that the command doesn't exist
					callingClient.ServerDirectMessage(r.serverMessage(fmt.Sprintf("Command not found: %s", message.Content)))
				}
			} else {
				for client := range r.Clients {
					// broadcast to all clients
					// append the sender's username to the message
					select {
					case client.Send <- message:
						r.Logf("Sent message from %s to %s\n", message.FromNick, client.Nickname)
						// successful send
						// nop
					default:
						// the client we are trying to send to doesn't exist
						// remove them from our client list
						r.Logf("%s is not logged in\n", client.Nickname)
						close(client.Send)
						delete(r.Clients, client)
					}
				}
			}
		case rs := <-r.SwitchRoom:
			// a client wants to switch rooms
			if _, ok := r.Clients[rs.client]; ok {
				// remove client from client list
				delete(r.Clients, rs.client)
				// send "left" message
				go func() {
					r.Broadcast <- r.serverMessage(fmt.Sprintf("---- <%s> left %s (switched rooms) ----", rs.client.Nickname, r.RoomName))
				}()
				// DON'T close the send channel, need for the next room
				// physically swtich the room
				rs.client.CurrentRoom = rs.targetRoom
				// move the client into the new room
				rs.targetRoom.Register <- rs.client
				r.Logf("Successfully moved %v to %v\n", rs.client.Nickname, rs.targetRoom.RoomName)
			}
		}
	}
}

// helper method to send a message originating from the server itself
func (r Room) serverMessage(content string) Message {
	return Message{
		Uuid:            r.Uuid,
		FromNick:        fmt.Sprintf("{%s}", r.RoomName),
		Content:         content,
		SentTime:        time.Now(),
		ServerName:      r.RoomName,
		IsDirectMessage: true,
	}
}

// checks if the nickname already exists in the room
func (r Room) NicknameAlreadyExists(nickname string) bool {
	for client, isInRoom := range r.Clients {
		if isInRoom && client.Nickname == nickname {
			return true
		}
	}
	return false
}
