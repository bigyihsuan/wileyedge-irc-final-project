# Terminal Client

## Running

```sh
cd /path/to/repo/src/terminal-client
go run terminal-client.go [--host url:port] [--room roomname] [--nick nickname]
```

### Flags

- `--host`: specifies the address and port of the server to connect to. Default is `localhost:8080`.
- `--room`: specifies the room to initially join in. Default is `main`.
- `--nick`: specifies a nickname to use. If not provided, will ask for a nickname on program launch.

## Functionality

The terminal client is a command-line-based client for the IRC-like chat service.
It supports reading and writing messages to a server.

## Program Stucture and Flow

The terminal client performs the following:

- It receives its required arguments from the command line; it prompts the user for some information otherwise (specifically, nicknames).
- The client then attempts to connect via Websockets to the server.
- If successsful, it begins sending to and receiving messages from the server.
- When the client receives a message (as JSON), it unpacks it into a `Message` for formatting for output.
- When the user types some text and presses enter, the client sends the message to the server.
- The above two actions are done asynchronously: a user can send and receive messages at the same time.
- When the user sends `/exit` or otherwise halts the client program, the server will close the websocket connection and exit.
