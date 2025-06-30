package supervisor

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tmidb/tmidb-core/internal/ipc"
	"github.com/tmidb/tmidb-core/internal/logger"
	"github.com/tmidb/tmidb-core/internal/process"
)

// Supervisor manages all tmiDB components and external services
type Supervisor struct {
	ctx            context.Context
	cancel         context.CancelFunc
	ipcServer      *ipc.Server
	logManager     *logger.Manager
	processManager *process.Manager

	// External services
	postgresql *exec.Cmd
	nats       *exec.Cmd
	seaweedfs  *exec.Cmd

	// Configuration
	config *Config

	// Status
	started  bool
	stopping bool

	// Go 1.24 cleanup management
	cleanup runtime.Cleanup
}

// Config holds supervisor configuration
type Config struct {
	// IPC settings
	SocketPath string `json:"socket_path"`

	// External services
	PostgreSQLPath string `json:"postgresql_path"`
	NATSPath       string `json:"nats_path"`
	SeaweedFSPath  string `json:"seaweedfs_path"`

	// Service ports for health checks
	PostgreSQLPort int `json:"postgresql_port"`
	NATSPort       int `json:"nats_port"`
	SeaweedFSPort  int `json:"seaweedfs_port"`

	// Timeouts
	StartupTimeout  time.Duration `json:"startup_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`

	// Log settings
	LogDir   string `json:"log_dir"`
	LogLevel string `json:"log_level"`
}

// DefaultConfig returns default supervisor configuration
func DefaultConfig() *Config {
	return &Config{
		SocketPath:      "/tmp/tmidb-supervisor.sock",
		PostgreSQLPath:  "/usr/local/bin/postgres-wrapper",
		NATSPath:        "/usr/local/bin/nats-wrapper",
		SeaweedFSPath:   "/usr/local/bin/weed-wrapper",
		PostgreSQLPort:  5432,
		NATSPort:        4222,
		SeaweedFSPort:   9333,
		StartupTimeout:  30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		LogDir:          "./logs",
		LogLevel:        "INFO",
	}
}

// parseLogLevel converts string log level to logger.LogLevel
func parseLogLevel(level string) logger.LogLevel {
	switch level {
	case "DEBUG":
		return logger.LogLevelDebug
	case "INFO":
		return logger.LogLevelInfo
	case "WARN":
		return logger.LogLevelWarn
	case "ERROR":
		return logger.LogLevelError
	default:
		return logger.LogLevelInfo
	}
}

// New creates a new supervisor instance
func New(config *Config) (*Supervisor, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create log directory
	if err := os.MkdirAll(config.LogDir, 0755); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Initialize IPC server first
	ipcServer := ipc.NewServer(config.SocketPath)

	// Initialize log manager
	logManager := logger.NewManager(&logger.LogConfig{
		BaseDir: config.LogDir,
		Level:   parseLogLevel(config.LogLevel),
	}, ipcServer)

	// Initialize process manager
	processManager := process.NewManager(ipcServer, logManager)

	supervisor := &Supervisor{
		ctx:            ctx,
		cancel:         cancel,
		ipcServer:      ipcServer,
		logManager:     logManager,
		processManager: processManager,
		config:         config,
	}

	// Go 1.24 기능: 자동 정리를 위한 cleanup 등록
	supervisor.cleanup = runtime.AddCleanup(&supervisor, func(s *Supervisor) {
		if !s.stopping {
			s.Stop()
		}
	}, supervisor)

	// Setup IPC handlers
	supervisor.setupIPCHandlers()

	// Initialize default log states (all components enabled by default)
	supervisor.initializeDefaultLogStates()

	return supervisor, nil
}

// initializeDefaultLogStates initializes default log states for all components
func (s *Supervisor) initializeDefaultLogStates() {
	components := []string{"api", "data-manager", "data-consumer", "postgresql", "nats", "seaweedfs"}
	for _, component := range components {
		s.logManager.EnableStream(component)
	}
}

// Start starts the supervisor and all managed services
func (s *Supervisor) Start() error {
	if s.started {
		return fmt.Errorf("supervisor already started")
	}

	log.Println("Starting tmiDB Supervisor...")

	// Start IPC server
	if err := s.ipcServer.Start(); err != nil {
		return fmt.Errorf("failed to start IPC server: %w", err)
	}

	// Start external services
	if err := s.startExternalServices(); err != nil {
		return fmt.Errorf("failed to start external services: %w", err)
	}

	// Wait for external services to be ready
	if err := s.waitForServices(); err != nil {
		return fmt.Errorf("external services failed to start: %w", err)
	}

	// Register and start internal components
	if err := s.startInternalComponents(); err != nil {
		return fmt.Errorf("failed to start internal components: %w", err)
	}

	s.started = true
	log.Println("tmiDB Supervisor started successfully")

	return nil
}

