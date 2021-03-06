package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/awnumar/memguard"
	"log"
	"loki/config"
	"net"
	"os"
	"os/signal"
	"syscall"
)

var masterkey *memguard.LockedBuffer

func main() {
	setupLogging()

	socketFile := config.GetSocketfilePath()

	log.Printf("Starting key server on file: %s\n", socketFile)

	key, err := readSecretFromStdin()

	if err != nil {
		panic(err)
	}

	masterkey, err = memguard.NewMutableFromBytes(key)

	if err != nil {
		panic(err)
	}

	if _, err := os.Stat(socketFile); err == nil {
		os.Remove(socketFile)
	}

	ln, err := net.Listen("unix", socketFile)
	if err != nil {
		log.Fatal("Listen error: ", err)
	}

	// setting the correct permissions to the socket file
	if err := os.Chmod(socketFile, 0700); err != nil {
		log.Fatal("Error setting filemode: ", err)
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func(ln net.Listener, c chan os.Signal) {
		sig := <-c
		log.Printf("Caught signal %s: shutting down.", sig)
		ln.Close()
		os.Exit(0)
	}(ln, sigc)

	for {
		log.Printf("Accepting connections")
		fd, err := ln.Accept()
		if err != nil {
			log.Fatal("Accept error: ", err)
		}

		go keyServer(fd)
	}
}

func setupLogging() {
	f, err := os.OpenFile(config.AgentLogfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening file: %v", err)
		panic(err)
	}
	log.SetOutput(f)
}

func readSecretFromStdin() ([]byte, error) {
	reader := bufio.NewReader(os.Stdin)
	data := make([]byte, config.KeyLength)
	n, err := reader.Read(data)

	if err != nil {
		return []byte{}, errors.New("Could not read data from stdin")
	}

	if n != config.KeyLength {
		return []byte{}, errors.New("Could not read all bytes, but only : " + string(data))
	}
	return data, nil
}

func keyServer(c net.Conn) {

	buf := make([]byte, 512)
	nr, err := c.Read(buf)

	if err != nil {
		log.Fatal("Read error: ", err)
		return
	}

	request := string(buf[0:nr])
	log.Println("Server got:", request)

	if request == config.RequestMagic {
		c.Write(masterkey.Buffer())
	} else if request == config.ShutdownMagic {
		log.Println("Shutting down on request")
		c.Close()
		os.Remove(config.GetSocketfilePath())
		os.Exit(0)
	} else {
		log.Println("Bouncing request")
		c.Write([]byte("go away."))
	}

}
