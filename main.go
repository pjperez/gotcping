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
	countPtr := flag.Int("count", 10, "Number of requests to send")
	timeoutPtr := flag.Int("timeout", 5, "Timeout for each request, in seconds")

	flag.Parse()

	//args := flag.Args()
	host := *hostPtr
	port := *portPtr
	count := *countPtr
	timeout := *timeoutPtr

	_, err := net.LookupIP(host)
	if err != nil {
		fmt.Println("error: unknown host")
		os.Exit(2)
	}

	/*addr := fmt.Sprintf("%s:%d", host, port)*/

	ping(host, port, count, timeout)

	/*	_, err = net.DialTimeout("tcp", addr, time.Second*time.Duration(timeout))
		if err != nil {
			fmt.Println(fmt.Sprintf("%s port %d closed.", host, port))
			os.Exit(0)
		}

		fmt.Println(fmt.Sprintf("%s port %d open.", host, port))*/
}

func ping(host string, port int, count int, timeout int) {

	addr := fmt.Sprintf("%s:%d", host, port)

	for i := 1; count >= i; i++ {
		timeStart := time.Now()
		_, err := net.DialTimeout("tcp", addr, time.Second*time.Duration(timeout))
		responseTime := time.Since(timeStart)
		if err != nil {
			fmt.Println(fmt.Sprintf("%s port %d closed.", host, port))
			os.Exit(0)
		}

		fmt.Println(fmt.Sprintf("Connected to %s:%d, RTT=%.2fms", host, port, float32(responseTime)/1e6))
		time.Sleep(1e9)
	}
}