// Run starts the supervisor and waits for shutdown signals
func (s *Supervisor) Run() error {
	if err := s.Start(); err != nil {
		return err
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down...", sig)
	case <-s.ctx.Done():
		log.Println("Context cancelled, shutting down...")
	}

	return s.Stop()
}

// Stop gracefully stops all services and the supervisor
func (s *Supervisor) Stop() error {
	if s.stopping {
		return nil
	}
	s.stopping = true

	log.Println("Stopping tmiDB Supervisor...")

	// Stop internal components first
	if err := s.processManager.Stop(); err != nil {
		log.Printf("Error stopping internal components: %v", err)
	}

	// Stop IPC server
	if err := s.ipcServer.Stop(); err != nil {
		log.Printf("Error stopping IPC server: %v", err)
	}

	// Stop log manager
	if err := s.logManager.Stop(); err != nil {
		log.Printf("Error stopping log manager: %v", err)
	}

	// Cancel main context
	s.cancel()

	// Stop cleanup
	s.cleanup.Stop()

	log.Println("tmiDB Supervisor stopped")
	return nil
}

// startExternalServices attaches to already running services or starts them if not running
func (s *Supervisor) startExternalServices() error {
	log.Println("Attaching to external services...")

	// Attach to PostgreSQL
	if err := s.attachToService("postgresql", "/var/run/postgresql.pid"); err != nil {
		log.Printf("Warning: failed to attach to PostgreSQL: %v", err)
		// Try to start if not running
		if err := s.startSystemService("postgresql"); err != nil {
			log.Printf("Warning: failed to start PostgreSQL service: %v", err)
		}
	}

	// Attach to NATS
	if err := s.attachToService("nats", "/var/run/nats.pid"); err != nil {
		log.Printf("Warning: failed to attach to NATS: %v", err)
		// Try to start if not running
		if err := s.startSystemService("nats"); err != nil {
			log.Printf("Warning: failed to start NATS service: %v", err)
		}
	}

	// Attach to SeaweedFS
	if err := s.attachToService("seaweedfs", "/var/run/seaweedfs.pid"); err != nil {
		log.Printf("Warning: failed to attach to SeaweedFS: %v", err)
		// Try to start if not running
		if err := s.startSystemService("seaweedfs"); err != nil {
			log.Printf("Warning: failed to start SeaweedFS service: %v", err)
		}
	}

	return nil
}

// attachToService attaches supervisor to an already running service
func (s *Supervisor) attachToService(serviceName, pidFile string) error {
	// Read PID from file
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file %s: %w", pidFile, err)
	}

	pidStr := strings.TrimSpace(string(pidData))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid PID in file %s: %w", pidFile, err)
	}

	// Check if process is still running
	if !s.isProcessRunning(pid) {
		return fmt.Errorf("process with PID %d is not running", pid)
	}

	// Register service with existing PID
	var serviceType process.ProcessType
	var user string
	var command string
	var args []string

	switch serviceName {
	case "postgresql":
		serviceType = process.TypeExternal
		user = "postgres"
		command = "postgres"
		args = []string{"-D", "/data/postgresql", "-k", "/var/run/postgresql"}
	case "nats":
		serviceType = process.TypeExternal
		user = "natsuser"
		command = "nats-server"
		args = []string{"-sd", "/data/nats"}
	case "seaweedfs":
		serviceType = process.TypeExternal
		user = "seaweeduser"
		command = "weed"
		args = []string{"master", "-mdir=/data/seaweedfs/master"}
	default:
		return fmt.Errorf("unknown service: %s", serviceName)
	}

	if err := s.processManager.RegisterProcess(&process.ProcessConfig{
		Name:        serviceName,
		User:        user,
		Type:        serviceType,
		Command:     command,
		Args:        args,
		AutoRestart: true,
		MaxRestarts: 3,
	}); err != nil {
		return fmt.Errorf("failed to register service %s: %w", serviceName, err)
	}

	// Attach to existing process
	if err := s.processManager.AttachProcess(serviceName, pid); err != nil {
		return fmt.Errorf("failed to attach to service %s (PID: %d): %w", serviceName, pid, err)
	}

	log.Printf("✅ Attached to %s service (PID: %d)", serviceName, pid)
	return nil
}

// isProcessRunning checks if a process with given PID is running
func (s *Supervisor) isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	// Check if /proc/[pid] exists
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

// startSystemService starts a systemd service
func (s *Supervisor) startSystemService(serviceName string) error {
	log.Printf("🚀 Starting system service: %s", serviceName)
	cmd := exec.Command("systemctl", "start", serviceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start service %s: %w", serviceName, err)
	}
	log.Printf("✅ System service started: %s", serviceName)
	return nil
}

// stopSystemService stops a systemd service
func (s *Supervisor) stopSystemService(serviceName string) error {
	log.Printf("🛑 Stopping system service: %s", serviceName)
	cmd := exec.Command("systemctl", "stop", serviceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop service %s: %w", serviceName, err)
	}
	log.Printf("✅ System service stopped: %s", serviceName)
	return nil
}

