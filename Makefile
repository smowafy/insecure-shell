all: client server

client: client.go
	go build -race -o client client.go

server: server.go connection.go utils.go packet.go
	go build -race -o server ./server.go ./connection.go ./utils.go ./packet.go
