package chatroom

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// a command that got called by a client
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

// for fmt.Error() string
func (ce CommandError) Error() string {
	return fmt.Sprintf("Command %s failed: %s", ce.CommandName, ce.Reason)
}

// the built-in commands users can run
// these start with a slash
type CommandList map[string]Command

// checks if a command is available to run
func (cl CommandList) InCommandList(commandName string) bool {
	_, ok := cl[commandName]
	return ok
}

func NewCommandList() CommandList {
	return CommandList{
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
}

// makes a new room, when given a room name
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

// lists all open rooms
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

// moves the calling client to a room with a given name
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

// force the client to leave and disconnect
func exitRoom(r *Room, c *Client, s string) *CommandError {
	r.Unregister <- c
	c.KickSignal <- r
	return nil
}

// list all users in the current room
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

// list all users in the current server
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

// prints out help messages and lists available commands
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

// sends a direct message between the calling client and a target, given a nickname and a message
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
		Uuid:            c.Uuid,
		FromNick:        c.Nickname,
		Content:         whisperContents,
		SentTime:        time.Now(),
		ServerName:      r.RoomName,
		IsDirectMessage: true,
	})

	return nil
}
