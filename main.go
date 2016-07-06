package main

import (
  "os"
  "fmt"
  "net"
  "flag"
  "time"
  "strconv"
)

func usage(filename string) {
  fmt.Println(fmt.Sprintf("Usage: %s [-t timeout_second] <host> <port>", filename))
}

func main() {
  filename := os.Args[0]

  var timeout = flag.Int("t", 10, "timeout second")
  flag.Parse()
  args := flag.Args()
  if len(args) < 2 {
    fmt.Println("Argument Miss")
    usage(filename)
    os.Exit(2)
  }

  port, err := strconv.Atoi(args[1])
  if err != nil || port < 1 || port > 65535 {
    fmt.Println(fmt.Sprintf("Argument [%s] was not correct, <port> must be a positive integer in the range 1 - 65535", args[1]))
    os.Exit(2)
  }

  host := args[0]
  _, err = net.LookupIP(host)
  if err != nil {
    fmt.Println("error: unknown host")
    os.Exit(2)
  }

  addr := fmt.Sprintf("%s:%d", host, port)

  _, err = net.DialTimeout("tcp", addr, time.Second * time.Duration(*timeout))
  if err != nil {
    fmt.Println(fmt.Sprintf("%s port %d closed.", host, port))
    os.Exit(0)
  }

  fmt.Println(fmt.Sprintf("%s port %d open.", host, port))
}
