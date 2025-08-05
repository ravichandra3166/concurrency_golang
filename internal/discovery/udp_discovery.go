package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"network-device/internal/config"
)

// UDPDiscovery handles device discovery using UDP multicast
type UDPDiscovery struct {
	config         *config.UDPConfig
	discoveryConfig *config.DiscoveryConfig
	deviceInfo     DeviceInfo
	conn           *net.UDPConn
	multicastConn  *net.UDPConn
	peers          map[string]*PeerDevice
	peersMutex     sync.RWMutex
	shutdown       chan struct{}
	wg             sync.WaitGroup
	running        atomic.Bool
	messageHandler MessageHandler
}

// DeviceInfo contains information about this device
type DeviceInfo struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Location string    `json:"location"`
	Address  string    `json:"address"`
	Port     int       `json:"port"`
	LastSeen time.Time `json:"last_seen"`
	Services []string  `json:"services"`
}

// PeerDevice represents a discovered peer device
type PeerDevice struct {
	DeviceInfo
	RemoteAddr *net.UDPAddr `json:"-"`
	TTL        time.Time    `json:"ttl"`
}

// DiscoveryMessage represents a discovery protocol message
type DiscoveryMessage struct {
	Type      MessageType `json:"type"`
	Device    DeviceInfo  `json:"device"`
	Timestamp time.Time   `json:"timestamp"`
	TTL       int         `json:"ttl"`
}

// MessageType defines the type of discovery message
type MessageType string

const (
	MessageTypeAnnounce  MessageType = "announce"
	MessageTypeQuery     MessageType = "query"
	MessageTypeResponse  MessageType = "response"
	MessageTypeHeartbeat MessageType = "heartbeat"
	MessageTypeGoodbye   MessageType = "goodbye"
)

// MessageHandler handles incoming discovery messages
type MessageHandler interface {
	OnDeviceDiscovered(device *PeerDevice)
	OnDeviceLost(deviceID string)
	OnMessage(msg *DiscoveryMessage, from *net.UDPAddr)
}

// DefaultMessageHandler provides default message handling
type DefaultMessageHandler struct{}

func (h *DefaultMessageHandler) OnDeviceDiscovered(device *PeerDevice) {
	log.Printf("Device discovered: %s (%s) at %s", device.Name, device.ID, device.Address)
}

func (h *DefaultMessageHandler) OnDeviceLost(deviceID string) {
	log.Printf("Device lost: %s", deviceID)
}

func (h *DefaultMessageHandler) OnMessage(msg *DiscoveryMessage, from *net.UDPAddr) {
	log.Printf("Discovery message from %s: %s", from.String(), msg.Type)
}

// NewUDPDiscovery creates a new UDP discovery service
func NewUDPDiscovery(udpConfig *config.UDPConfig, discoveryConfig *config.DiscoveryConfig, deviceConfig *config.DeviceConfig) *UDPDiscovery {
	return &UDPDiscovery{
		config:          udpConfig,
		discoveryConfig: discoveryConfig,
		deviceInfo: DeviceInfo{
			ID:       deviceConfig.ID,
			Name:     deviceConfig.Name,
			Type:     deviceConfig.Type,
			Location: deviceConfig.Location,
			Port:     udpConfig.Port,
			LastSeen: time.Now(),
			Services: []string{"tcp", "udp", "discovery"},
		},
		peers:          make(map[string]*PeerDevice),
		shutdown:       make(chan struct{}),
		messageHandler: &DefaultMessageHandler{},
	}
}

// SetMessageHandler sets a custom message handler
func (d *UDPDiscovery) SetMessageHandler(handler MessageHandler) {
	d.messageHandler = handler
}

// Start starts the discovery service
func (d *UDPDiscovery) Start(ctx context.Context) error {
	if !d.discoveryConfig.Enabled {
		log.Println("UDP discovery service is disabled")
		return nil
	}

	// Get local IP address
	localAddr, err := d.getLocalAddr()
	if err != nil {
		return fmt.Errorf("failed to get local address: %w", err)
	}
	d.deviceInfo.Address = localAddr

	// Start UDP server
	if err := d.startUDPServer(); err != nil {
		return fmt.Errorf("failed to start UDP server: %w", err)
	}

	// Start multicast listener
	if err := d.startMulticastListener(); err != nil {
		return fmt.Errorf("failed to start multicast listener: %w", err)
	}

	d.running.Store(true)
	log.Printf("UDP discovery service started on %s:%d", localAddr, d.config.DiscoveryPort)

	// Start announcement routine
	d.wg.Add(1)
	go d.announceRoutine()

	// Start peer cleanup routine
	d.wg.Add(1)
	go d.peerCleanupRoutine()

	// Start query routine
	d.wg.Add(1)
	go d.queryRoutine()

	return nil
}

