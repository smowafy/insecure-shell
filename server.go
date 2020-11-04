package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

const sock_path string = "/tmp/ish.sock"

/*

  TODO: send received SIGWINCH signal from the client to the server to resize the terminal.
  Maybe we'll need to serialize the new terminal size and send it over.

  // Handle pty size.
  ch := make(chan os.Signal, 1)
  signal.Notify(ch, syscall.SIGWINCH)
  go func() {
      for range ch {
        if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
          log.Printf("error resizing pty: %s", err)
        }
      }
  }()
  ch <- syscall.SIGWINCH // Initial resize.

*/

func acceptConnections(l net.Listener, connectionQueue chan net.Conn) {
	for {
		c, err := l.Accept()

		// listener is closed, the function should exit
		if err != nil {
			return
		}

		log.Printf("[accept connection] enqueueing a connection\n")

		connectionQueue <- c
	}
}

func main() {
	l, err := net.Listen("unix", sock_path)

	if err != nil {
		panic(err)
	}

	defer func() { _ = l.Close() }() // best effort

	sigChan := make(chan os.Signal)

	signal.Notify(
		sigChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	/*
	   select between a channel of connections and os signal channel, a goroutine that would accept connections and push them to the channel and then call handleConnection
	*/

	connQueue := make(chan net.Conn)

	terminationReportingQueue := make(chan ConnectionHandler)

	go acceptConnections(l, connQueue)

	handlerSet := NewHandlerSet()

	for {
		select {
		case conn := <-connQueue:
			log.Printf("[main] connection from the connection queue\n")
			handler := NewConnectionHandler(conn, terminationReportingQueue)
			handlerSet.Add(handler)
			log.Printf("[main] added a new handler to the handler set\ncurrent set count: %v\n", len(handlerSet))

		case <-sigChan:
			log.Printf("[main] received signal to terminate\n")
			for handler := range handlerSet {
				log.Printf("[main] terminating handler %v\n", handler)
				// keep consuming errors till the channel is closed
				for err := range handler.Close() {
					log.Print(err)
				}
			}
			return

		case handler := <-terminationReportingQueue:
			log.Printf("[main] handler reporting termination\n")
			for err := range handler.Close() {
				log.Print(err)
			}

			handlerSet.Remove(handler)
			log.Printf("[main] removed a handler from the handler set\ncurrent set count: %v\n", len(handlerSet))
		}
	}

}
