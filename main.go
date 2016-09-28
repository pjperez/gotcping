package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {

	hostPtr := flag.String("host", "bing.com", "Host or IP address to test")
	portPtr := flag.Int("port", 80, "Port number to query")
	//countPtr := flag.Int("count", 10, "Number of requests to send")
	timeoutPtr := flag.Int("timeout", 5, "Timeout for each request, in seconds")

	flag.Parse()

	//args := flag.Args()
	host := *hostPtr
	port := *portPtr
	//count := *countPtr
	timeout := *timeoutPtr

	/*	port, err := strconv.Atoi(args[1])
		if err != nil || port < 1 || port > 65535 {
			fmt.Println(fmt.Sprintf("Argument [%s] was not correct, <port> must be a positive integer in the range 1 - 65535", args[1]))
			os.Exit(2)
		}*/

	_, err := net.LookupIP(host)
	if err != nil {
		fmt.Println("error: unknown host")
		os.Exit(2)
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	_, err = net.DialTimeout("tcp", addr, time.Second*time.Duration(timeout))
	if err != nil {
		fmt.Println(fmt.Sprintf("%s port %d closed.", host, port))
		os.Exit(0)
	}

	fmt.Println(fmt.Sprintf("%s port %d open.", host, port))
}

// func ping(host, port, count, timeout) {}
