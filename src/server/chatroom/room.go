package chatroom

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
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
	Commands   CommandList
}

type RoomSwitch struct {
	client     *Client
	targetRoom *Room
}

func NewRoom(roomName string) *Room {
	r := &Room{
		Uuid:       uuid.New(),
		RoomName:   roomName,
		Clients:    make(map[*Client]IsInRoom),
		Broadcast:  make(chan Message),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		SwitchRoom: make(chan *RoomSwitch),
	}
	ActiveRooms[r.Uuid] = r
	StringToRoomUUID[roomName] = r.Uuid
	r.Commands = CommandList{
		// create a new room
		"make": {
			Name:       "make",
			Operation:  makeRoom,
			HelpString: "Usage:\n/make roomName\n    Makes a new room with a given name.",
		},
		// list rooms, marking which one the client is in
		"listrooms": {
			Name:       "listrooms",
			Operation:  listRoom,
			HelpString: "Usage:\n/listrooms\n    Lists all open rooms.",
		},
		// join a room
		"join": {
			Name:       "join",
			Operation:  joinRoom,
			HelpString: "Usage:\n/join roomName\n    Moves the client to the given room.",
		},
		// exit entirely
		"exit": {
			Name:       "exit",
			Operation:  exitRoom,
			HelpString: "Usage:\n/exit\n    Leave the server.",
		},
		// list the users in the current room
		"listusers": {
			Name:       "listusers",
			Operation:  listUsers,
			HelpString: "Usage:\n/listusers\n    List the users in the current room.",
		},
		// list all users in the current server
		"listallusers": {
			Name:       "listallusers",
			Operation:  listAllUsers,
			HelpString: "Usage:\n/listallusers\n    List all users in the current server.",
		},
		// list commands
		"help": {
			Name:       "help",
			Operation:  help,
			HelpString: "Usage:\n/help\n    List all available commands.\n/help command\n    Print out the helpstring for that command.",
		},
		// whisper: send a dm to another client
		"whisper": {
			Name:       "whisper",
			Operation:  whisper,
			HelpString: "Usage:\n/whisper nickName message\n    Direct message a user with the given nickname.",
		},
	}
	return r
}

func (r Room) Logf(format string, v ...any) {
	log.Printf("[%v] %s", r.RoomName, fmt.Sprintf(format, v...))
}
func (r Room) Logln(v ...any) {
	log.Printf("[%v] %s", r.RoomName, fmt.Sprintln(v...))
}

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
			// a client wants to swithc rooms
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

// a representation of a message, containing a source and its contents
type Message struct {
	Uuid       uuid.UUID `json:"Uuid"`       // the UUID of the user this message is from
	FromNick   string    `json:"FromNick"`   // the nickname of the user this message is from
	Content    string    `json:"Content"`    // the actual message
	SentTime   time.Time `json:"SentTime"`   // when this message was sent
	ServerName string    `json:"ServerName"` // the name of the server this message is being broadcasted to
}

func (m Message) IsCommand() bool {
	return len(m.Content) > 0 && m.Content[0] == '/'
}

// splits a message into a command name and arguments
// these arguments are a single string
func (m Message) ToCommand() CalledCommand {
	components := strings.SplitN(m.Content, " ", 2)
	command := CalledCommand{Uuid: m.Uuid}
	if len(components) > 0 {
		// get the command name
		if components[0][0] == '/' {
			command.Name = strings.TrimLeft(components[0], "/")
		}
	}
	if len(components) >= 2 {
		command.Args = components[1]
	}
	log.Println(command)
	return command
}

type CalledCommand struct {
	Uuid uuid.UUID // the user running the command
	Name string    // the name of the command
	Args string    // the arguments to the command
}

