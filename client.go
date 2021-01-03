package main

import (
	"github.com/creack/pty"
	"golang.org/x/crypto/ssh/terminal"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

const sock_path string = "/tmp/ish.sock"

func ForwardData(w io.Writer, r io.Reader) {
	var count int
	var err error

	buf := make([]byte, 8)

	for {
		count, err = r.Read(buf)

		if err != nil {
			if err != io.EOF {
				panic(err)
			}
		}

		if count == 0 {
			break
		}

		packet := make([]byte, 2)

		packet[0] = 0
		packet[1] = byte(count)

		packet = append(packet, buf[:count]...)

		w.Write(packet)
	}
}

func ForwardWindowSize(w io.Writer, ptmx *os.File) {
	sz, err := pty.GetsizeFull(ptmx)

	if err != nil {
		panic(err)
	}

	log.Printf("[ForwardWindowSize] Window size received: %v\n", sz)

	packet := make([]byte, 2)

	packet[0] = 1
	packet[1] = 8

	// big endian
	packet = append(packet, byte(sz.Rows>>8), byte(sz.Rows))
	packet = append(packet, byte(sz.Cols>>8), byte(sz.Cols))
	packet = append(packet, byte(sz.X>>8), byte(sz.X))
	packet = append(packet, byte(sz.Y>>8), byte(sz.Y))

	log.Printf("[ForwardWindowSize] sending packet with payload: %v\n", packet[2:])

	w.Write(packet)
}

func main() {
	c, err := net.Dial("unix", sock_path)
	defer c.Close()

	if err != nil {
		panic(err)
	}

	log.Printf("[client] dial successful\n")

	// Set stdin in raw mode.
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))

	if err != nil {
		panic(err)
	}

	defer func() {
		log.Printf("[client] raw mode off\n")
		_ = terminal.Restore(int(os.Stdin.Fd()), oldState)
	}() // Best effort.

	go func() {
		ForwardData(c, os.Stdin)
	}()

	go func() {
		_, _ = io.Copy(os.Stdout, c)
	}()

	sigChan := make(chan os.Signal)

	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGWINCH)

	for {
		sig := <-sigChan

		switch sig {
		case syscall.SIGTERM:
			break
		case syscall.SIGWINCH:
			ForwardWindowSize(c, os.Stdin)
		}
	}
}
