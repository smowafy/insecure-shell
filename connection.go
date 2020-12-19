package main

import (
	"fmt"
	"github.com/creack/pty"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type ConnectionHandler interface {
	Close() <-chan error
	WriteToPtmx([]byte) error
	SetPtmxSize(r, c, x, y uint16) error
}

type connectionHandler struct {
	conn net.Conn
	ptmx *os.File
	cmd  *exec.Cmd
	// channel for reporting termination event
	trc chan<- ConnectionHandler
}

// nice trick to make sure `connectionHandler` always implements the interface,
// see this in a comment in this PR
// https://github.com/kubernetes/kubernetes/pull/9905 (I added this here to
// remember this)
var _ ConnectionHandler = &connectionHandler{}

func NewConnectionHandler(connection net.Conn, reportTermChannel chan ConnectionHandler) ConnectionHandler {
	h := connectionHandler{
		conn: connection,
		trc:  reportTermChannel,
	}

	go h.handleConnection()

	return &h
}

func (h *connectionHandler) WriteToPtmx(data []byte) error {
	nwritten, err := h.ptmx.Write(data)

	if err != nil {
		return fmt.Errorf("[WriteToPtmx] exited with error %s\n", err)
	}

	if nwritten != len(data) {
		return fmt.Errorf("[WriteToPtmx] didn't write fully to pty, %d/%d written\n", nwritten, len(data))
	}

	return nil
}

func (h *connectionHandler) SetPtmxSize(row, column, x, y uint16) error {
	ws := pty.Winsize{
		Rows: row,
		Cols: column,
		X:    x,
		Y:    y,
	}

	return pty.Setsize(h.ptmx, &ws)
}

func (h *connectionHandler) Close() <-chan error {
	// to make sure the goroutine will not block waiting for a receiving end
	res := make(chan error, 2)

	go func(c chan error) {
		var err error

		h.conn.Write([]byte("Process terminated, closing connection\n"))

		// in case Close() was issued via a direct call and not upon request for
		// termination.
		h.ensureProcessExited()

		err = h.conn.Close()

		if err != nil {
			c <- err
		}

		close(c)
	}(res)

	return res
}

func (h *connectionHandler) ensureProcessExited() {
	if h.cmd.ProcessState != nil && h.cmd.ProcessState.Exited() {
		log.Printf("[connectionHandler][ensure process exited] already exited, returning\n")
		return
	}

	log.Printf("[connectionHandler][ensure process exited] process PID: %v\n", h.cmd.Process.Pid)

	// close associated pty
	err := h.ptmx.Close()

	if err != nil {
		log.Printf("[connectionHandler][ensure process exited] closing ptmx failed, err %v\n", err)
	}

	// send a best effort TERM signal to the process
	err = h.cmd.Process.Signal(syscall.SIGTERM)

	log.Printf("[connectionHandler][ensure process exited] sent a termination signal to the process, err: %v\n", err)

	log.Printf("[connectionHandler][ensure process exited] process PID: %v\n", h.cmd.Process.Pid)

	processExit := make(chan struct{})

	go func() {
		h.cmd.Process.Wait()

		log.Printf("[connectionHandler][ensure][goroutine] done waiting for process\n")

		processExit <- struct{}{}
	}()

	select {
	// grace period of 5 seconds
	case <-time.After(5 * time.Second):
		log.Printf("[connectionHandler][ensure process exited] grace period of %v elapsed, force kill if still executing\n", (5000 * time.Millisecond))

		if h.cmd.ProcessState == nil || !h.cmd.ProcessState.Exited() {
			log.Printf("[connectionHandler][ensure process exited] attempting to kill process process\n")
			h.cmd.Process.Kill()
		}

	case <-processExit:
		break
	}
}

func (h *connectionHandler) handleConnection() {
	var err error

	h.cmd = exec.Command("bash")

	h.ptmx, err = pty.Start(h.cmd)

	if err != nil {
		log.Print("Failed to initialize pty for process", err)

		return
	}

	go func() {
		// TODO: propagate error
		n, _ := io.Copy(h.conn, h.ptmx)
		log.Printf("[connectionHandler] copied %d bytes to conn\n", n)
	}()
	go func() {
		// TODO: propagate error
		err := <-h.routePackets()
		log.Printf("[connectionHandler] ended receiving packets with error %v\n", err)
	}()

	go func() {
		h.cmd.Process.Wait()
		log.Printf("[connectionHandler] process exited, submitting request for termination")
		// connectionHandler passes itself to the termination queue
		h.trc <- h
	}()
}

func (h *connectionHandler) routePackets() chan error {
	buf := make([]byte, PacketMaxSize)
	chn := make(chan error)

	go func() {
		var err error

		defer func() {
			chn <- err
			close(chn)
		}()

		for {
			count, err := h.conn.Read(buf)

			log.Printf("[RoutePackets] finished reading with count %d, err %v\n", count, err)

			if count == 0 {
				log.Printf("[RoutePackets] received invalid packet; dropping\n")
				return
			}

			if count < 2 { // Type + Size
				log.Printf("[RoutePackets] received invalid packet; dropping\n")
				continue
			}

			log.Printf("[RoutePackets] buf: %v\n", buf)

			packet := Packet{
				Type:    buf[0],
				Size:    buf[1],
				Payload: buf[2 : 2+buf[1]],
			}

			if int(packet.Type) >= len(HandlerTable) {
				log.Printf("[RoutePackets] invalid packet type; dropping\n")
				continue
			}

			HandlerTable[packet.Type](h, packet)

			if err != nil {
				return
			}
		}
	}()

	return chn
}
