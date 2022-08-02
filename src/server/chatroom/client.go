package chatroom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// https://github.com/gorilla/websocket/blob/af47554f343b4675b30172ac301638d350db34a5/examples/chat/client.go#L16-L38
const (
	// Time allowed to write a message to the peer.
	writeWait = time.Second * 1

	// Time allowed to read the next pong message from the peer.
	pongWait = time.Second * 1

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 1024
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// middleman between websocket and chatroom
type Client struct {
	Uuid        uuid.UUID
	Nickname    string
	CurrentRoom *Room           // the room this client is in
	Connection  *websocket.Conn // connection to the CLIENT
	Send        chan Message    // channel of outbound messages
	KickSignal  chan *Room      // used for when a room kicks/force-exists the client
}

// reads incoming messages from the webclient for relaying to the server
func (c *Client) readSocket() {
	// unregister and disconnect when done reading
	defer func() {
		log.Println(c.Nickname, "closing readSocket")
		c.CurrentRoom.Unregister <- c
		c.CurrentRoom = nil
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
		c.CurrentRoom.Logf("client got message `%s` from %s\n", string(message), c.Nickname)
		sent := Message{
			Uuid:     c.Uuid,
			FromNick: c.Nickname,
			Content:  string(message),
			SentTime: time.Now(),
			RoomName: c.CurrentRoom.RoomName,
		}
		c.CurrentRoom.Broadcast <- sent // send the message to the room
	}
}

// moves messages from the current room to the websocket connection to the webclient
func (c *Client) writeSocket() {
	ticker := time.NewTicker(pingPeriod) // tick every so often
	defer func() {
		log.Println(c.Nickname, "closing writeSocket")
		ticker.Stop()
		c.Connection.WriteControl(websocket.CloseNormalClosure, []byte{}, time.Now().Add(writeWait))
		c.Connection.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Connection.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// room closed the channel
				log.Println(c.Nickname, "room closed channel")
				c.Connection.WriteMessage(websocket.CloseMessage, []byte{})
			}
			w, err := c.Connection.NextWriter(websocket.TextMessage)
			if err != nil {
				// cannot write to the connection
				log.Println(c.Nickname, "cannot write to connection")
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
				log.Println(c.Nickname, "cannot close outbound writer")
				return
			}
		case <-ticker.C:
			// when on tick
			c.Connection.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				// ping to the server failed
				log.Println(c.Nickname, "failed to ping")
				return
			}
		case room := <-c.KickSignal:
			// room wants to kick us out
			log.Println(c.Nickname, "kicked or exited by", room.RoomName)
			return
		}
	}
}

// sends a dm from the server to the web client
func (c Client) ServerDirectMessage(message Message) {
	w, err := c.Connection.NextWriter(websocket.TextMessage)
	if err != nil {
		// cannot write to the connection
		return
	}
	message.IsDirectMessage = true
	message.Content = "(DM) " + message.Content
	// send the content of the message to the client
	// w.Write(message.Content)
	json.NewEncoder(w).Encode(message)
	if err := w.Close(); err != nil {
		// cannot close the writer to the outbound queue
		return
	}
}

// sends a dm from this client to some other client
func (c Client) DirectMessageToOtherClient(other Client, message Message) {
	w, err := other.Connection.NextWriter(websocket.TextMessage)
	if err != nil {
		// cannot write to the connection
		return
	}
	message.Content = fmt.Sprintf("(%s) ", other.Nickname) + message.Content
	message.IsDirectMessage = true
	// send the content of the message to the client
	// w.Write(message.Content)
	json.NewEncoder(w).Encode(message)
	if err := w.Close(); err != nil {
		// cannot close the writer to the outbound queue
		return
	}
}

// handle websocket requests from peers
func ServeWebSocket(room *Room, w http.ResponseWriter, r *http.Request) {
	// convert http to websocket
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		// failed to convert
		log.Println(err)
		return
	}
	nickname := r.URL.Query().Get("nickname")
	nickname = strings.ReplaceAll(nickname, " ", "_")
	room.Logf("Got client with nickname `%s`", nickname)

	if room.NicknameAlreadyExists(nickname) {
		room.Logf("Nickname %v already exists, changing nickname...\n", nickname)
		nickname = fmt.Sprintf("%s_%d", nickname, time.Now().Unix()%int64(len(room.Clients)))
		room.Logf("Nickname is now %s\n", nickname)
	}

	client := &Client{
		Nickname:    string(nickname),
		CurrentRoom: room,
		Connection:  conn,
		Send:        make(chan Message),
		Uuid:        uuid.New(),
		KickSignal:  make(chan *Room),
	}
	// enter the room
	client.CurrentRoom.Register <- client

	// async getting and writing of messages
	go client.readSocket()
	go client.writeSocket()
}
