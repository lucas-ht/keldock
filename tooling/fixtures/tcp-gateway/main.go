// Package main forwards a disposable local port without terminating TLS.
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "expected exact disposable cluster target")
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", ":443")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for {
		client, err := listener.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		go proxy(client, os.Args[1])
	}
}

func proxy(client net.Conn, target string) {
	defer func() { _ = client.Close() }()
	upstream, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return
	}
	defer func() { _ = upstream.Close() }()
	if client.SetDeadline(time.Now().Add(2*time.Minute)) != nil || upstream.SetDeadline(time.Now().Add(2*time.Minute)) != nil {
		return
	}
	done := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(upstream, client)
		if stream, ok := upstream.(*net.TCPConn); ok {
			_ = stream.CloseWrite()
		}
		done <- struct{}{}
	}()
	_, _ = io.Copy(client, upstream)
	if stream, ok := client.(*net.TCPConn); ok {
		_ = stream.CloseWrite()
	}
	<-done
}
