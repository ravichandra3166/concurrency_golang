package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"network-device/internal/config"
)

// TCPServer represents a concurrent TCP server
type TCPServer struct {
	config        *config.TCPConfig
	deviceID      string
	listener      net.Listener
	connections   map[string]*ClientConnection
	connMutex     sync.RWMutex
	activeConns   int64
	maxConns      int64
	messageQueue  chan Message
	broadcastChan chan []byte
	shutdown      chan struct{}
	wg            sync.WaitGroup
	running       atomic.Bool
}

// ClientConnection represents an active client connection
type ClientConnection struct {
	ID       string
	Conn     net.Conn
	LastSeen time.Time
	SendCh   chan []byte
	ctx      context.Context
	cancel   context.CancelFunc
}

// Message represents a message between clients
type Message struct {
	From    string
	To      string
	Content []byte
	Type    MessageType
}

// MessageType defines the type of message
type MessageType int

const (
	MessageTypeText MessageType = iota
	MessageTypeBroadcast
	MessageTypeSystem
	MessageTypeHeartbeat
)

// NewTCPServer creates a new TCP server instance
func NewTCPServer(cfg *config.TCPConfig, deviceID string) *TCPServer {
	return &TCPServer{
		config:        cfg,
		deviceID:      deviceID,
		connections:   make(map[string]*ClientConnection),
		maxConns:      int64(cfg.MaxConnections),
		messageQueue:  make(chan Message, 1000),
		broadcastChan: make(chan []byte, 100),
		shutdown:      make(chan struct{}),
	}
}

// Start starts the TCP server
func (s *TCPServer) Start(ctx context.Context) error {
	if !s.config.Enabled {
		log.Println("TCP server is disabled")
		return nil
	}

	addr := fmt.Sprintf(":%d", s.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start TCP server on %s: %w", addr, err)
	}

	s.listener = listener
	s.running.Store(true)

	log.Printf("TCP server started on %s (max connections: %d)", addr, s.config.MaxConnections)

	// Start message processor
	s.wg.Add(1)
	go s.messageProcessor()

	// Start broadcast handler
	s.wg.Add(1)
	go s.broadcastHandler()

	// Start connection cleaner
	s.wg.Add(1)
	go s.connectionCleaner()

	// Accept connections
	s.wg.Add(1)
	go s.acceptConnections(ctx)

	return nil
}

// Stop stops the TCP server gracefully
func (s *TCPServer) Stop() error {
	if !s.running.Load() {
		return nil
	}

	log.Println("Stopping TCP server...")
	s.running.Store(false)
	
	close(s.shutdown)
	
	if s.listener != nil {
		s.listener.Close()
	}
	
	// Close all client connections
	s.connMutex.Lock()
	for _, conn := range s.connections {
		conn.cancel()
		conn.Conn.Close()
	}
	s.connMutex.Unlock()
	
	// Wait for all goroutines to finish
	s.wg.Wait()
	
	log.Println("TCP server stopped")
	return nil
}

