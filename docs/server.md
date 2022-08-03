# Server

## Running

```sh
cd /path/to/repo/src/server
go run main.go [--addr url:port]
```

### Flags

- `--addr`: specifies the url and port of this server instance.

## Functionality

The server handles message passing and broadcasting, room management, and running commands passed from clients.

## Program Stucture

The server has four principal parts: `Client`, `Message`, `Command`, and `Room`.

### Rooms

A room represents a channel in IRC;
clients within a room will broadcast messages to all other clients in the room.

### Clients

Clients are "middlemen", sitting between the actual client and the server's rooms.
It represents a raw websocket connection to a remote client.

### Message

A message represents the text that clients and servers send and receive.
Each message knows which room it came from, which client it came from, its own contents, whether it is a private message, among other properties.
This is the basic unit of communication between rooms and clients.
Messages are converted into JSON for communcation with clients.

### Commands

Commands are a subset of messages, which start with the slash character `/`.
There are several commands available, ranging from printing help text to making new rooms to list out users.
Commands can take multiple arguments, separated by spaces.

## Program Flow

The server's program flow can be summarized as follows:

- A remote client makes a websocket request for `ws://.../ws/...`
- The server creates a new `Client` to represent that remote client, and registers them in the requested room.
- Whenever the remote client sends a message through its websocket, the `Client` representing them captures that text and packages it into a `Message`. This `Message` is then sent to the destination `Room` for further action.
- If the `Message` is a command, as in it starts with `/`, it is processed into a `Command` and is executed by the room. Command output gets sent as a direct message from the room to the client.
- Otherwise, it is a regular message, and is broadcast to all users in the same room as the source client.
- When a remote client closes their connection, or runs the `/exit` command, they are removed from the room and their connection is closed.
