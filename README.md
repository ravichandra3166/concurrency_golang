# Network Device - Concurrent Go Network Program

A configurable, concurrent network device program written in Go that provides TCP server functionality, UDP device discovery, and HTTP monitoring capabilities.

## Features

- 🚀 **Concurrent TCP Server**: Handles multiple client connections simultaneously using goroutines
- 🔍 **UDP Device Discovery**: Automatic device discovery using multicast UDP
- 📊 **HTTP Monitoring**: RESTful endpoints for monitoring device status and metrics
- ⚙️ **Configurable**: YAML-based configuration with command-line overrides
- 🔒 **Security**: Built-in support for TLS and authentication (configurable)
- 📝 **Logging**: Structured logging with configurable levels
- 🏗️ **Modular Architecture**: Clean separation of concerns with internal packages

## Architecture

```
network-device/
├── cmd/device/           # Main application entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── server/          # TCP server implementation
│   ├── discovery/       # UDP discovery service
│   └── device/          # Device orchestration and HTTP monitoring
├── examples/client/     # Example client applications
├── config/             # Configuration files
└── logs/              # Log files (created at runtime)
```

## Quick Start

### 1. Build the Application

```bash
go mod tidy
go build -o network-device cmd/device/main.go
```

### 2. Run with Default Configuration

```bash
./network-device run
```

### 3. Run with Custom Configuration

```bash
./network-device run --config config/device.yaml
```

### 4. Run with Command Line Overrides

```bash
./network-device run --device-id my-device --tcp-port 9090 --verbose
```

## Configuration

The device uses YAML configuration files. Here's the default configuration structure:

```yaml
device:
  id: "device-001"
  name: "Network Device 1"
  type: "gateway"
  location: "datacenter-1"

network:
  tcp:
    enabled: true
    port: 8080
    max_connections: 1000
    timeout: 30s
    buffer_size: 4096
  udp:
    enabled: true
    port: 8081
    discovery_port: 9999
    multicast_address: "239.255.255.250"

services:
  discovery:
    enabled: true
    interval: 30s
    announce_interval: 10s
  monitoring:
    enabled: true
    metrics_port: 8082
    health_check_interval: 5s

logging:
  level: "info"
  file: "logs/device.log"
  max_size: 100
  max_backups: 3

security:
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
  auth:
    enabled: false
    token_secret: ""
```

## CLI Commands

### Run the Device
```bash
# Basic run
./network-device run

# With custom config
./network-device run --config /path/to/config.yaml

# With overrides
./network-device run --device-id dev001 --tcp-port 8080 --udp-port 8081
```

### Show Configuration
```bash
./network-device config
```

### Check Status
```bash
./network-device status
```

### Help
```bash
./network-device --help
./network-device run --help
```

## TCP Server

The TCP server supports concurrent connections and provides:

- **Message Echo**: Messages are echoed back to the sender with timestamps
- **Broadcasting**: Messages are broadcast to all connected clients
- **Connection Management**: Automatic cleanup of stale connections
- **Rate Limiting**: Configurable maximum concurrent connections
- **Timeouts**: Configurable connection timeouts

### Example TCP Client Usage

```bash
# Run the example TCP client
go run examples/client/tcp_client.go localhost:8080
```

## UDP Discovery

The UDP discovery service enables automatic device detection on the network:

- **Multicast Announcements**: Periodic device announcements
- **Query/Response**: Discovery queries and responses
- **Peer Management**: Automatic peer detection and cleanup
- **TTL Management**: Time-to-live for discovered devices

### Example Discovery Client Usage

```bash
# Run the example discovery client
go run examples/client/udp_discovery_client.go
```

## HTTP Monitoring

The HTTP monitoring service provides RESTful endpoints:

| Endpoint | Description |
|----------|-------------|
| `/health` | Health check endpoint |
| `/status` | Device status and statistics |
| `/peers` | List of discovered peer devices |
| `/connections` | TCP connection information |
| `/config` | Device configuration |

### Example API Usage

```bash
# Check health
curl http://localhost:8082/health

# Get device status
curl http://localhost:8082/status

# List peer devices
curl http://localhost:8082/peers

# View active connections
curl http://localhost:8082/connections

# Get configuration
curl http://localhost:8082/config
```

## Examples

### Running Multiple Devices

Terminal 1:
```bash
./network-device run --device-id device-001 --device-name "Gateway Device" --tcp-port 8080
```

Terminal 2:
```bash
./network-device run --device-id device-002 --device-name "Edge Device" --tcp-port 8081 --config config/device.yaml
```

### Testing TCP Connectivity

```bash
# Connect with telnet
telnet localhost 8080

# Or use the example client
go run examples/client/tcp_client.go localhost:8080
```

### Monitoring Devices

```bash
# Monitor device health
watch -n 2 curl -s http://localhost:8082/health

# Check peer discovery
curl -s http://localhost:8082/peers | jq '.'
```

## Development

### Project Structure

- `cmd/device/main.go`: Application entry point with CLI
- `internal/config/`: Configuration loading and validation
- `internal/server/`: TCP server with concurrent connection handling
- `internal/discovery/`: UDP multicast discovery service
- `internal/device/`: Device orchestration and HTTP API
- `examples/client/`: Example client applications

### Key Concurrency Features

1. **Goroutine-based TCP Server**: Each client connection runs in its own goroutine
2. **Channel Communication**: Uses channels for message passing between goroutines
3. **Context Cancellation**: Proper context handling for graceful shutdowns
4. **Atomic Operations**: Thread-safe counters and flags
5. **Mutex Protection**: Safe access to shared data structures

### Building and Testing

```bash
# Install dependencies
go mod tidy

# Build
go build -o network-device cmd/device/main.go

# Run tests (if you add them)
go test ./...

# Format code
go fmt ./...

# Vet code
go vet ./...
```

## Security Considerations

- **Network Exposure**: The device opens network ports; use firewalls appropriately
- **Multicast Traffic**: UDP discovery uses multicast; consider network policies
- **Authentication**: Enable authentication in production environments
- **TLS**: Enable TLS for encrypted communications
- **Resource Limits**: Configure appropriate connection and timeout limits

## Troubleshooting

### Common Issues

1. **Port Already in Use**
   ```bash
   # Check what's using the port
   lsof -i :8080
   
   # Use different port
   ./network-device run --tcp-port 8090
   ```

2. **Multicast Not Working**
   ```bash
   # Check multicast support
   ip maddr show
   
   # Verify network interface
   ip route show
   ```

3. **Permission Denied**
   ```bash
   # For ports < 1024, run with sudo
   sudo ./network-device run --tcp-port 80
   ```

### Logs

Check the log file for detailed information:
```bash
tail -f logs/device.log
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is provided as-is for educational and development purposes.