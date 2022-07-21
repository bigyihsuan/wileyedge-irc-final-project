package chatroom

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

var rooms = make(map[uuid.UUID]*Room) // the list of channels

// manages active clients, and broadcasting to active clients
type Room struct {
	Uuid       uuid.UUID        // unique room identifier
	RoomName   string           // name of the room
	Clients    map[*Client]bool // the list of registered clients
	Broadcast  chan Message     // inbound messages from clients
	Register   chan *Client     // register requests from clients
	Unregister chan *Client     // unregister requests from clients
}

func NewRoom(roomName string) *Room {
	r := &Room{
		Uuid:       uuid.New(),
		RoomName:   roomName,
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan Message),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
	rooms[r.Uuid] = r
	return r
}

func (r Room) GetClientByUuid(uuid uuid.UUID) *Client {
	for c := range r.Clients {
		if c.Uuid == uuid {
			return c
		}
	}
	return nil
}

// run the server
func (r *Room) Run() {
	for {
		select {
		case client := <-r.Register:
			// register an incoming user
			log.Printf("Register %s\n", client.Nickname)
			// broadcast "joined" message
			go func() {
				r.Broadcast <- r.serverMessage(fmt.Sprintf("---- <%s> joined %s ----", client.Nickname, r.RoomName))
			}()
			r.Clients[client] = true
		case client := <-r.Unregister:
			// unregister an outgoing user
			// check if the user is actually in the room first
			if _, ok := r.Clients[client]; ok {
				// they are in, remove them
				log.Printf("Unregister %s\n", client.Nickname)
				// broadcast "left" message
				go func() {
					r.Broadcast <- r.serverMessage(fmt.Sprintf("---- <%s> left %s ----", client.Nickname, r.RoomName))
				}()
				delete(r.Clients, client)
				close(client.Send)
			}
		case message := <-r.Broadcast:
			// a message just came in from some client
			// check if it's a slash-command first
			if message.IsCommand() {
				log.Printf("Got command `%s` from %v\n", message.Content, message.Uuid)
				command := message.ToCommand()
				if commands.InCommandList(command.Name) {
					callingClient := r.GetClientByUuid(command.Uuid)
					err := commands[command.Name].Operation(r, callingClient, command.Args...)
					if err != nil {
						callingClient.DirectMessage(r.serverMessage(err.Error()))
					}
				}
			} else {
				for client := range r.Clients {
					// broadcast to all clients
					// append the sender's username to the message
					select {
					case client.Send <- message:
						log.Printf("Sent message from %s to %s\n", message.Uuid, client.Uuid)
						// successful send
						// nop
					default:
						// the client we are trying to send to doesn't exist
						// remove them from our client list
						log.Printf("%s is not logged in\n", client.Nickname)
						close(client.Send)
						delete(r.Clients, client)
					}
				}
			}
		}
	}
}

// a representation of a message, containing a source and its contents
type Message struct {
	Uuid     uuid.UUID `json:"Uuid"`     // the UUID of the user this message is from
	FromNick string    `json:"FromNick"` // the nickname of the user this message is from
	Content  string    `json:"Content"`  // the actual message
	SentTime time.Time `json:"SentTime"`
}

func (m Message) IsCommand() bool {
	return len(m.Content) > 0 && m.Content[0] == '/'
}
func (m Message) ToCommand() CalledCommand {
	components := strings.Split(m.Content, " ")
	command := CalledCommand{Uuid: m.Uuid}
	for i, c := range components {
		if i == 0 && c[0] == '/' {
			command.Name = c[1:]
			break
		}
	}
	command.Args = components[1:]
	return command
}

type CalledCommand struct {
	Uuid uuid.UUID // the user running the command
	Name string    // the name of the command
	Args []string  // the arguments to the command
}

// a command that operates on a client
type Command struct {
	Name      string
	Operation func(r *Room, c *Client, s ...string) *CommandError
}

type CommandError struct {
	CommandName string
	Reason      string
}

func (ce CommandError) Error() string {
	return fmt.Sprintf("Command %s failed: %s", ce.CommandName, ce.Reason)
}

// the built-in commands users can run
// these start with a slash
type CommandList map[string]Command

func (cl CommandList) InCommandList(commandName string) bool {
	_, ok := cl[commandName]
	return ok
}

var commands = CommandList{
	// create a new room
	"make": {
		Name: "make",
		Operation: func(r *Room, c *Client, s ...string) *CommandError {
			if len(s) != 1 {
				return &CommandError{
					CommandName: "make",
					Reason:      fmt.Sprintf("Wrong number of arguments: want 1 (channel name), got %v", len(s)),
				}
			}
			NewRoom(s[0])
			return nil
		},
	},
	// list rooms, marking which one the client is in
	"list": {
		Name: "list",
		Operation: func(r *Room, c *Client, s ...string) *CommandError {
			var builder strings.Builder
			builder.WriteString("Channels:\n")
			builder.WriteString("---------\n")
			for u, room := range rooms {
				builder.WriteString(room.RoomName)
				if u == c.CurrentRoom.Uuid {
					builder.WriteString(" ***")
				}
				builder.WriteString("\n")
			}
			c.DirectMessage(r.serverMessage(builder.String()))
			log.Println(c.Uuid)
			log.Println(builder.String())
			return nil
		},
	},
	// join the room
	// exit entirely
}

func (r Room) serverMessage(content string) Message {
	return Message{
		Uuid:     r.Uuid,
		FromNick: "SERVER",
		Content:  content,
		SentTime: time.Now(),
	}
}
