package chatroom

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// https://github.com/gorilla/websocket/blob/af47554f343b4675b30172ac301638d350db34a5/examples/chat/client.go#L16-L38
const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// middleman between websocket and chatroom
type Client struct {
	Nickname    string
	CurrentRoom *Room           // the room this client is in
	Connection  *websocket.Conn // connection to the server
	Send        chan Message    // channel of outbound messages
}

func (c *Client) readSocket() {
	// unregister and disconnect when done reading
	defer func() {
		c.CurrentRoom.Unregister <- c
		c.Connection.Close()
	}()

	// setting things for conn...
	c.Connection.SetReadLimit(maxMessageSize)
	c.Connection.SetReadDeadline(time.Now().Add(pongWait))
	c.Connection.SetPongHandler(func(string) error { c.Connection.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	// main message-reading loop
	for {
		_, message, err := c.Connection.ReadMessage()

		// failed to get a message
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
		log.Printf("client got message `%s` from %s\n", string(message), c.Nickname)
		sent := Message{
			From:     c.Nickname,
			Content:  string(message),
			SentTime: time.Now(),
		}
		c.CurrentRoom.Broadcast <- sent
	}
}

// moves messages from the current room to the websocket connection
func (c *Client) writeSocket() {
	ticker := time.NewTicker(pingPeriod) // tick every so often
	defer func() {
		ticker.Stop()
		c.Connection.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Connection.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// room closed the channel
				c.Connection.WriteMessage(websocket.CloseMessage, []byte{})
			}
			w, err := c.Connection.NextWriter(websocket.TextMessage)
			if err != nil {
				// cannot write to the connection
				return
			}
			// send the content of the message to the client
			// w.Write(message.Content)
			json.NewEncoder(w).Encode(message)

			// add queued messages to current websocket message
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				// w.Write((<-c.Send).Content)
				json.NewEncoder(w).Encode(<-c.Send)
			}

			if err := w.Close(); err != nil {
				// cannot close the writer to the outbound queue
				return
			}
		case <-ticker.C:
			// when on tick
			c.Connection.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				// ping to the server failed
				return
			}
		}
	}
}

// handle websocket requests from peers
func ServeWebSocket(room *Room, w http.ResponseWriter, r *http.Request) {
	// convert http to websocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// failed to convert
		log.Println(err)
		return
	}
	nickname := r.URL.Query().Get("nickname")
	log.Printf("Got client with nickname `%s`", nickname)
	client := &Client{Nickname: string(nickname), CurrentRoom: room, Connection: conn, Send: make(chan Message)}
	// enter the room
	client.CurrentRoom.Register <- client

	// async getting and writing of messages
	go client.readSocket()
	go client.writeSocket()
}
