# tmiDB Core

This project contains the core functionalities of the tmiDB platform.

## Services Included

- **Database (PostgreSQL + TimescaleDB)**: The main data store.
- **NATS**: The message bus for asynchronous communication.
- **SeaweedFS**: The object storage for large files (S3 compatible).
- **migrator**: A Go service that runs once to set up or migrate the database schema.
- **api**: The main Go service providing the REST API and web console.
- **worker**: A Go service for background jobs, processing messages from NATS.

## Functionality

- Provides the main database schema and handles migrations.
- Exposes a REST API for data interaction.
- Serves the web console for administration and data exploration.
- Processes background tasks and data writing asynchronously.

## CLI Usage

tmiDB provides a command-line interface for managing and monitoring all components.

### Installation

```bash
# Build the CLI
go build -o ./bin/tmidb-cli ./cmd/cli
```

### Basic Commands

```bash
# Show status of all components
tmidb-cli status

# Process management
tmidb-cli process list                    # List all processes
tmidb-cli process status api              # Check status of specific component
tmidb-cli process start data-consumer     # Start a component
tmidb-cli process stop data-consumer      # Stop a component
tmidb-cli process restart api             # Restart a component

# Log management
tmidb-cli logs                            # Show recent logs from all components
tmidb-cli logs api                        # Show logs from specific component
tmidb-cli logs api -f                     # Follow logs in real-time
tmidb-cli logs enable data-manager        # Enable logging for component
tmidb-cli logs disable data-consumer      # Disable logging for component
tmidb-cli logs status                     # Show log status for all components

# System monitoring
tmidb-cli monitor health                  # Overall system health check
tmidb-cli monitor services                # Service health status
tmidb-cli monitor system                  # Real-time system resource monitoring
```

### Advanced Commands

```bash
# Process group management
tmidb-cli process group list              # List process groups
tmidb-cli process group start all         # Start all processes
tmidb-cli process group stop core         # Stop core services
tmidb-cli process batch start api data-manager  # Batch control

# Log filtering and search
tmidb-cli logs filter --level=error       # Filter by log level
tmidb-cli logs filter --since=1h --pattern="error"  # Time and pattern filter
tmidb-cli logs search "connection failed"  # Search with regex

# Configuration management
tmidb-cli config get api.port             # Get config value
tmidb-cli config set log.level debug      # Set config value
tmidb-cli config list                     # List all configs
tmidb-cli config export config.yaml       # Export configuration
tmidb-cli config import config.yaml       # Import configuration

# Backup and restore
tmidb-cli backup create                   # Create backup
tmidb-cli backup restore backup-20240101  # Restore from backup
tmidb-cli backup list                     # List available backups
tmidb-cli backup verify backup-20240101   # Verify backup integrity

# Diagnostics
tmidb-cli diagnose all                    # Complete system diagnostics
tmidb-cli diagnose component api          # Diagnose specific component
tmidb-cli diagnose connectivity           # Check connectivity
tmidb-cli diagnose performance            # Performance analysis
tmidb-cli diagnose fix --dry-run          # Fix issues (dry-run)

# JSON output support
tmidb-cli status --output json            # JSON output
tmidb-cli process list -o json-pretty     # Pretty JSON output
```

### Environment Variables

- `TMIDB_SOCKET_PATH`: Unix socket path for IPC communication (default: `/tmp/tmidb-supervisor.sock`)

For more details, see [CLI Blueprint](cli_blueprint.md) and [CLI Development Summary](cli_development_summary.md).