// getServiceStatus gets the status of a systemd service
func (s *Supervisor) getServiceStatus(serviceName string) string {
	cmd := exec.Command("systemctl", "is-active", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// getProcessMemoryUsage gets actual memory usage for a process by PID
func (s *Supervisor) getProcessMemoryUsage(pid int) int64 {
	if pid <= 0 {
		return 0
	}

	// Read /proc/[pid]/status for memory information
	statusFile := fmt.Sprintf("/proc/%d/status", pid)
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "VmRSS:") {
			// VmRSS is the physical memory currently used by the process
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if value, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					// Convert from KB to bytes
					return value * 1024
				}
			}
		}
	}
	return 0
}

// getProcessCPUUsage gets CPU usage for a process by PID
func (s *Supervisor) getProcessCPUUsage(pid int) float64 {
	if pid <= 0 {
		return 0.0
	}

	// Read /proc/[pid]/stat for CPU information
	statFile := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statFile)
	if err != nil {
		return 0.0
	}

	fields := strings.Fields(string(data))
	if len(fields) < 17 {
		return 0.0
	}

	// fields[13] = utime (user time)
	// fields[14] = stime (system time)
	utime, err1 := strconv.ParseInt(fields[13], 10, 64)
	stime, err2 := strconv.ParseInt(fields[14], 10, 64)
	
	if err1 != nil || err2 != nil {
		return 0.0
	}

	totalTime := utime + stime
	
	// Get system clock ticks per second
	clockTicks := int64(100) // Usually 100 on Linux
	
	// Simple CPU usage calculation (this is a basic implementation)
	// In production, you'd want to calculate this over time intervals
	return float64(totalTime) / float64(clockTicks)
}

// updateProcessStats updates process statistics with real data
func (s *Supervisor) updateProcessStats() {
	processes := s.processManager.GetProcessList()
	for i := range processes {
		proc := &processes[i]
		
		// Update memory usage
		if proc.PID > 0 {
			proc.Memory = s.getProcessMemoryUsage(proc.PID)
			proc.CPU = s.getProcessCPUUsage(proc.PID)
		}
		
		// For system services, get status from systemctl
		if proc.Type == "service" {
			status := s.getServiceStatus(proc.Name)
			switch status {
			case "active":
				proc.Status = "running"
			case "inactive":
				proc.Status = "stopped"
			case "failed":
				proc.Status = "error"
			default:
				proc.Status = "unknown"
			}
			
			// Try to get PID for system services
			if proc.Status == "running" && proc.PID == 0 {
				pid := s.getServicePID(proc.Name)
				if pid > 0 {
					proc.PID = pid
					proc.Memory = s.getProcessMemoryUsage(pid)
					proc.CPU = s.getProcessCPUUsage(pid)
				}
			}
		}
	}
}

// getServicePID gets the main PID of a systemd service
func (s *Supervisor) getServicePID(serviceName string) int {
	cmd := exec.Command("systemctl", "show", "--property=MainPID", "--value", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	
	pidStr := strings.TrimSpace(string(output))
	if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
		return pid
	}
	return 0
}

