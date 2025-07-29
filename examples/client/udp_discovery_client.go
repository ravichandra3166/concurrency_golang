package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

// DiscoveryMessage represents a discovery protocol message
type DiscoveryMessage struct {
	Type      string    `json:"type"`
	Device    DeviceInfo `json:"device"`
	Timestamp time.Time `json:"timestamp"`
	TTL       int       `json:"ttl"`
}

// DeviceInfo contains information about a device
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

func main() {
	multicastAddr := "239.255.255.250:9999"
	
	fmt.Printf("UDP Discovery Client\n")
	fmt.Printf("Listening for device announcements on %s\n", multicastAddr)
	fmt.Println("Press Ctrl+C to stop")
	
	// Listen for multicast messages
	addr, err := net.ResolveUDPAddr("udp", multicastAddr)
	if err != nil {
		log.Fatalf("Error resolving address: %v", err)
	}
	
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		log.Fatalf("Error listening on multicast: %v", err)
	}
	defer conn.Close()
	
	// Send discovery query
	go sendDiscoveryQuery()
	
	// Keep track of discovered devices
	devices := make(map[string]*DeviceInfo)
	
	buffer := make([]byte, 1024)
	for {
		n, senderAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Error reading UDP message: %v", err)
			continue
		}
		
		var msg DiscoveryMessage
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}
		
		// Process discovery message
		switch msg.Type {
		case "announce", "response":
			deviceID := msg.Device.ID
			if _, exists := devices[deviceID]; !exists {
				fmt.Printf("\n🎯 NEW DEVICE DISCOVERED:\n")
				fmt.Printf("   ID:       %s\n", msg.Device.ID)
				fmt.Printf("   Name:     %s\n", msg.Device.Name)
				fmt.Printf("   Type:     %s\n", msg.Device.Type)
				fmt.Printf("   Location: %s\n", msg.Device.Location)
				fmt.Printf("   Address:  %s:%d\n", msg.Device.Address, msg.Device.Port)
				fmt.Printf("   Services: %v\n", msg.Device.Services)
				fmt.Printf("   From:     %s\n", senderAddr.String())
				fmt.Printf("   Time:     %s\n", msg.Timestamp.Format("15:04:05"))
				fmt.Println()
			} else {
				fmt.Printf("📡 Heartbeat from %s (%s) at %s\n", 
					msg.Device.Name, msg.Device.ID, msg.Timestamp.Format("15:04:05"))
			}
			devices[deviceID] = &msg.Device
			
		case "goodbye":
			deviceID := msg.Device.ID
			if _, exists := devices[deviceID]; exists {
				fmt.Printf("👋 Device %s (%s) has left the network\n", 
					msg.Device.Name, msg.Device.ID)
				delete(devices, deviceID)
			}
		}
	}
}

// sendDiscoveryQuery sends a discovery query to find devices
func sendDiscoveryQuery() {
	time.Sleep(2 * time.Second) // Wait a bit before sending query
	
	multicastAddr := "239.255.255.250:9999"
	addr, err := net.ResolveUDPAddr("udp", multicastAddr)
	if err != nil {
		log.Printf("Error resolving query address: %v", err)
		return
	}
	
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("Error creating query connection: %v", err)
		return
	}
	defer conn.Close()
	
	// Create discovery query message
	queryMsg := DiscoveryMessage{
		Type: "query",
		Device: DeviceInfo{
			ID:       "discovery-client",
			Name:     "Discovery Client",
			Type:     "client",
			Location: "local",
			Services: []string{"discovery"},
		},
		Timestamp: time.Now(),
		TTL:       30,
	}
	
	data, err := json.Marshal(queryMsg)
	if err != nil {
		log.Printf("Error marshaling query: %v", err)
		return
	}
	
	fmt.Println("🔍 Sending discovery query...")
	if _, err := conn.Write(data); err != nil {
		log.Printf("Error sending query: %v", err)
	}
}