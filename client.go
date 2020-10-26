package main

import(
  "os"
  "io"
  "net"
  "golang.org/x/crypto/ssh/terminal"
)

const sock_path string = "/tmp/ish.sock"

func main() {

  c, err := net.Dial("unix", sock_path)

  if err != nil {
    panic(err)
  }

  // Set stdin in raw mode.
  oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))

  if err != nil {
    panic(err)
  }

  defer func() { _ = terminal.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

  // Copy stdin to the pty and the pty to stdout.
  go func() { _, _ = io.Copy(c, os.Stdin) }()
  _, _ = io.Copy(os.Stdout, c)
}


