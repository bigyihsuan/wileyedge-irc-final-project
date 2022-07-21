package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// location of the server
var address = flag.String("server address", "localhost:8080", "address of the server")

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
		Path:     "/ws",
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
	go func() {
		defer close(done) // notify the outside world that we're done getting messages
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("read: ", err)
				return
			}
			log.Printf("received: %s", message)
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// start sending-receiving messages
	// receiving messages

	for {
		select {
		case <-done:
			// finish when no more messages
			return
		// case t := <-ticker.C:
		// send a message containing the current time to the server
		// err := conn.WriteMessage(websocket.TextMessage, []byte(t.String()))
		// if err != nil {
		// 	log.Println("write:", err)
		// 	return
		// }
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
		}
	}

}
