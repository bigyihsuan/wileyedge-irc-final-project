# TODO: Minimum Requirements

## Server

- [x] Implement server in Go (gRPC, REST, Kakfa, etc)
- [x] Server accepts incoming messages from clients
- [ ] Server accepts incoming messages from other servers
- [ ] Server relays info to and from other servers on the same network (?)

## Client

- [x] Ask for a nickname
- [x] Usernames are unique to a session
- [x] Clients should have a globally unique ID
- [x] Non-unique usernames will be made unique `(takes current unix time and mods it against number of people in the room)`
- [/] Display list of available servers, ***including classmates' servers*** `(currently only local servers)`
- [/] Allow the user to connect to any of the servers `(currently only local servers)`

## Activities

- [x] View all available channels
- [x] Create a new channel
- [x] Join 1 or more chanels and talk `(can only join 1 channel at a time)`
- [x] Make PMs with one other user

## Additional

```md
Task 1) in server generate new UUID for every client and redirect it to every message
Task 2) send message to selected Client
```

## Personal Notes

- `Client.Connection` is the server's connection to the web client
- To switch rooms, will need to change client's current room, not their connection
