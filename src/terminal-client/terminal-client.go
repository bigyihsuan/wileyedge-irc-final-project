package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Message struct {
	Uuid     uuid.UUID
	FromNick string
	Content  string
	SentTime time.Time
}

func (m Message) String() string {
	return "[" + m.SentTime.Local().Format("15:04:05") + "] " + "<" + m.FromNick + "> " + m.Content
}

// location of the server
var address = flag.String("addr", "localhost:8080", "address of the server")

// starting room name
var roomName = flag.String("room", "main", "starting room")

// https://github.com/gorilla/websocket/blob/master/examples/echo/client.go

func main() {
	flag.Parse()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	reader := bufio.NewReader(os.Stdin)
	// enter a nickname
	fmt.Print("Enter nickname: ")
	nickname, _ := reader.ReadString('\n')
	nickname = strings.TrimSpace(nickname)
	nickname = url.QueryEscape(nickname)

	// create url for request
	serverUrl := url.URL{
		Scheme:   "ws", // websockets uses `ws` scheme
		Host:     *address,
		Path:     "/ws/" + *roomName,
		RawQuery: "nickname=" + nickname, // send nickname to the server (as a query)
	}
	log.Printf("Connecting to `%s` as `%s`\n", serverUrl.String(), nickname)

	// create a websocket connection to the server
	conn, _, err := websocket.DefaultDialer.Dial(serverUrl.String(), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	done := make(chan struct{})

	// start sending-receiving messages
	// receiving messages
	go func() {
		defer close(done) // notify the outside world that we're done getting messages
		for {
			_, message, err := conn.ReadMessage() // leech off of the broadcast channel
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Println("read: ", err)
					return
				} else {
					return
				}
			}
			var m Message
			json.Unmarshal(message, &m)
			fmt.Println(m.String())
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	messageInput := make(chan string) // so the user can async input messages

	go func(ch chan string) {
		defer close(ch)
		reader := bufio.NewReader(os.Stdin)
		for {
			s, err := reader.ReadString('\n')
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					return
				} else {
					log.Fatal(err)
					return
				}
			}
			fmt.Fprint(os.Stdin, "\r")
			ch <- s
		}
	}(messageInput)

stdinloop:
	for {
		select {
		case <-done:
			// finish when no more messages
			return
		case <-interrupt:
			log.Println("interrupt")
			// close connection
			err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
				// nop
			case <-time.After(time.Second):
				// nop
			}
			return
		case content, ok := <-messageInput:
			// get the message content from the user
			if !ok {
				break stdinloop
			} else {
				content = strings.TrimSpace(content)
				conn.WriteMessage(websocket.TextMessage, []byte(content))
			}
		case <-ticker.C:
			// nop so we can wait for input
		}
	}

}
