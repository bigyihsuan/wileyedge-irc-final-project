package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"

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

	// create url for request
	serverUrl := url.URL{
		Scheme: "ws", // websockets uses `ws` scheme
		Host:   *address,
		Path:   "/ws/" + nickname,
	}
	log.Printf("Connecting to `%s` as `%s`\n", serverUrl.String(), nickname)

	// create a websocket connection to the server
	conn, _, err := websocket.DefaultDialer.Dial(serverUrl.String(), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	// send nickname to the server

	// start sending-receiving messages
}
