package main

import (
	"flag"
	"fmt"
	"irc-final-project/chatroom"
	"log"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type server_init_info struct {
	Name           string                     `json:"Name"`
	Uuid           uuid.UUID                  `json:"Uuid"`
	AvailableRooms map[chatroom.RoomInfo]bool `json:"AvailableRooms"`
}

var addr = flag.String("addr", ":8080", "http service address")
var hub = flag.String("hub", "", "the address of the hub server")
var serverName = flag.String("name", "server", "the name of this server")

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
	// pre-room run setup
	flag.Parse()

	var thisServer chatroom.Server

	serverUuid, _ := uuid.NewUUID()

	thisServer.Name = *serverName
	thisServer.Uuid = serverUuid
	thisServer.LocalRooms = make(map[chatroom.RoomInfo]*chatroom.Room)
	thisServer.ChildServers = make(map[chatroom.ServerInfo]*chatroom.RemoteServer)

	r := mux.NewRouter()
	main := thisServer.NewRoom(*serverName + "_main")
	// if hub arg is given, then connect to the hub's server
	if *hub != "" {
		hubUrl := url.URL{
			Scheme: "ws",
			Host:   *hub,
			Path:   fmt.Sprintf("/ws/server/%s/%s", url.PathEscape(main.Uuid.String()), *serverName),
		}
		log.Println("connecting to hub:", *hub, hubUrl.String())

		// establish connection with the hub
		conn, _, err := websocket.DefaultDialer.Dial(hubUrl.String(), nil)
		if err != nil {
			log.Fatal(err)
			return
		}
		// get the uuid and name of the hub
		var info server_init_info
		conn.ReadJSON(&info)
		log.Println(info)
		serverInfo := chatroom.ServerInfo{Name: info.Name, Uuid: info.Uuid, Conn: conn}
		thisServer.ParentServer = chatroom.NewRemoteServer(serverInfo)
		conn.WriteJSON(server_init_info{
			Name:           thisServer.Name,
			Uuid:           thisServer.Uuid,
			AvailableRooms: thisServer.AvailableRooms(),
		})
		log.Println("new parent server", serverInfo.Name, serverInfo.Uuid)
	}

	// start expecting messages to/from other servers
	// go thisServer.RelayMessages()
	// start up the main room
	go main.Run()

	// serve the web page for the web client
	r.HandleFunc("/", serveHome)

	// server-only connection to communicate between servers
	// the `{uuid}` field is the uuid of the server instance that's connecting to this one
	r.HandleFunc("/ws/server/{uuid}/{serverName}", func(w http.ResponseWriter, r *http.Request) {
		log.Println("/ws/server/{uuid}/{serverName}", r.URL)
		// send over the needed information to the remote server
		conn, err := chatroom.Upgrader.Upgrade(w, r, nil)
		defer func() { conn.Close() }()
		if err != nil {
			log.Println(err)
			return
		}
		conn.WriteJSON(server_init_info{
			Name:           *serverName,
			Uuid:           serverUuid,
			AvailableRooms: thisServer.AvailableRooms(),
		})
		// get information of the child
		var info server_init_info
		conn.ReadJSON(&info)
		log.Println(info)
		serverInfo := chatroom.ServerInfo{Name: info.Name, Uuid: info.Uuid, Conn: conn}
		thisServer.ChildServers[serverInfo] = chatroom.NewRemoteServer(serverInfo)
		log.Println("new child server", serverInfo.Name, serverInfo.Uuid)
	})

	// for private messaging people
	// sourcename is you
	// targetname is the person you're sending the dms to
	r.HandleFunc("/ws/client/{sourcename}/{targetname}", func(w http.ResponseWriter, r *http.Request) {
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
			thisServer.LocalRooms[room.RoomInfo()] = room
			go room.Run()
			chatroom.ServeWebSocket(room, w, r)
		}
	})

	// actual websocket connection for the client to communicate with the server
	r.HandleFunc("/ws/client/{servername}", func(w http.ResponseWriter, r *http.Request) {
		log.Println("/ws/{servername}", r.URL)
		vars := mux.Vars(r)
		log.Println(vars)
		var room *chatroom.Room
		if ro, ok := chatroom.ActiveRooms[chatroom.StringToRoomUUID[vars["servername"]]]; !ok {
			log.Println("main(): making new room", vars["servername"])
			room = chatroom.NewRoom(vars["servername"])
			thisServer.LocalRooms[room.RoomInfo()] = room
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