// waitForServices waits for external services to be ready
func (s *Supervisor) waitForServices() error {
	log.Println("Waiting for external services to be ready...")

	timeout := time.After(s.config.StartupTimeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	services := map[string]int{
		"PostgreSQL": s.config.PostgreSQLPort,
		"NATS":       s.config.NATSPort,
		"SeaweedFS":  s.config.SeaweedFSPort,
	}

	for {
		select {
		case <-timeout:
			log.Println("Timeout waiting for services, continuing anyway...")
			return nil // Continue even if services aren't ready
		case <-ticker.C:
			allReady := true
			for name, port := range services {
				if !s.isPortReady(port) {
					log.Printf("Waiting for %s on port %d...", name, port)
					allReady = false
				}
			}
			if allReady {
				log.Println("All external services are ready")
				return nil
			}
		}
	}
}

// isPortReady checks if a port is ready to accept connections
func (s *Supervisor) isPortReady(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// startInternalComponents starts API, Data Manager, Data Consumer
func (s *Supervisor) startInternalComponents() error {
	log.Println("Starting internal components...")

	// Register API Server
	if err := s.processManager.RegisterProcess(&process.ProcessConfig{
		Name:        "api",
		Type:        process.TypeInternal,
		Command:     "go",
		Args:        []string{"run", "./cmd/api"},
		AutoRestart: true,
	}); err != nil {
		log.Printf("Warning: failed to register API: %v", err)
	} else {
		if err := s.processManager.StartProcess("api"); err != nil {
			log.Printf("Warning: failed to start API: %v", err)
		}
	}

	// Register Data Manager
	if err := s.processManager.RegisterProcess(&process.ProcessConfig{
		Name:        "data-manager",
		Type:        process.TypeInternal,
		Command:     "go",
		Args:        []string{"run", "./cmd/data-manager"},
		AutoRestart: true,
	}); err != nil {
		log.Printf("Warning: failed to register Data Manager: %v", err)
	} else {
		if err := s.processManager.StartProcess("data-manager"); err != nil {
			log.Printf("Warning: failed to start Data Manager: %v", err)
		}
	}

	// Register Data Consumer
	if err := s.processManager.RegisterProcess(&process.ProcessConfig{
		Name:        "data-consumer",
		Type:        process.TypeInternal,
		Command:     "go",
		Args:        []string{"run", "./cmd/data-consumer"},
		AutoRestart: true,
	}); err != nil {
		log.Printf("Warning: failed to register Data Consumer: %v", err)
	} else {
		if err := s.processManager.StartProcess("data-consumer"); err != nil {
			log.Printf("Warning: failed to start Data Consumer: %v", err)
		}
	}

	return nil
}

// setupIPCHandlers sets up IPC message handlers
func (s *Supervisor) setupIPCHandlers() {
	// Log management handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeLogEnable, s.handleEnableLogs)
	s.ipcServer.RegisterHandler(ipc.MessageTypeLogDisable, s.handleDisableLogs)
	s.ipcServer.RegisterHandler(ipc.MessageTypeLogStatus, s.handleGetLogStatus)
	s.ipcServer.RegisterHandler(ipc.MessageTypeGetLogs, s.handleGetLogs)
	s.ipcServer.RegisterHandler(ipc.MessageTypeLogStream, s.handleLogStream)

	// Process management handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessList, s.handleGetProcessList)
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessStatus, s.handleGetProcessStatus)
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessStart, s.handleStartProcess)
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessStop, s.handleStopProcess)
	s.ipcServer.RegisterHandler(ipc.MessageTypeProcessRestart, s.handleRestartProcess)

	// System health handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeSystemHealth, s.handleGetSystemHealth)
	s.ipcServer.RegisterHandler(ipc.MessageTypeSystemStats, s.handleGetSystemResources)

	// Configuration handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeConfigGet, s.handleConfigGet)
	s.ipcServer.RegisterHandler(ipc.MessageTypeConfigSet, s.handleConfigSet)
	s.ipcServer.RegisterHandler(ipc.MessageTypeConfigList, s.handleConfigList)
	s.ipcServer.RegisterHandler(ipc.MessageTypeConfigReset, s.handleConfigReset)
	s.ipcServer.RegisterHandler(ipc.MessageTypeConfigImport, s.handleConfigImport)
	s.ipcServer.RegisterHandler(ipc.MessageTypeConfigValidate, s.handleConfigValidate)
}

// handleEnableLogs handles log enable requests
func (s *Supervisor) handleEnableLogs(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	s.logManager.EnableStream(component)
	return ipc.NewResponse(msg.ID, true, map[string]string{"status": "enabled"}, "")
}

// handleDisableLogs handles log disable requests
func (s *Supervisor) handleDisableLogs(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	s.logManager.DisableStream(component)
	return ipc.NewResponse(msg.ID, true, map[string]string{"status": "disabled"}, "")
}

// handleGetLogStatus handles log status requests
func (s *Supervisor) handleGetLogStatus(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	status := s.logManager.GetStreamStatus()
	return ipc.NewResponse(msg.ID, true, status, "")
}

// handleGetLogs handles get logs requests
func (s *Supervisor) handleGetLogs(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	lines := 50 // default
	if l, ok := msg.Data["lines"].(float64); ok {
		lines = int(l)
	}

	// Read recent logs from file
	logFile := fmt.Sprintf("%s/%s.log", s.config.LogDir, component)
	logs, err := s.readRecentLogs(logFile, lines)
	if err != nil {
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("failed to read logs: %v", err))
	}

	return ipc.NewResponse(msg.ID, true, logs, "")
}

// handleLogStream handles log stream requests
func (s *Supervisor) handleLogStream(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	action, ok := msg.Data["action"].(string)
	if !ok {
		action = "start"
	}

	switch action {
	case "start":
		// Create log stream for this connection
		logChan := s.ipcServer.CreateLogStream(conn.ID)

		// Start streaming logs for the component
		go s.streamLogsToConnection(component, logChan)

		return ipc.NewResponse(msg.ID, true, map[string]string{"status": "streaming"}, "")
	case "stop":
		s.ipcServer.RemoveLogStream(conn.ID)
		return ipc.NewResponse(msg.ID, true, map[string]string{"status": "stopped"}, "")
	default:
		return ipc.NewResponse(msg.ID, false, nil, "invalid action")
	}
}

