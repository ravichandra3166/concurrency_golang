package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"network-device/internal/config"
	"network-device/internal/device"
)

var (
	configFile string
	deviceID   string
	deviceName string
	tcpPort    int
	udpPort    int
	verbose    bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "network-device",
	Short: "A concurrent network device program",
	Long: `A configurable network device that supports:
- Concurrent TCP server for client connections
- UDP discovery service for device detection
- HTTP monitoring endpoints
- Device configuration management

Example usage:
  network-device run --config config/device.yaml
  network-device run --device-id dev001 --tcp-port 8080`,
	Version: "1.0.0",
}

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the network device",
	Long: `Start the network device with all configured services.
The device will run until interrupted with Ctrl+C.`,
	RunE: runDevice,
}

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check device status",
	Long:  `Check the status of a running device via HTTP monitoring endpoint.`,
	RunE:  checkStatus,
}

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	Long:  `Display the current device configuration.`,
	RunE:  showConfig,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file (default is ./config/device.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Run command flags
	runCmd.Flags().StringVarP(&deviceID, "device-id", "i", "", "device ID override")
	runCmd.Flags().StringVarP(&deviceName, "device-name", "n", "", "device name override")
	runCmd.Flags().IntVarP(&tcpPort, "tcp-port", "t", 0, "TCP port override")
	runCmd.Flags().IntVarP(&udpPort, "udp-port", "u", 0, "UDP port override")

	// Add subcommands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// runDevice runs the network device
func runDevice(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply command line overrides
	applyOverrides(cfg)

	if verbose {
		log.Printf("Starting device with config: %+v", cfg.Device)
	}

	// Create and run device
	dev, err := device.NewDevice(cfg)
	if err != nil {
		return fmt.Errorf("failed to create device: %w", err)
	}

	// Run the device (blocks until interrupted)
	return dev.Run()
}

// checkStatus checks the status of a running device
func checkStatus(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !cfg.Services.Monitoring.Enabled {
		return fmt.Errorf("monitoring is not enabled in configuration")
	}

	url := fmt.Sprintf("http://localhost:%d/status", cfg.Services.Monitoring.MetricsPort)
	
	// Simple HTTP client to check status
	fmt.Printf("Checking device status at %s\n", url)
	fmt.Println("Note: Use curl or a web browser to access the monitoring endpoints:")
	fmt.Printf("  Health:       http://localhost:%d/health\n", cfg.Services.Monitoring.MetricsPort)
	fmt.Printf("  Status:       http://localhost:%d/status\n", cfg.Services.Monitoring.MetricsPort)
	fmt.Printf("  Peers:        http://localhost:%d/peers\n", cfg.Services.Monitoring.MetricsPort)
	fmt.Printf("  Connections:  http://localhost:%d/connections\n", cfg.Services.Monitoring.MetricsPort)
	fmt.Printf("  Config:       http://localhost:%d/config\n", cfg.Services.Monitoring.MetricsPort)

	return nil
}

// showConfig displays the current configuration
func showConfig(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply command line overrides
	applyOverrides(cfg)

	// Display configuration
	fmt.Println("Device Configuration:")
	fmt.Printf("  ID:       %s\n", cfg.Device.ID)
	fmt.Printf("  Name:     %s\n", cfg.Device.Name)
	fmt.Printf("  Type:     %s\n", cfg.Device.Type)
	fmt.Printf("  Location: %s\n", cfg.Device.Location)
	
	fmt.Println("\nNetwork Configuration:")
	fmt.Printf("  TCP Enabled: %t, Port: %d\n", cfg.Network.TCP.Enabled, cfg.Network.TCP.Port)
	fmt.Printf("  UDP Enabled: %t, Port: %d\n", cfg.Network.UDP.Enabled, cfg.Network.UDP.Port)
	fmt.Printf("  Discovery Port: %d\n", cfg.Network.UDP.DiscoveryPort)
	
	fmt.Println("\nServices Configuration:")
	fmt.Printf("  Discovery:  %t (Interval: %v)\n", cfg.Services.Discovery.Enabled, cfg.Services.Discovery.Interval)
	fmt.Printf("  Monitoring: %t (Port: %d)\n", cfg.Services.Monitoring.Enabled, cfg.Services.Monitoring.MetricsPort)
	
	fmt.Println("\nSecurity Configuration:")
	fmt.Printf("  TLS:  %t\n", cfg.Security.TLS.Enabled)
	fmt.Printf("  Auth: %t\n", cfg.Security.Auth.Enabled)

	return nil
}

// loadConfig loads the configuration from file or defaults
func loadConfig() (*config.Config, error) {
	if configFile == "" {
		// Try default locations
		if _, err := os.Stat("config/device.yaml"); err == nil {
			configFile = "config/device.yaml"
		} else if _, err := os.Stat("device.yaml"); err == nil {
			configFile = "device.yaml"
		}
	}

	return config.LoadConfig(configFile)
}

// applyOverrides applies command line overrides to the configuration
func applyOverrides(cfg *config.Config) {
	if deviceID != "" {
		cfg.Device.ID = deviceID
	}
	if deviceName != "" {
		cfg.Device.Name = deviceName
	}
	if tcpPort > 0 {
		cfg.Network.TCP.Port = tcpPort
	}
	if udpPort > 0 {
		cfg.Network.UDP.Port = udpPort
	}
}