# TODO: Minimum Requirements

## Server

- [x] Implement server in Go (gRPC, REST, Kakfa, etc) `(REST)`
- [x] Server accepts incoming messages from clients
- [ ] Server accepts incoming messages from other servers
  - Coordinate with your classmates to put all servers on the same net, so that that every server can communicate with every other server.
  - You may use an EC2 instance to host the server, but this is not a requirement if you can host the server another way.
- [ ] The server relays information to and from other servers on the same net.

## Client

- [x] Ask for a nickname
- [x] Usernames are unique to a session
- [x] Clients should have a globally unique ID
- [x] Non-unique usernames will be made unique `(takes current unix time and mods it against number of people in the room)`
- [ ] Display list of available servers, ***including classmates' servers***
- [ ] Allow the user to connect to any of the servers

## Activities

- [x] View all available channels
- [x] Create a new channel
- [x] Join 1 or more chanels and talk `(can only talk in 1 channel at a time)`
- [x] Make PMs with one other user

## Additional

```md
Task 1) in server generate new UUID for every client and redirect it to every message (done)
Task 2) send message to selected Client (done)
```

## Personal Notes

- `Client.Connection` is the server's connection to the web client
- To switch rooms, will need to change client's current room, not their connection
- Need to be able to run multiple instances of the server
- Each server instance needs to be able to talk to each other
- All servers' rooms are a shared pool of rooms
