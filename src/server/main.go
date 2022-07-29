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
	// serve the web page for the web client
	r.HandleFunc("/", serveHome)
	// r.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
	// 	log.Println("/ws", r.URL)
	// 	chatroom.ServeWebSocket(main, w, r)
	// })

	// for private messaging people
	// sourcename is you
	// targetname is the person you're sending the dms to
	r.HandleFunc("/ws/{sourcename}/{targetname}", func(w http.ResponseWriter, r *http.Request) {
		log.Println("/ws/{servername}", r.URL)
		vars := mux.Vars(r)
		log.Println(vars)
		// check if a room for this pair exists
		// this room is called `targetname-sourcename` and `sourcename-targetname`
		// both names point to the same room
		srctar := vars["sourcename"] + "-" + vars["targetname"]
		tarsrc := vars["targetname"] + "-" + vars["sourcename"]
		srctar_uuid, srctar_ok := chatroom.StringToRoomUUID[srctar]
		tarsrc_uuid, tarsrc_ok := chatroom.StringToRoomUUID[tarsrc]

		switch {
		case tarsrc_ok && srctar_ok:
			// both exist, send the client to the tarsrc room
			tarsrc_room := chatroom.ActiveRooms[tarsrc_uuid]
			go tarsrc_room.Run()
			chatroom.ServeWebSocket(tarsrc_room, w, r)
		case tarsrc_ok && !srctar_ok:
			// only tarsrc exists, set the keys appropiately
			tarsrc_room := chatroom.ActiveRooms[tarsrc_uuid]
			chatroom.StringToRoomUUID[srctar] = tarsrc_room.Uuid
			go tarsrc_room.Run()
			chatroom.ServeWebSocket(tarsrc_room, w, r)
		case !tarsrc_ok && srctar_ok:
			// only srctar exists, set the keys appropiately
			srctar_room := chatroom.ActiveRooms[srctar_uuid]
			chatroom.StringToRoomUUID[tarsrc] = srctar_room.Uuid
			go srctar_room.Run()
			chatroom.ServeWebSocket(srctar_room, w, r)
		default:
			// neither key exists, create a new room
			room := chatroom.NewRoom(tarsrc)
			chatroom.ActiveRooms[room.Uuid] = room
			chatroom.StringToRoomUUID[tarsrc] = room.Uuid
			chatroom.StringToRoomUUID[srctar] = room.Uuid
			go room.Run()
			chatroom.ServeWebSocket(room, w, r)
		}
	})

	// actual websocket connection for the client to communicate with the server
	r.HandleFunc("/ws/{servername}", func(w http.ResponseWriter, r *http.Request) {
		log.Println("/ws/{servername}", r.URL)
		vars := mux.Vars(r)
		log.Println(vars)
		var room *chatroom.Room
		if ro, ok := chatroom.ActiveRooms[chatroom.StringToRoomUUID[vars["servername"]]]; !ok {
			log.Println("main(): making new room", vars["servername"])
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
