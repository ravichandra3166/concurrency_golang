package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config represents the complete device configuration
type Config struct {
	Device   DeviceConfig   `mapstructure:"device" yaml:"device"`
	Network  NetworkConfig  `mapstructure:"network" yaml:"network"`
	Services ServicesConfig `mapstructure:"services" yaml:"services"`
	Logging  LoggingConfig  `mapstructure:"logging" yaml:"logging"`
	Security SecurityConfig `mapstructure:"security" yaml:"security"`
}

// DeviceConfig contains device identification and metadata
type DeviceConfig struct {
	ID       string `mapstructure:"id" yaml:"id"`
	Name     string `mapstructure:"name" yaml:"name"`
	Type     string `mapstructure:"type" yaml:"type"`
	Location string `mapstructure:"location" yaml:"location"`
}

// NetworkConfig contains network service configurations
type NetworkConfig struct {
	TCP TCPConfig `mapstructure:"tcp" yaml:"tcp"`
	UDP UDPConfig `mapstructure:"udp" yaml:"udp"`
}

// TCPConfig contains TCP server settings
type TCPConfig struct {
	Enabled        bool          `mapstructure:"enabled" yaml:"enabled"`
	Port           int           `mapstructure:"port" yaml:"port"`
	MaxConnections int           `mapstructure:"max_connections" yaml:"max_connections"`
	Timeout        time.Duration `mapstructure:"timeout" yaml:"timeout"`
	BufferSize     int           `mapstructure:"buffer_size" yaml:"buffer_size"`
}

// UDPConfig contains UDP service settings
type UDPConfig struct {
	Enabled          bool   `mapstructure:"enabled" yaml:"enabled"`
	Port             int    `mapstructure:"port" yaml:"port"`
	DiscoveryPort    int    `mapstructure:"discovery_port" yaml:"discovery_port"`
	MulticastAddress string `mapstructure:"multicast_address" yaml:"multicast_address"`
}

// ServicesConfig contains additional service configurations
type ServicesConfig struct {
	Discovery  DiscoveryConfig  `mapstructure:"discovery" yaml:"discovery"`
	Monitoring MonitoringConfig `mapstructure:"monitoring" yaml:"monitoring"`
}

// DiscoveryConfig contains device discovery settings
type DiscoveryConfig struct {
	Enabled          bool          `mapstructure:"enabled" yaml:"enabled"`
	Interval         time.Duration `mapstructure:"interval" yaml:"interval"`
	AnnounceInterval time.Duration `mapstructure:"announce_interval" yaml:"announce_interval"`
}

// MonitoringConfig contains monitoring and health check settings
type MonitoringConfig struct {
	Enabled             bool          `mapstructure:"enabled" yaml:"enabled"`
	MetricsPort         int           `mapstructure:"metrics_port" yaml:"metrics_port"`
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval" yaml:"health_check_interval"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level      string `mapstructure:"level" yaml:"level"`
	File       string `mapstructure:"file" yaml:"file"`
	MaxSize    int    `mapstructure:"max_size" yaml:"max_size"`
	MaxBackups int    `mapstructure:"max_backups" yaml:"max_backups"`
}

// SecurityConfig contains security settings
type SecurityConfig struct {
	TLS  TLSConfig  `mapstructure:"tls" yaml:"tls"`
	Auth AuthConfig `mapstructure:"auth" yaml:"auth"`
}

// TLSConfig contains TLS/SSL settings
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	CertFile string `mapstructure:"cert_file" yaml:"cert_file"`
	KeyFile  string `mapstructure:"key_file" yaml:"key_file"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	Enabled     bool   `mapstructure:"enabled" yaml:"enabled"`
	TokenSecret string `mapstructure:"token_secret" yaml:"token_secret"`
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()
	
	// Set default values
	setDefaults(v)
	
	// Set config file path
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("device")
		v.SetConfigType("yaml")
		v.AddConfigPath("./config")
		v.AddConfigPath(".")
	}
	
	// Enable environment variables
	v.AutomaticEnv()
	v.SetEnvPrefix("DEVICE")
	
	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Printf("Warning: Config file not found, using defaults\n")
		} else {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}
	
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}
	
	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Device defaults
	v.SetDefault("device.id", "device-001")
	v.SetDefault("device.name", "Network Device")
	v.SetDefault("device.type", "gateway")
	v.SetDefault("device.location", "unknown")
	
	// Network defaults
	v.SetDefault("network.tcp.enabled", true)
	v.SetDefault("network.tcp.port", 8080)
	v.SetDefault("network.tcp.max_connections", 1000)
	v.SetDefault("network.tcp.timeout", "30s")
	v.SetDefault("network.tcp.buffer_size", 4096)
	
	v.SetDefault("network.udp.enabled", true)
	v.SetDefault("network.udp.port", 8081)
	v.SetDefault("network.udp.discovery_port", 9999)
	v.SetDefault("network.udp.multicast_address", "239.255.255.250")
	
	// Services defaults
	v.SetDefault("services.discovery.enabled", true)
	v.SetDefault("services.discovery.interval", "30s")
	v.SetDefault("services.discovery.announce_interval", "10s")
	
	v.SetDefault("services.monitoring.enabled", true)
	v.SetDefault("services.monitoring.metrics_port", 8082)
	v.SetDefault("services.monitoring.health_check_interval", "5s")
	
	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.file", "logs/device.log")
	v.SetDefault("logging.max_size", 100)
	v.SetDefault("logging.max_backups", 3)
	
	// Security defaults
	v.SetDefault("security.tls.enabled", false)
	v.SetDefault("security.auth.enabled", false)
}

// validateConfig validates the loaded configuration
func validateConfig(config *Config) error {
	if config.Device.ID == "" {
		return fmt.Errorf("device ID cannot be empty")
	}
	
	if config.Network.TCP.Enabled && (config.Network.TCP.Port <= 0 || config.Network.TCP.Port > 65535) {
		return fmt.Errorf("invalid TCP port: %d", config.Network.TCP.Port)
	}
	
	if config.Network.UDP.Enabled && (config.Network.UDP.Port <= 0 || config.Network.UDP.Port > 65535) {
		return fmt.Errorf("invalid UDP port: %d", config.Network.UDP.Port)
	}
	
	if config.Services.Monitoring.Enabled && (config.Services.Monitoring.MetricsPort <= 0 || config.Services.Monitoring.MetricsPort > 65535) {
		return fmt.Errorf("invalid metrics port: %d", config.Services.Monitoring.MetricsPort)
	}
	
	// Create logs directory if it doesn't exist
	if config.Logging.File != "" {
		logDir := filepath.Dir(config.Logging.File)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}
	}
	
	return nil
}