// a command that operates on a client
type Command struct {
	Name       string
	Operation  func(r *Room, c *Client, s string) *CommandError
	HelpString string
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

func makeRoom(r *Room, c *Client, s string) *CommandError {
	args := strings.SplitN(s, " ", 2)
	if len(args) != 1 {
		return &CommandError{
			CommandName: "make",
			Reason:      fmt.Sprintf("Wrong number of arguments: want 1 (channel name), got %v", len(args)),
		}
	}
	roomName := args[0]
	newroom := NewRoom(roomName)
	c.ServerDirectMessage(r.serverMessage(fmt.Sprintf("Successfully made new room `%s`", roomName)))
	r.Logln(c.Nickname, fmt.Sprintf("made new room `%s`", newroom.RoomName))
	return nil
}

func listRoom(r *Room, c *Client, s string) *CommandError {
	var builder strings.Builder
	builder.WriteString("\nChannels:\n")
	builder.WriteString("---------\n")
	for u, room := range ActiveRooms {
		builder.WriteString(room.RoomName)
		if u == c.CurrentRoom.Uuid {
			builder.WriteString(" (* joined)")
		}
		builder.WriteString("\n")
	}
	c.ServerDirectMessage(r.serverMessage(builder.String()))
	r.Logln(c.Nickname, "listed rooms")
	// log.Println(builder.String())
	return nil
}

func joinRoom(r *Room, c *Client, s string) *CommandError {
	args := strings.SplitN(s, " ", 2)
	if len(args) != 1 {
		return &CommandError{
			CommandName: "join",
			Reason:      fmt.Sprintf("Wrong number of arguments: want 1 (channel name), got %v", len(args)),
		}
	}
	// see if the wanted room exists
	nextRoom, ok := ActiveRooms[StringToRoomUUID[args[0]]]
	if !ok {
		return &CommandError{
			CommandName: "join",
			Reason:      fmt.Sprintf("Room `%v` does not exist", s),
		}
	}
	if !nextRoom.isRunning {
		r.Logln("running", nextRoom.RoomName)
		go nextRoom.Run()
	}
	// room exists, we're all ok
	// remove the client from the current room
	// tell the client to switch to the new room
	// switch message contains the new room name for the webclient to display
	switchMessage, err := websocket.NewPreparedMessage(websocket.TextMessage, []byte(nextRoom.RoomName))
	if err != nil {
		r.Logf("Cannot make switch message for %v: %v\n", c.Nickname, err)
		// reset state
		r.Register <- c
	}
	err = c.Connection.WritePreparedMessage(switchMessage)
	if err != nil {
		r.Logf("Failed to send switch message to %v: %v", c.Nickname, err)
	}
	r.Logf("switching %s from this room", c.Nickname)
	roomswitch := new(RoomSwitch)
	roomswitch.client = c
	roomswitch.targetRoom = nextRoom
	go func() {
		r.SwitchRoom <- roomswitch
	}()
	inroom, ok := r.Clients[c]
	r.Logln("in the room:", inroom, ok)
	return nil
}

func exitRoom(r *Room, c *Client, s string) *CommandError {
	// force the client to leave and disconnect
	r.Unregister <- c
	c.KickSignal <- r
	return nil
}

func listUsers(r *Room, c *Client, s string) *CommandError {
	var builder strings.Builder
	builder.WriteString("\nUsers:\n")
	builder.WriteString("---------\n")
	for client, inRoom := range r.Clients {
		if inRoom {
			builder.WriteString(client.Nickname)
			if client.Uuid == c.Uuid {
				builder.WriteString(" (* you)")
			}
			builder.WriteString("\n")
		}
	}
	c.ServerDirectMessage(r.serverMessage(builder.String()))
	r.Logln(c.Nickname, "listed room users")
	return nil
}
func listAllUsers(r *Room, c *Client, s string) *CommandError {
	var builder strings.Builder
	builder.WriteString("\nAll Users:\n")
	builder.WriteString("---------\n")
	for client, inRoom := range AllUsers {
		if inRoom {
			builder.WriteString(client.Nickname)
			if client.Uuid == c.Uuid {
				builder.WriteString(" (* you)")
			}
			builder.WriteString("\n")
		}
	}
	c.ServerDirectMessage(r.serverMessage(builder.String()))
	r.Logln(c.Nickname, "listed all users")
	return nil
}

func help(r *Room, c *Client, s string) *CommandError {
	var builder strings.Builder
	var args []string
	if len(s) == 0 {
		args = []string{}
	} else {
		args = strings.SplitAfter(s, " ")
	}
	fmt.Println(args)
	fmt.Println(len(args))
	switch len(args) {
	case 0:
		// no args = list commands
		builder.WriteString("\nAvailable Commands:\n")
		builder.WriteString("-------------------\n")
		for command := range r.Commands {
			builder.WriteString(command)
			builder.WriteString("\n")
		}
		c.ServerDirectMessage(r.serverMessage(builder.String()))
	case 1:
		// 1 arg = print the helpstring of the command
		command, ok := r.Commands[args[0]]
		if !ok {
			// command doesn't exist
			return &CommandError{
				CommandName: "help",
				Reason:      fmt.Sprintf("Command `%s` does not exist", args[0]),
			}
		}
		c.ServerDirectMessage(r.serverMessage(command.HelpString))
	}
	return nil
}

func whisper(r *Room, c *Client, s string) *CommandError {
	// expected arguments:
	// target nickname args[0]
	// message contents args[1]
	args := strings.SplitN(s, " ", 2)
	if len(args) < 2 {
		return &CommandError{
			CommandName: "whisper",
			Reason:      fmt.Sprintf("Wrong number of arguments: want 2 (nickname, contents), got %v args", len(args)),
		}
	}
	targetName := args[0]
	whisperContents := args[1]
	target := r.GetClientByNickname(targetName)
	if target == nil {
		return &CommandError{
			CommandName: "whisper",
			Reason:      fmt.Sprintf("Target client %s does not exist, or is offline", targetName),
		}
	}
	c.DirectMessageToOtherClient(*target, Message{
		Uuid:       c.Uuid,
		FromNick:   c.Nickname,
		Content:    whisperContents,
		SentTime:   time.Now(),
		ServerName: r.RoomName,
	})

	return nil
}

func (r Room) serverMessage(content string) Message {
	return Message{
		Uuid:       r.Uuid,
		FromNick:   fmt.Sprintf("{%s}", r.RoomName),
		Content:    content,
		SentTime:   time.Now(),
		ServerName: r.RoomName,
	}
}

func (r Room) NicknameAlreadyExists(nickname string) bool {
	for client, isInRoom := range r.Clients {
		if isInRoom && client.Nickname == nickname {
			return true
		}
	}
	return false
}
