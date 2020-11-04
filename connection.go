package main

import (
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
}

type handler struct {
	conn net.Conn
	ptmx *os.File
	cmd  *exec.Cmd
	// channel for reporting termination event
	trc chan<- ConnectionHandler
}

func NewConnectionHandler(connection net.Conn, reportTermChannel chan ConnectionHandler) ConnectionHandler {
	h := handler{
		conn: connection,
		trc:  reportTermChannel,
	}

	go h.handleConnection()

	return &h
}

func (h *handler) Close() <-chan error {
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

func (h *handler) ensureProcessExited() {
	if h.cmd.ProcessState != nil && h.cmd.ProcessState.Exited() {
		log.Printf("[handler][ensure process exited] already exited, returning\n")
		return
	}


	log.Printf("[handler][ensure process exited] process PID: %v\n", h.cmd.Process.Pid)

	// close associated pty
	err := h.ptmx.Close()

	if err != nil {
		log.Printf("[handler][ensure process exited] closing ptmx failed, err %v\n", err)
	}

	// send a best effort TERM signal to the process
	err = h.cmd.Process.Signal(syscall.SIGTERM)

	log.Printf("[handler][ensure process exited] sent a termination signal to the process, err: %v\n", err)

	log.Printf("[handler][ensure process exited] process PID: %v\n", h.cmd.Process.Pid)

	processExit := make(chan struct{})

	go func() {
		h.cmd.Process.Wait()

		log.Printf("[handler][ensure][goroutine] done waiting for process\n")

		processExit <- struct{}{}
	}()

	select {
	// grace period of 5 seconds
	case <-time.After(5 * time.Second):
		log.Printf("[handler][ensure process exited] grace period of %v elapsed, force kill if still executing\n", (5000 * time.Millisecond))

		if h.cmd.ProcessState == nil || !h.cmd.ProcessState.Exited() {
			log.Printf("[handler][ensure process exited] attempting to kill process process\n")
			h.cmd.Process.Kill()
		}

	case <-processExit:
		break
	}
}

func (h *handler) handleConnection() {
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
		log.Printf("[handler] copied %d bytes to conn\n", n)
	}()
	go func() {
		// TODO: propagate error
		n, _ := io.Copy(h.ptmx, h.conn)
		log.Printf("[handler] copied %d bytes to to ptmx\n", n)
	}()

	go func() {
		h.cmd.Process.Wait()
		log.Printf("[handler] process exited, submitting request for termination")
		// handler passes itself to the termination queue
		h.trc <- h
	}()
}