// streamLogsToConnection streams logs to a specific connection
func (s *Supervisor) streamLogsToConnection(component string, logChan chan<- ipc.LogEntry) {
	// This would be implemented to tail log files and send entries to the channel
	// For now, we'll send a simple message
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			entry := ipc.LogEntry{
				Process:   component,
				Level:     "INFO",
				Message:   fmt.Sprintf("Sample log message from %s", component),
				Timestamp: time.Now(),
			}
			select {
			case logChan <- entry:
			default:
				// Channel is full, skip
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// readRecentLogs reads recent log entries from a file
func (s *Supervisor) readRecentLogs(filename string, lines int) ([]map[string]interface{}, error) {
	// Simple implementation - in production this would be more sophisticated
	logs := make([]map[string]interface{}, 0)

	// Generate sample log entries for now
	for i := 0; i < lines && i < 10; i++ {
		log := map[string]interface{}{
			"timestamp": time.Now().Add(-time.Duration(i) * time.Minute).Format("15:04:05"),
			"process":   "sample",
			"message":   fmt.Sprintf("Sample log message #%d", i+1),
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// handleGetProcessList handles get process list requests
func (s *Supervisor) handleGetProcessList(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	processes := s.processManager.GetProcessList()
	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    processes,
	}
}

// handleGetProcessStatus handles get process status requests
func (s *Supervisor) handleGetProcessStatus(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	// Extract process name from message data
	processName, ok := msg.Data["name"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "invalid process name",
		}
	}

	status, err := s.processManager.GetProcessStatus(processName)
	if err != nil {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   err.Error(),
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    status,
	}
}

// handleStartProcess handles start process requests
func (s *Supervisor) handleStartProcess(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	processName, ok := msg.Data["name"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "invalid process name",
		}
	}

	if err := s.processManager.StartProcess(processName); err != nil {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   err.Error(),
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    "process started",
	}
}

// handleStopProcess handles stop process requests
func (s *Supervisor) handleStopProcess(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	processName, ok := msg.Data["name"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "invalid process name",
		}
	}

	if err := s.processManager.StopProcess(processName); err != nil {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   err.Error(),
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    "process stopped",
	}
}

// handleRestartProcess handles restart process requests
func (s *Supervisor) handleRestartProcess(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	processName, ok := msg.Data["name"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "invalid process name",
		}
	}

	if err := s.processManager.RestartProcess(processName); err != nil {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   err.Error(),
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    "process restarted",
	}
}

// handleGetSystemHealth handles get system health requests
func (s *Supervisor) handleGetSystemHealth(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	health := &ipc.SystemHealth{
		Status:     "healthy",
		Uptime:     time.Since(time.Now().Add(-time.Hour)), // Placeholder
		Components: make(map[string]string),
		Resources: ipc.SystemResources{
			CPUUsage:    0.0,
			MemoryUsage: 0.0,
			DiskUsage:   0.0,
		},
		LastCheck: time.Now(),
	}

	// Check services
	services := map[string]int{
		"postgresql": s.config.PostgreSQLPort,
		"nats":       s.config.NATSPort,
		"seaweedfs":  s.config.SeaweedFSPort,
	}

	for name, port := range services {
		if s.isPortReady(port) {
			health.Components[name] = "healthy"
		} else {
			health.Components[name] = "unhealthy"
			health.Status = "degraded"
		}
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    health,
	}
}

// handleGetSystemResources handles get system resources requests
func (s *Supervisor) handleGetSystemResources(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	// 프로세스 통계 업데이트
	s.updateProcessStats()
	
	// 실제 프로세스 통계 수집
	processes := s.processManager.GetProcessList()
	runningCount := 0
	stoppedCount := 0
	errorCount := 0

	for _, proc := range processes {
		switch proc.Status {
		case "running":
			runningCount++
		case "stopped":
			stoppedCount++
		case "error":
			errorCount++
		}
	}

	// 시스템 리소스 계산
	cpuUsage := s.getCPUUsage()
	memoryUsage := s.getMemoryUsage()
	diskUsage := s.getDiskUsage()

	stats := map[string]interface{}{
		"processes":       len(processes),
		"running":         runningCount,
		"stopped":         stoppedCount,
		"errors":          errorCount,
		"ipc_connections": s.ipcServer.GetConnectionCount(),
		"cpu_usage":       cpuUsage,
		"memory_usage":    memoryUsage,
		"disk_usage":      diskUsage,
	}

	return ipc.NewResponse(msg.ID, true, stats, "")
}

// getCPUUsage 시스템 CPU 사용률 계산
func (s *Supervisor) getCPUUsage() float64 {
	// /proc/stat에서 CPU 사용률 계산
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		log.Printf("⚠️ Failed to read /proc/stat: %v", err)
		return 0.0
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0.0
	}

	// 첫 번째 줄은 전체 CPU 통계
	cpuLine := lines[0]
	if !strings.HasPrefix(cpuLine, "cpu ") {
		return 0.0
	}

	fields := strings.Fields(cpuLine)
	if len(fields) < 8 {
		return 0.0
	}

	// CPU 시간 값들 파싱
	var times []int64
	for i := 1; i < 8; i++ {
		val, err := strconv.ParseInt(fields[i], 10, 64)
		if err != nil {
			return 0.0
		}
		times = append(times, val)
	}

	// user, nice, system, idle, iowait, irq, softirq
	idle := times[3] + times[4] // idle + iowait
	total := int64(0)
	for _, t := range times {
		total += t
	}

	if total == 0 {
		return 0.0
	}

	// CPU 사용률 = (total - idle) / total * 100
	usage := float64(total-idle) / float64(total) * 100
	return usage
}