// Stop stops the discovery service
func (d *UDPDiscovery) Stop() error {
	if !d.running.Load() {
		return nil
	}

	log.Println("Stopping UDP discovery service...")
	d.running.Store(false)

	// Send goodbye message
	d.sendGoodbye()

	close(d.shutdown)

	if d.conn != nil {
		d.conn.Close()
	}
	if d.multicastConn != nil {
		d.multicastConn.Close()
	}

	d.wg.Wait()
	log.Println("UDP discovery service stopped")
	return nil
}

// startUDPServer starts the UDP server for direct communication
func (d *UDPDiscovery) startUDPServer() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", d.config.Port))
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	d.conn = conn

	// Start message handler
	d.wg.Add(1)
	go d.handleUDPMessages(conn)

	return nil
}

// startMulticastListener starts the multicast listener for discovery
func (d *UDPDiscovery) startMulticastListener() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", d.config.MulticastAddress, d.config.DiscoveryPort))
	if err != nil {
		return err
	}

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		return err
	}

	d.multicastConn = conn

	// Start multicast message handler
	d.wg.Add(1)
	go d.handleMulticastMessages(conn)

	return nil
}

// handleUDPMessages handles direct UDP messages
func (d *UDPDiscovery) handleUDPMessages(conn *net.UDPConn) {
	defer d.wg.Done()

	buffer := make([]byte, 1024)
	for d.running.Load() {
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if d.running.Load() {
				log.Printf("Error reading UDP message: %v", err)
			}
			continue
		}

		var msg DiscoveryMessage
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			log.Printf("Error unmarshaling discovery message: %v", err)
			continue
		}

		d.processDiscoveryMessage(&msg, addr)
	}
}

// handleMulticastMessages handles multicast discovery messages
func (d *UDPDiscovery) handleMulticastMessages(conn *net.UDPConn) {
	defer d.wg.Done()

	buffer := make([]byte, 1024)
	for d.running.Load() {
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if d.running.Load() {
				log.Printf("Error reading multicast message: %v", err)
			}
			continue
		}

		var msg DiscoveryMessage
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			log.Printf("Error unmarshaling multicast message: %v", err)
			continue
		}

		// Ignore our own messages
		if msg.Device.ID == d.deviceInfo.ID {
			continue
		}

		d.processDiscoveryMessage(&msg, addr)
	}
}

// processDiscoveryMessage processes a discovery message
func (d *UDPDiscovery) processDiscoveryMessage(msg *DiscoveryMessage, from *net.UDPAddr) {
	// Call message handler
	if d.messageHandler != nil {
		d.messageHandler.OnMessage(msg, from)
	}

	switch msg.Type {
	case MessageTypeAnnounce, MessageTypeHeartbeat:
		d.handleDeviceAnnounce(msg, from)

	case MessageTypeQuery:
		d.handleQuery(msg, from)

	case MessageTypeResponse:
		d.handleResponse(msg, from)

	case MessageTypeGoodbye:
		d.handleGoodbye(msg)
	}
}

// handleDeviceAnnounce handles device announcement messages
func (d *UDPDiscovery) handleDeviceAnnounce(msg *DiscoveryMessage, from *net.UDPAddr) {
	deviceID := msg.Device.ID
	
	d.peersMutex.Lock()
	defer d.peersMutex.Unlock()

	isNewDevice := false
	if _, exists := d.peers[deviceID]; !exists {
		isNewDevice = true
	}

	// Update or add peer
	d.peers[deviceID] = &PeerDevice{
		DeviceInfo: msg.Device,
		RemoteAddr: from,
		TTL:        time.Now().Add(time.Duration(msg.TTL) * time.Second),
	}

	if isNewDevice && d.messageHandler != nil {
		d.messageHandler.OnDeviceDiscovered(d.peers[deviceID])
	}
}

// handleQuery handles discovery query messages
func (d *UDPDiscovery) handleQuery(msg *DiscoveryMessage, from *net.UDPAddr) {
	// Send response with our device information
	response := DiscoveryMessage{
		Type:      MessageTypeResponse,
		Device:    d.deviceInfo,
		Timestamp: time.Now(),
		TTL:       int(d.discoveryConfig.Interval.Seconds()),
	}

	d.sendMessage(&response, from)
}

// handleResponse handles discovery response messages
func (d *UDPDiscovery) handleResponse(msg *DiscoveryMessage, from *net.UDPAddr) {
	// Same as announce
	d.handleDeviceAnnounce(msg, from)
}

