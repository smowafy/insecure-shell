package main

import (
  "net"
  "os"
  "os/exec"
  "os/signal"
  "github.com/creack/pty"
  "io"
  "log"
  "syscall"
)

const sock_path string = "/tmp/ish.sock"

func main() {
  l, err := net.Listen("unix", sock_path)

  if err != nil {
    panic(err)
  }

  defer func() { _ = l.Close() }() // best effort

  c, err := l.Accept()

  if err != nil {
    panic(err)
  }

  defer func() { _ = c.Close() }() // best effort


  cmd := exec.Command("bash")

  ptmx, err := pty.Start(cmd)

  if err != nil {
    panic(err)
  }

  defer func() { _ = ptmx.Close() }() // Best effort.

  /*

  TODO: send received SIGWINCH signal from the client to the daemon to resize the terminal.
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

  go func() {
    _, _ = io.Copy(c, ptmx)
    log.Printf("from the goroutine, copying stdin")
  }()

  log.Printf("from the main thread, before copying stdout")

  sigChan := make(chan os.Signal, 1)

  signal.Notify(sigChan, syscall.SIGINT)

  go func(chn chan os.Signal, conn net.Conn) {
    s := <-sigChan
    log.Printf("received signal %v\n", s)

    log.Printf("is it interrupt %v\n", (s == syscall.SIGINT))

    conn.Close()
  }(sigChan, c)

  _, _ = io.Copy(ptmx, c)

  log.Printf("from the main thread, after copying stdout")

}
