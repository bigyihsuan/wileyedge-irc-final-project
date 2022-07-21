package chatroom

import "log"

// manages active clients, and broadcasting to active clients
type Room struct {
	Clients    map[*Client]bool // the list of registered clients
	Broadcast  chan Message     // inbound messages from clients
	Register   chan *Client     // register requests from clients
	Unregister chan *Client     // unregister requests from clients
}

// a representation of a message, containing a source and its contents
type Message struct {
	From    string `json:"From"`    // the nickname of the user this message is from
	Content string `json:"Content"` // the actual message
}

func NewRoom() *Room {
	return &Room{
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan Message),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

// run the server
func (r *Room) Run() {
	for {
		select {
		case client := <-r.Register:
			// register an incoming user
			log.Printf("Register %s\n", client.Nickname)
			r.Clients[client] = true
		case client := <-r.Unregister:
			// unregister an outgoing user
			// check if the user is actually in the room first
			if _, ok := r.Clients[client]; ok {
				// they are in, remove them
				log.Printf("Unregister %s\n", client.Nickname)
				delete(r.Clients, client)
				close(client.Send)
			}
		case message := <-r.Broadcast:
			// a message just came in from some client
			for client := range r.Clients {
				// broadcast to all clients
				// append the sender's username to the message
				select {
				case client.Send <- message:
					log.Printf("Sent message from %s to %s\n", message.From, client.Nickname)
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