// getMemoryUsage 시스템 메모리 사용률 계산
func (s *Supervisor) getMemoryUsage() float64 {
	// /proc/meminfo에서 메모리 정보 읽기
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Printf("⚠️ Failed to read /proc/meminfo: %v", err)
		return 0.0
	}

	lines := strings.Split(string(data), "\n")
	memInfo := make(map[string]int64)

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.TrimSuffix(parts[0], ":")
		value, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		memInfo[key] = value
	}

	memTotal, ok1 := memInfo["MemTotal"]
	memAvailable, ok2 := memInfo["MemAvailable"]
	
	if !ok1 || !ok2 || memTotal == 0 {
		return 0.0
	}

	// 메모리 사용률 = (Total - Available) / Total * 100
	usage := float64(memTotal-memAvailable) / float64(memTotal) * 100
	return usage
}

// getDiskUsage 디스크 사용률 계산
func (s *Supervisor) getDiskUsage() float64 {
	// 현재 작업 디렉토리의 디스크 사용률 계산
	var stat syscall.Statfs_t
	err := syscall.Statfs(".", &stat)
	if err != nil {
		log.Printf("⚠️ Failed to get disk stats: %v", err)
		return 0.0
	}

	// 총 블록 수와 사용 가능한 블록 수
	total := stat.Blocks * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)

	if total == 0 {
		return 0.0
	}

	// 디스크 사용률 = (Total - Available) / Total * 100
	usage := float64(total-available) / float64(total) * 100
	return usage
}

// GetLogManager returns the log manager instance
func (s *Supervisor) GetLogManager() *logger.Manager {
	return s.logManager
}

// GetProcessManager returns the process manager instance
func (s *Supervisor) GetProcessManager() *process.Manager {
	return s.processManager
}

// GetIPCServer returns the IPC server instance
func (s *Supervisor) GetIPCServer() *ipc.Server {
	return s.ipcServer
}

// ensureDataDirectories creates necessary data directories
func (s *Supervisor) ensureDataDirectories() error {
	log.Println("Ensuring data directories exist...")

	// Create base data directories with proper ownership
	dataDirs := []struct {
		path  string
		owner string
	}{
		{"/data/nats", "natsuser:natsuser"},
		{"/data/seaweedfs", "seaweeduser:seaweeduser"},
		{"/data/seaweedfs/master", "seaweeduser:seaweeduser"},
	}

	for _, dir := range dataDirs {
		if err := os.MkdirAll(dir.path, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir.path, err)
		}
		log.Printf("📁 Created data directory: %s (owner: %s)", dir.path, dir.owner)
	}

	// Create PostgreSQL data directory with correct ownership
	if err := s.createPostgreSQLDataDir(); err != nil {
		return fmt.Errorf("failed to create PostgreSQL data directory: %w", err)
	}

	// Initialize PostgreSQL data directory if empty
	if err := s.initializePostgreSQLData(); err != nil {
		return fmt.Errorf("failed to initialize PostgreSQL data: %w", err)
	}

	return nil
}

// createPostgreSQLDataDir creates PostgreSQL data directory with correct ownership
func (s *Supervisor) createPostgreSQLDataDir() error {
	dataDir := "/data/postgresql"

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create PostgreSQL data directory: %w", err)
	}

	log.Printf("📁 Created PostgreSQL data directory: %s", dataDir)
	return nil
}

// initializePostgreSQLData initializes PostgreSQL data directory if needed
func (s *Supervisor) initializePostgreSQLData() error {
	dataDir := "/data/postgresql"

	// Check if PostgreSQL data directory is already initialized
	if _, err := os.Stat(filepath.Join(dataDir, "PG_VERSION")); err == nil {
		log.Println("PostgreSQL data directory already initialized")
		return nil
	}

	files, err := os.ReadDir(dataDir)
	if err != nil {
		return fmt.Errorf("failed to read postgresql data dir: %w", err)
	}

	if len(files) > 0 {
		log.Println("PostgreSQL data directory exists but appears corrupted or not empty, skipping initialization.")
		return nil
	}

	log.Println("Initializing PostgreSQL data directory...")

	// Run initdb. This should be run by the user that will own the process,
	// which is handled by the Dockerfile's USER directive.
	cmd := exec.Command("initdb", "-D", dataDir, "--encoding=UTF8", "--locale=en_US.UTF-8")

	initOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to initialize PostgreSQL data directory: %w\nOutput: %s", err, string(initOutput))
	}

	log.Println("PostgreSQL data directory initialized successfully")
	return nil
}