// handleGoodbye handles goodbye messages
func (d *UDPDiscovery) handleGoodbye(msg *DiscoveryMessage) {
	deviceID := msg.Device.ID
	
	d.peersMutex.Lock()
	if _, exists := d.peers[deviceID]; exists {
		delete(d.peers, deviceID)
		d.peersMutex.Unlock()
		
		if d.messageHandler != nil {
			d.messageHandler.OnDeviceLost(deviceID)
		}
	} else {
		d.peersMutex.Unlock()
	}
}

// announceRoutine periodically announces this device
func (d *UDPDiscovery) announceRoutine() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.discoveryConfig.AnnounceInterval)
	defer ticker.Stop()

	// Send initial announcement
	d.announce()

	for {
		select {
		case <-d.shutdown:
			return
		case <-ticker.C:
			d.announce()
		}
	}
}

// queryRoutine periodically queries for other devices
func (d *UDPDiscovery) queryRoutine() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.discoveryConfig.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-d.shutdown:
			return
		case <-ticker.C:
			d.sendQuery()
		}
	}
}

// peerCleanupRoutine periodically removes expired peers
func (d *UDPDiscovery) peerCleanupRoutine() {
	defer d.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.shutdown:
			return
		case <-ticker.C:
			d.cleanupExpiredPeers()
		}
	}
}

// announce sends an announcement message
func (d *UDPDiscovery) announce() {
	d.deviceInfo.LastSeen = time.Now()
	
	msg := DiscoveryMessage{
		Type:      MessageTypeAnnounce,
		Device:    d.deviceInfo,
		Timestamp: time.Now(),
		TTL:       int(d.discoveryConfig.Interval.Seconds()) * 2,
	}

	d.sendMulticast(&msg)
}

// sendQuery sends a query message
func (d *UDPDiscovery) sendQuery() {
	msg := DiscoveryMessage{
		Type:      MessageTypeQuery,
		Device:    d.deviceInfo,
		Timestamp: time.Now(),
		TTL:       30,
	}

	d.sendMulticast(&msg)
}

// sendGoodbye sends a goodbye message
func (d *UDPDiscovery) sendGoodbye() {
	msg := DiscoveryMessage{
		Type:      MessageTypeGoodbye,
		Device:    d.deviceInfo,
		Timestamp: time.Now(),
		TTL:       0,
	}

	d.sendMulticast(&msg)
}

// sendMulticast sends a message via multicast
func (d *UDPDiscovery) sendMulticast(msg *DiscoveryMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling discovery message: %v", err)
		return
	}

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", d.config.MulticastAddress, d.config.DiscoveryPort))
	if err != nil {
		log.Printf("Error resolving multicast address: %v", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("Error creating multicast connection: %v", err)
		return
	}
	defer conn.Close()

	if _, err := conn.Write(data); err != nil {
		log.Printf("Error sending multicast message: %v", err)
	}
}

// sendMessage sends a direct message to a specific address
func (d *UDPDiscovery) sendMessage(msg *DiscoveryMessage, to *net.UDPAddr) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	if _, err := d.conn.WriteToUDP(data, to); err != nil {
		log.Printf("Error sending direct message to %s: %v", to.String(), err)
	}
}

// cleanupExpiredPeers removes peers that have exceeded their TTL
func (d *UDPDiscovery) cleanupExpiredPeers() {
	now := time.Now()
	
	d.peersMutex.Lock()
	var expiredPeers []string
	for id, peer := range d.peers {
		if now.After(peer.TTL) {
			expiredPeers = append(expiredPeers, id)
		}
	}
	
	for _, id := range expiredPeers {
		delete(d.peers, id)
	}
	d.peersMutex.Unlock()

	// Notify about lost devices
	for _, id := range expiredPeers {
		if d.messageHandler != nil {
			d.messageHandler.OnDeviceLost(id)
		}
	}
}

// getLocalAddr gets the local IP address
func (d *UDPDiscovery) getLocalAddr() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// GetPeers returns a copy of current peers
func (d *UDPDiscovery) GetPeers() map[string]*PeerDevice {
	d.peersMutex.RLock()
	defer d.peersMutex.RUnlock()

	peers := make(map[string]*PeerDevice)
	for id, peer := range d.peers {
		peerCopy := *peer
		peers[id] = &peerCopy
	}
	return peers
}

// GetStats returns discovery service statistics
func (d *UDPDiscovery) GetStats() map[string]interface{} {
	d.peersMutex.RLock()
	peerCount := len(d.peers)
	d.peersMutex.RUnlock()

	return map[string]interface{}{
		"enabled":     d.discoveryConfig.Enabled,
		"running":     d.running.Load(),
		"peer_count":  peerCount,
		"device_id":   d.deviceInfo.ID,
		"device_addr": d.deviceInfo.Address,
		"port":        d.config.Port,
	}
}