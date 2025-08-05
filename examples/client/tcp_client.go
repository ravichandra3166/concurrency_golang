package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run tcp_client.go <server_address:port>")
		fmt.Println("Example: go run tcp_client.go localhost:8080")
		os.Exit(1)
	}

	serverAddr := os.Args[1]
	
	// Connect to the server
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", serverAddr, err)
	}
	defer conn.Close()

	fmt.Printf("Connected to %s\n", serverAddr)
	fmt.Println("Type messages and press Enter to send. Type 'quit' to exit.")

	// Start a goroutine to read responses from server
	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Printf("Server: %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading from server: %v", err)
		}
	}()

	// Read input from user and send to server
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("You: ")
		if !scanner.Scan() {
			break
		}
		
		text := strings.TrimSpace(scanner.Text())
		if text == "quit" {
			break
		}
		
		if text == "" {
			continue
		}

		// Send message to server
		_, err := conn.Write([]byte(text + "\n"))
		if err != nil {
			log.Printf("Error sending message: %v", err)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading input: %v", err)
	}

	fmt.Println("Disconnecting...")
}