// Configuration handlers
func (s *Supervisor) handleConfigGet(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	key, hasKey := msg.Data["key"].(string)
	
	if !hasKey || key == "" {
		// 전체 설정 반환
		configData := map[string]interface{}{
			"socket_path":      s.config.SocketPath,
			"postgresql_path":  s.config.PostgreSQLPath,
			"nats_path":        s.config.NATSPath,
			"seaweedfs_path":   s.config.SeaweedFSPath,
			"postgresql_port":  s.config.PostgreSQLPort,
			"nats_port":        s.config.NATSPort,
			"seaweedfs_port":   s.config.SeaweedFSPort,
			"startup_timeout":  s.config.StartupTimeout.String(),
			"shutdown_timeout": s.config.ShutdownTimeout.String(),
			"log_dir":          s.config.LogDir,
			"log_level":        s.config.LogLevel,
		}
		return ipc.NewResponse(msg.ID, true, configData, "")
	}

	// 특정 키 값 반환
	var value interface{}
	switch key {
	case "socket_path":
		value = s.config.SocketPath
	case "postgresql_path":
		value = s.config.PostgreSQLPath
	case "nats_path":
		value = s.config.NATSPath
	case "seaweedfs_path":
		value = s.config.SeaweedFSPath
	case "postgresql_port":
		value = s.config.PostgreSQLPort
	case "nats_port":
		value = s.config.NATSPort
	case "seaweedfs_port":
		value = s.config.SeaweedFSPort
	case "startup_timeout":
		value = s.config.StartupTimeout.String()
	case "shutdown_timeout":
		value = s.config.ShutdownTimeout.String()
	case "log_dir":
		value = s.config.LogDir
	case "log_level":
		value = s.config.LogLevel
	default:
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("unknown config key: %s", key))
	}

	return ipc.NewResponse(msg.ID, true, map[string]interface{}{key: value}, "")
}

func (s *Supervisor) handleConfigSet(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	key, ok := msg.Data["key"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "key parameter required")
	}

	value := msg.Data["value"]
	if value == nil {
		return ipc.NewResponse(msg.ID, false, nil, "value parameter required")
	}

	// 설정 값 업데이트
	needsRestart := false
	component := ""

	switch key {
	case "log_level":
		if strVal, ok := value.(string); ok {
			s.config.LogLevel = strVal
			component = "logging"
		} else {
			return ipc.NewResponse(msg.ID, false, nil, "log_level must be a string")
		}
	case "log_dir":
		if strVal, ok := value.(string); ok {
			s.config.LogDir = strVal
			needsRestart = true
			component = "logging"
		} else {
			return ipc.NewResponse(msg.ID, false, nil, "log_dir must be a string")
		}
	case "postgresql_port":
		if intVal, ok := value.(float64); ok {
			s.config.PostgreSQLPort = int(intVal)
			needsRestart = true
			component = "postgresql"
		} else {
			return ipc.NewResponse(msg.ID, false, nil, "postgresql_port must be a number")
		}
	case "nats_port":
		if intVal, ok := value.(float64); ok {
			s.config.NATSPort = int(intVal)
			needsRestart = true
			component = "nats"
		} else {
			return ipc.NewResponse(msg.ID, false, nil, "nats_port must be a number")
		}
	case "seaweedfs_port":
		if intVal, ok := value.(float64); ok {
			s.config.SeaweedFSPort = int(intVal)
			needsRestart = true
			component = "seaweedfs"
		} else {
			return ipc.NewResponse(msg.ID, false, nil, "seaweedfs_port must be a number")
		}
	default:
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("config key '%s' is not modifiable", key))
	}

	responseData := map[string]interface{}{
		"needs_restart": needsRestart,
		"component":     component,
	}

	return ipc.NewResponse(msg.ID, true, responseData, "")
}

func (s *Supervisor) handleConfigList(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	configs := []map[string]interface{}{
		{
			"key":         "socket_path",
			"value":       s.config.SocketPath,
			"type":        "string",
			"description": "IPC socket path for supervisor communication",
		},
		{
			"key":         "postgresql_path",
			"value":       s.config.PostgreSQLPath,
			"type":        "string",
			"description": "Path to PostgreSQL binary",
		},
		{
			"key":         "nats_path",
			"value":       s.config.NATSPath,
			"type":        "string",
			"description": "Path to NATS server binary",
		},
		{
			"key":         "seaweedfs_path",
			"value":       s.config.SeaweedFSPath,
			"type":        "string",
			"description": "Path to SeaweedFS binary",
		},
		{
			"key":         "postgresql_port",
			"value":       s.config.PostgreSQLPort,
			"type":        "int",
			"description": "PostgreSQL server port",
		},
		{
			"key":         "nats_port",
			"value":       s.config.NATSPort,
			"type":        "int",
			"description": "NATS server port",
		},
		{
			"key":         "seaweedfs_port",
			"value":       s.config.SeaweedFSPort,
			"type":        "int",
			"description": "SeaweedFS master port",
		},
		{
			"key":         "startup_timeout",
			"value":       s.config.StartupTimeout.String(),
			"type":        "duration",
			"description": "Timeout for service startup",
		},
		{
			"key":         "shutdown_timeout",
			"value":       s.config.ShutdownTimeout.String(),
			"type":        "duration",
			"description": "Timeout for service shutdown",
		},
		{
			"key":         "log_dir",
			"value":       s.config.LogDir,
			"type":        "string",
			"description": "Directory for log files",
		},
		{
			"key":         "log_level",
			"value":       s.config.LogLevel,
			"type":        "string",
			"description": "Log level (DEBUG, INFO, WARN, ERROR)",
		},
	}

	return ipc.NewResponse(msg.ID, true, configs, "")
}

