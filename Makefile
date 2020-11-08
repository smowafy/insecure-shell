all: client server

client: client.go
	go build -o client client.go 

server: server.go connection.go utils.go
	go build -o server ./server.go ./connection.go ./utils.go