// acceptConnections accepts incoming connections
func (s *TCPServer) acceptConnections(ctx context.Context) {
	defer s.wg.Done()
	
	for s.running.Load() {
		conn, err := s.listener.Accept()
		if err != nil {
			if !s.running.Load() {
				return // Server is shutting down
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		
		// Check connection limit
		currentConns := atomic.LoadInt64(&s.activeConns)
		if currentConns >= s.maxConns {
			log.Printf("Connection limit reached (%d), rejecting new connection", s.maxConns)
			conn.Close()
			continue
		}
		
		// Handle the connection
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles a single client connection
func (s *TCPServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()
	
	// Create connection context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Generate connection ID
	connID := fmt.Sprintf("%s-%d", conn.RemoteAddr().String(), time.Now().UnixNano())
	
	// Create client connection
	client := &ClientConnection{
		ID:       connID,
		Conn:     conn,
		LastSeen: time.Now(),
		SendCh:   make(chan []byte, 100),
		ctx:      ctx,
		cancel:   cancel,
	}
	
	// Set connection timeout
	if s.config.Timeout > 0 {
		conn.SetDeadline(time.Now().Add(s.config.Timeout))
	}
	
	// Register connection
	s.addConnection(client)
	defer s.removeConnection(connID)
	
	log.Printf("New client connected: %s", connID)
	
	// Send welcome message
	welcomeMsg := fmt.Sprintf("Welcome to %s! Your connection ID: %s\n", s.deviceID, connID)
	client.SendCh <- []byte(welcomeMsg)
	
	// Start sender goroutine
	s.wg.Add(1)
	go s.clientSender(client)
	
	// Read from connection
	buffer := make([]byte, s.config.BufferSize)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		default:
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err := conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					log.Printf("Client %s disconnected", connID)
				} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Printf("Client %s read timeout", connID)
				} else {
					log.Printf("Error reading from client %s: %v", connID, err)
				}
				return
			}
			
			if n > 0 {
				client.LastSeen = time.Now()
				
				// Process the message
				msg := Message{
					From:    connID,
					Content: buffer[:n],
					Type:    MessageTypeText,
				}
				
				select {
				case s.messageQueue <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// clientSender handles sending messages to a client
func (s *TCPServer) clientSender(client *ClientConnection) {
	defer s.wg.Done()
	
	for {
		select {
		case <-client.ctx.Done():
			return
		case <-s.shutdown:
			return
		case data := <-client.SendCh:
			if _, err := client.Conn.Write(data); err != nil {
				log.Printf("Error writing to client %s: %v", client.ID, err)
				return
			}
		}
	}
}

// messageProcessor processes incoming messages
func (s *TCPServer) messageProcessor() {
	defer s.wg.Done()
	
	for {
		select {
		case <-s.shutdown:
			return
		case msg := <-s.messageQueue:
			s.processMessage(msg)
		}
	}
}

// processMessage processes a single message
func (s *TCPServer) processMessage(msg Message) {
	switch msg.Type {
	case MessageTypeText:
		// Echo message back to sender with timestamp
		response := fmt.Sprintf("[%s] Echo: %s", time.Now().Format("15:04:05"), string(msg.Content))
		s.sendToClient(msg.From, []byte(response))
		
		// Broadcast to other clients
		broadcast := fmt.Sprintf("[%s] %s: %s", time.Now().Format("15:04:05"), msg.From, string(msg.Content))
		s.broadcastChan <- []byte(broadcast)
		
	case MessageTypeBroadcast:
		s.broadcastChan <- msg.Content
		
	case MessageTypeHeartbeat:
		// Update last seen time
		s.connMutex.RLock()
		if client, exists := s.connections[msg.From]; exists {
			client.LastSeen = time.Now()
		}
		s.connMutex.RUnlock()
	}
}

// broadcastHandler handles broadcasting messages to all clients
func (s *TCPServer) broadcastHandler() {
	defer s.wg.Done()
	
	for {
		select {
		case <-s.shutdown:
			return
		case data := <-s.broadcastChan:
			s.broadcastToAll(data)
		}
	}
}

// sendToClient sends a message to a specific client
func (s *TCPServer) sendToClient(clientID string, data []byte) {
	s.connMutex.RLock()
	client, exists := s.connections[clientID]
	s.connMutex.RUnlock()
	
	if exists {
		select {
		case client.SendCh <- data:
		default:
			log.Printf("Client %s send channel full, dropping message", clientID)
		}
	}
}

// broadcastToAll broadcasts a message to all connected clients except sender
func (s *TCPServer) broadcastToAll(data []byte) {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()
	
	for _, client := range s.connections {
		select {
		case client.SendCh <- data:
		default:
			log.Printf("Client %s send channel full, dropping broadcast", client.ID)
		}
	}
}

// addConnection adds a new connection to the server
func (s *TCPServer) addConnection(client *ClientConnection) {
	s.connMutex.Lock()
	s.connections[client.ID] = client
	s.connMutex.Unlock()
	
	atomic.AddInt64(&s.activeConns, 1)
}

// removeConnection removes a connection from the server
func (s *TCPServer) removeConnection(clientID string) {
	s.connMutex.Lock()
	if _, exists := s.connections[clientID]; exists {
		delete(s.connections, clientID)
		atomic.AddInt64(&s.activeConns, -1)
	}
	s.connMutex.Unlock()
	
	log.Printf("Client %s disconnected (active: %d)", clientID, atomic.LoadInt64(&s.activeConns))
}

// connectionCleaner periodically cleans up stale connections
func (s *TCPServer) connectionCleaner() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.cleanStaleConnections()
		}
	}
}

// cleanStaleConnections removes connections that haven't been seen recently
func (s *TCPServer) cleanStaleConnections() {
	now := time.Now()
	staleThreshold := 5 * time.Minute
	
	s.connMutex.Lock()
	var staleConnections []string
	for id, client := range s.connections {
		if now.Sub(client.LastSeen) > staleThreshold {
			staleConnections = append(staleConnections, id)
		}
	}
	s.connMutex.Unlock()
	
	for _, id := range staleConnections {
		log.Printf("Removing stale connection: %s", id)
		s.connMutex.Lock()
		if client, exists := s.connections[id]; exists {
			client.cancel()
			client.Conn.Close()
		}
		s.connMutex.Unlock()
	}
}

// GetStats returns server statistics
func (s *TCPServer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"active_connections": atomic.LoadInt64(&s.activeConns),
		"max_connections":    s.maxConns,
		"port":              s.config.Port,
		"enabled":           s.config.Enabled,
		"running":           s.running.Load(),
	}
}