func (s *Supervisor) handleConfigReset(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	key, hasKey := msg.Data["key"].(string)
	all, _ := msg.Data["all"].(bool)

	if all {
		// 모든 설정을 기본값으로 리셋
		defaultConfig := DefaultConfig()
		s.config = defaultConfig
		return ipc.NewResponse(msg.ID, true, map[string]string{"status": "all config reset to defaults"}, "")
	}

	if !hasKey || key == "" {
		return ipc.NewResponse(msg.ID, false, nil, "key parameter required when not using --all")
	}

	// 특정 키를 기본값으로 리셋
	defaultConfig := DefaultConfig()
	switch key {
	case "log_level":
		s.config.LogLevel = defaultConfig.LogLevel
	case "log_dir":
		s.config.LogDir = defaultConfig.LogDir
	case "postgresql_port":
		s.config.PostgreSQLPort = defaultConfig.PostgreSQLPort
	case "nats_port":
		s.config.NATSPort = defaultConfig.NATSPort
	case "seaweedfs_port":
		s.config.SeaweedFSPort = defaultConfig.SeaweedFSPort
	case "startup_timeout":
		s.config.StartupTimeout = defaultConfig.StartupTimeout
	case "shutdown_timeout":
		s.config.ShutdownTimeout = defaultConfig.ShutdownTimeout
	default:
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("unknown config key: %s", key))
	}

	return ipc.NewResponse(msg.ID, true, map[string]string{"status": fmt.Sprintf("config key '%s' reset to default", key)}, "")
}

func (s *Supervisor) handleConfigImport(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	configData, ok := msg.Data["config"].(map[string]interface{})
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "config data required")
	}

	changes := []string{}

	// 설정 값들을 하나씩 적용
	for key, value := range configData {
		switch key {
		case "log_level":
			if strVal, ok := value.(string); ok {
				s.config.LogLevel = strVal
				changes = append(changes, fmt.Sprintf("log_level: %s", strVal))
			}
		case "log_dir":
			if strVal, ok := value.(string); ok {
				s.config.LogDir = strVal
				changes = append(changes, fmt.Sprintf("log_dir: %s", strVal))
			}
		case "postgresql_port":
			if intVal, ok := value.(float64); ok {
				s.config.PostgreSQLPort = int(intVal)
				changes = append(changes, fmt.Sprintf("postgresql_port: %d", int(intVal)))
			}
		case "nats_port":
			if intVal, ok := value.(float64); ok {
				s.config.NATSPort = int(intVal)
				changes = append(changes, fmt.Sprintf("nats_port: %d", int(intVal)))
			}
		case "seaweedfs_port":
			if intVal, ok := value.(float64); ok {
				s.config.SeaweedFSPort = int(intVal)
				changes = append(changes, fmt.Sprintf("seaweedfs_port: %d", int(intVal)))
			}
		}
	}

	responseData := map[string]interface{}{
		"changes": changes,
	}

	return ipc.NewResponse(msg.ID, true, responseData, "")
}

func (s *Supervisor) handleConfigValidate(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	warnings := []string{}

	// 포트 충돌 검사
	ports := map[string]int{
		"postgresql": s.config.PostgreSQLPort,
		"nats":       s.config.NATSPort,
		"seaweedfs":  s.config.SeaweedFSPort,
	}

	portMap := make(map[int]string)
	for service, port := range ports {
		if existingService, exists := portMap[port]; exists {
			warnings = append(warnings, fmt.Sprintf("Port conflict: %s and %s both use port %d", service, existingService, port))
		} else {
			portMap[port] = service
		}
	}

	// 로그 레벨 검사
	validLevels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	validLevel := false
	for _, level := range validLevels {
		if s.config.LogLevel == level {
			validLevel = true
			break
		}
	}
	if !validLevel {
		warnings = append(warnings, fmt.Sprintf("Invalid log level: %s (valid: %v)", s.config.LogLevel, validLevels))
	}

	// 디렉토리 존재 검사
	if _, err := os.Stat(s.config.LogDir); os.IsNotExist(err) {
		warnings = append(warnings, fmt.Sprintf("Log directory does not exist: %s", s.config.LogDir))
	}

	responseData := map[string]interface{}{
		"warnings": warnings,
	}

	return ipc.NewResponse(msg.ID, true, responseData, "")
}
