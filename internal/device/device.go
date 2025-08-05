package device

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"network-device/internal/config"
	"network-device/internal/discovery"
	"network-device/internal/server"
)

// Device represents a network device with all its services
type Device struct {
	config      *config.Config
	tcpServer   *server.TCPServer
	udpDiscovery *discovery.UDPDiscovery
	httpServer  *http.Server
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	running     bool
	mutex       sync.RWMutex
}

// DeviceStats contains device statistics
type DeviceStats struct {
	DeviceID    string                 `json:"device_id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Location    string                 `json:"location"`
	Uptime      time.Duration          `json:"uptime"`
	StartTime   time.Time              `json:"start_time"`
	TCP         map[string]interface{} `json:"tcp"`
	UDP         map[string]interface{} `json:"udp"`
	Discovery   map[string]interface{} `json:"discovery"`
	PeerDevices int                    `json:"peer_devices"`
}

// NewDevice creates a new device instance
func NewDevice(cfg *config.Config) (*Device, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create TCP server
	tcpServer := server.NewTCPServer(&cfg.Network.TCP, cfg.Device.ID)

	// Create UDP discovery service
	udpDiscovery := discovery.NewUDPDiscovery(&cfg.Network.UDP, &cfg.Services.Discovery, &cfg.Device)

	// Create HTTP monitoring server
	var httpServer *http.Server
	if cfg.Services.Monitoring.Enabled {
		mux := http.NewServeMux()
		device := &Device{
			config:       cfg,
			tcpServer:    tcpServer,
			udpDiscovery: udpDiscovery,
			ctx:          ctx,
			cancel:       cancel,
		}
		setupMonitoringRoutes(mux, device)
		
		httpServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Services.Monitoring.MetricsPort),
			Handler: mux,
		}
	}

	return &Device{
		config:       cfg,
		tcpServer:    tcpServer,
		udpDiscovery: udpDiscovery,
		httpServer:   httpServer,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start starts all device services
func (d *Device) Start() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.running {
		return fmt.Errorf("device is already running")
	}

	log.Printf("Starting device: %s (%s)", d.config.Device.Name, d.config.Device.ID)

	// Start TCP server
	if d.config.Network.TCP.Enabled {
		if err := d.tcpServer.Start(d.ctx); err != nil {
			return fmt.Errorf("failed to start TCP server: %w", err)
		}
		log.Printf("TCP server started on port %d", d.config.Network.TCP.Port)
	}

	// Start UDP discovery
	if d.config.Services.Discovery.Enabled {
		if err := d.udpDiscovery.Start(d.ctx); err != nil {
			return fmt.Errorf("failed to start UDP discovery: %w", err)
		}
		log.Printf("UDP discovery started on port %d", d.config.Network.UDP.DiscoveryPort)
	}

	// Start HTTP monitoring server
	if d.config.Services.Monitoring.Enabled && d.httpServer != nil {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			log.Printf("HTTP monitoring server starting on port %d", d.config.Services.Monitoring.MetricsPort)
			if err := d.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTP server error: %v", err)
			}
		}()
	}

	// Start health check routine
	if d.config.Services.Monitoring.Enabled {
		d.wg.Add(1)
		go d.healthCheckRoutine()
	}

	d.running = true
	log.Printf("Device %s started successfully", d.config.Device.ID)
	return nil
}

// Stop stops all device services gracefully
func (d *Device) Stop() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if !d.running {
		return nil
	}

	log.Printf("Stopping device: %s", d.config.Device.ID)

	// Cancel context to signal all goroutines to stop
	d.cancel()

	// Stop HTTP server
	if d.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := d.httpServer.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down HTTP server: %v", err)
		}
	}

	// Stop UDP discovery
	if err := d.udpDiscovery.Stop(); err != nil {
		log.Printf("Error stopping UDP discovery: %v", err)
	}

	// Stop TCP server
	if err := d.tcpServer.Stop(); err != nil {
		log.Printf("Error stopping TCP server: %v", err)
	}

	// Wait for all goroutines to finish
	d.wg.Wait()

	d.running = false
	log.Printf("Device %s stopped", d.config.Device.ID)
	return nil
}

// Run runs the device until interrupted
func (d *Device) Run() error {
	// Start the device
	if err := d.Start(); err != nil {
		return err
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Device is running. Press Ctrl+C to stop.")

	// Block until we receive a signal
	sig := <-sigChan
	log.Printf("Received signal: %v", sig)

	// Stop the device
	return d.Stop()
}

// healthCheckRoutine performs periodic health checks
func (d *Device) healthCheckRoutine() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.Services.Monitoring.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.performHealthCheck()
		}
	}
}

// performHealthCheck performs a health check on all services
func (d *Device) performHealthCheck() {
	stats := d.GetStats()
	log.Printf("Health check - TCP connections: %v, Discovered peers: %d", 
		stats.TCP["active_connections"], stats.PeerDevices)
}

// GetStats returns current device statistics
func (d *Device) GetStats() *DeviceStats {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	// Get TCP stats
	tcpStats := d.tcpServer.GetStats()
	
	// Get UDP discovery stats
	discoveryStats := d.udpDiscovery.GetStats()
	
	// Get peer count
	peers := d.udpDiscovery.GetPeers()
	peerCount := len(peers)

	return &DeviceStats{
		DeviceID:    d.config.Device.ID,
		Name:        d.config.Device.Name,
		Type:        d.config.Device.Type,
		Location:    d.config.Device.Location,
		Uptime:      time.Since(time.Now()), // This will be corrected with actual start time
		StartTime:   time.Now(),             // This should be set when device starts
		TCP:         tcpStats,
		UDP:         map[string]interface{}{"port": d.config.Network.UDP.Port},
		Discovery:   discoveryStats,
		PeerDevices: peerCount,
	}
}

// setupMonitoringRoutes sets up HTTP routes for monitoring
func setupMonitoringRoutes(mux *http.ServeMux, device *Device) {
	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
	})

	// Device status endpoint
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		stats := device.GetStats()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		// Simple JSON response
		fmt.Fprintf(w, `{
			"device_id": "%s",
			"name": "%s",
			"type": "%s",
			"location": "%s",
			"tcp_active_connections": %v,
			"peer_devices": %d,
			"tcp_running": %v,
			"discovery_running": %v
		}`, stats.DeviceID, stats.Name, stats.Type, stats.Location,
			stats.TCP["active_connections"], stats.PeerDevices,
			stats.TCP["running"], stats.Discovery["running"])
	})

	// Peers endpoint
	mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		peers := device.udpDiscovery.GetPeers()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		fmt.Fprintf(w, `{"peer_count": %d, "peers": [`, len(peers))
		first := true
		for _, peer := range peers {
			if !first {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{
				"id": "%s",
				"name": "%s",
				"type": "%s",
				"address": "%s",
				"port": %d
			}`, peer.ID, peer.Name, peer.Type, peer.Address, peer.Port)
			first = false
		}
		fmt.Fprint(w, "]}")
	})

	// TCP connections endpoint
	mux.HandleFunc("/connections", func(w http.ResponseWriter, r *http.Request) {
		tcpStats := device.tcpServer.GetStats()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		fmt.Fprintf(w, `{
			"active_connections": %v,
			"max_connections": %v,
			"port": %v,
			"enabled": %v,
			"running": %v
		}`, tcpStats["active_connections"], tcpStats["max_connections"],
			tcpStats["port"], tcpStats["enabled"], tcpStats["running"])
	})

	// Device configuration endpoint
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		fmt.Fprintf(w, `{
			"device": {
				"id": "%s",
				"name": "%s",
				"type": "%s",
				"location": "%s"
			},
			"network": {
				"tcp_port": %d,
				"udp_port": %d,
				"discovery_port": %d
			},
			"services": {
				"tcp_enabled": %t,
				"udp_enabled": %t,
				"discovery_enabled": %t,
				"monitoring_enabled": %t
			}
		}`, device.config.Device.ID, device.config.Device.Name,
			device.config.Device.Type, device.config.Device.Location,
			device.config.Network.TCP.Port, device.config.Network.UDP.Port,
			device.config.Network.UDP.DiscoveryPort,
			device.config.Network.TCP.Enabled, device.config.Network.UDP.Enabled,
			device.config.Services.Discovery.Enabled, device.config.Services.Monitoring.Enabled)
	})
}

// IsRunning returns whether the device is currently running
func (d *Device) IsRunning() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.running
}

// GetConfig returns the device configuration
func (d *Device) GetConfig() *config.Config {
	return d.config
}

// GetTCPServer returns the TCP server instance
func (d *Device) GetTCPServer() *server.TCPServer {
	return d.tcpServer
}

// GetUDPDiscovery returns the UDP discovery service instance
func (d *Device) GetUDPDiscovery() *discovery.UDPDiscovery {
	return d.udpDiscovery
}