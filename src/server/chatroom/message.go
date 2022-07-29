package chatroom

import (
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

// a representation of a message, containing a source and its contents
type Message struct {
	Uuid            uuid.UUID `json:"Uuid"`            // the UUID of the user this message is from
	FromNick        string    `json:"FromNick"`        // the nickname of the user this message is from
	Content         string    `json:"Content"`         // the actual message
	SentTime        time.Time `json:"SentTime"`        // when this message was sent
	ServerName      string    `json:"ServerName"`      // the name of the server this message is being broadcasted to
	IsDirectMessage bool      `json:"IsDirectMessage"` // whether this is a direct message or not
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
