package main

import (
	"flag"
	"irc/chatroom"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

var addr = flag.String("addr", ":8080", "http service address")

func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println("serveHome", r.URL)
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "home.html")
}

func main() {
	flag.Parse()
	r := mux.NewRouter()
	main := chatroom.NewRoom("main")
	go main.Run()
	r.HandleFunc("/", serveHome)
	// r.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
	// 	log.Println("/ws", r.URL)
	// 	chatroom.ServeWebSocket(main, w, r)
	// })
	r.HandleFunc("/ws/{servername}", func(w http.ResponseWriter, r *http.Request) {
		log.Println("/ws/{servername}", r.URL)
		vars := mux.Vars(r)
		log.Println(vars)
		var room *chatroom.Room
		if ro, ok := chatroom.ActiveRooms[chatroom.StringToRoomUUID[vars["servername"]]]; !ok {
			log.Println("making new room", vars["servername"])
			room = chatroom.NewRoom(vars["servername"])
		} else {
			room = ro
		}
		go room.Run()
		chatroom.ServeWebSocket(room, w, r)
	})
	err := http.ListenAndServe(*addr, r)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
