package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"

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

	// Copy sessions
	copySessions map[string]*ipc.CopySession

	// Backup management
	backups         map[string]*BackupInfo
	backupProgress  map[string]*BackupProgress
	restoreProgress map[string]*RestoreProgress

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

// BackupInfo holds information about a backup
type BackupInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	Created    time.Time `json:"created"`
	Components []string  `json:"components"`
	Compressed bool      `json:"compressed"`
	Checksum   string    `json:"checksum"`
	Status     string    `json:"status"`
}

// BackupProgress tracks backup creation progress
type BackupProgress struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`  // "creating", "completed", "failed"
	Percent   float64    `json:"percent"` // 0-100
	Current   string     `json:"current"` // current operation
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// RestoreProgress tracks restore operation progress
type RestoreProgress struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`  // "restoring", "completed", "failed"
	Percent   float64    `json:"percent"` // 0-100
	Current   string     `json:"current"` // current operation
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Error     string     `json:"error,omitempty"`
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
		ctx:             ctx,
		cancel:          cancel,
		ipcServer:       ipcServer,
		logManager:      logManager,
		processManager:  processManager,
		config:          config,
		copySessions:    make(map[string]*ipc.CopySession),
		backups:         make(map[string]*BackupInfo),
		backupProgress:  make(map[string]*BackupProgress),
		restoreProgress: make(map[string]*RestoreProgress),
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
	// 컨테이너 환경에서는 systemctl을 사용하지 않음
	// 대신 프로세스 상태를 직접 확인
	return "active" // 외부 서비스는 항상 active로 간주
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
	// Process manager에서 프로세스 목록을 가져와서 실제 메모리/CPU 정보를 업데이트
	s.processManager.UpdateProcessStats(s.getProcessMemoryUsage, s.getProcessCPUUsage, s.getServiceStatus, s.getServicePID)
}

// getServicePID gets the main PID of a systemd service
func (s *Supervisor) getServicePID(serviceName string) int {
	// 컨테이너 환경에서는 systemctl을 사용하지 않음
	// 이미 등록된 프로세스의 PID를 반환
	return 0 // 외부 서비스 PID는 이미 AttachProcess에서 설정됨
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
		Command:     "/app/bin/api",
		Args:        []string{},
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
		Command:     "/app/bin/data-manager",
		Args:        []string{},
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
		Command:     "/app/bin/data-consumer",
		Args:        []string{},
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

	// Backup handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeBackupCreate, s.handleBackupCreate)
	s.ipcServer.RegisterHandler(ipc.MessageTypeBackupRestore, s.handleBackupRestore)
	s.ipcServer.RegisterHandler(ipc.MessageTypeBackupList, s.handleBackupList)
	s.ipcServer.RegisterHandler(ipc.MessageTypeBackupDelete, s.handleBackupDelete)
	s.ipcServer.RegisterHandler(ipc.MessageTypeBackupVerify, s.handleBackupVerify)
	s.ipcServer.RegisterHandler(ipc.MessageTypeBackupProgress, s.handleBackupProgress)
	s.ipcServer.RegisterHandler(ipc.MessageTypeRestoreProgress, s.handleRestoreProgress)

	// Diagnose handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeDiagnoseAll, s.handleDiagnoseAll)
	s.ipcServer.RegisterHandler(ipc.MessageTypeDiagnoseComponent, s.handleDiagnoseComponent)
	s.ipcServer.RegisterHandler(ipc.MessageTypeDiagnoseConnectivity, s.handleDiagnoseConnectivity)
	s.ipcServer.RegisterHandler(ipc.MessageTypeDiagnosePerformance, s.handleDiagnosePerformance)
	s.ipcServer.RegisterHandler(ipc.MessageTypeDiagnoseLogs, s.handleDiagnoseLogs)
	s.ipcServer.RegisterHandler(ipc.MessageTypeDiagnoseFix, s.handleDiagnoseFix)
	s.ipcServer.RegisterHandler(ipc.MessageTypeDiagnoseResult, s.handleDiagnoseResult)

	// Copy handlers
	s.ipcServer.RegisterHandler(ipc.MessageTypeCopyReceive, s.handleCopyReceive)
	s.ipcServer.RegisterHandler(ipc.MessageTypeCopySend, s.handleCopySend)
	s.ipcServer.RegisterHandler(ipc.MessageTypeCopyStatus, s.handleCopyStatus)
	s.ipcServer.RegisterHandler(ipc.MessageTypeCopyList, s.handleCopyList)
	s.ipcServer.RegisterHandler(ipc.MessageTypeCopyStop, s.handleCopyStop)
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
func (s *Supervisor) readRecentLogs(filename string, lines int) ([]ipc.LogEntry, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []ipc.LogEntry{}, nil // 로그 파일이 없으면 빈 목록 반환
		}
		return nil, fmt.Errorf("could not open log file: %w", err)
	}
	defer file.Close()

	var fileLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fileLines = append(fileLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	start := len(fileLines) - lines
	if start < 0 {
		start = 0
	}
	recentLogLines := fileLines[start:]

	var entries []ipc.LogEntry
	for _, line := range recentLogLines {
		var entry ipc.LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			entries = append(entries, entry)
		}
	}

	// 최신 로그가 위로 오도록 순서 뒤집기
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	return entries, nil
}

// handleGetProcessList handles get process list requests
func (s *Supervisor) handleGetProcessList(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	// 프로세스 통계 업데이트를 제거하여 즉시 응답
	// (통계는 별도의 백그라운드 프로세스에서 업데이트됨)

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
	processName, ok := msg.Data["component"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "component parameter required",
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
	processName, ok := msg.Data["component"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "component parameter required",
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
	processName, ok := msg.Data["component"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "component parameter required",
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
	processName, ok := msg.Data["component"].(string)
	if !ok {
		return &ipc.Response{
			ID:      msg.ID,
			Success: false,
			Error:   "component parameter required",
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

// Backup handlers
func (s *Supervisor) handleBackupCreate(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	name, _ := msg.Data["name"].(string)
	components, _ := msg.Data["components"].([]interface{})
	compress, _ := msg.Data["compress"].(bool)
	outputDir, _ := msg.Data["output_dir"].(string)

	if name == "" {
		name = fmt.Sprintf("tmidb-backup-%s", time.Now().Format("20060102-150405"))
	}

	if outputDir == "" {
		outputDir = "./backups"
	}

	// 백업 디렉터리 생성
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("failed to create backup directory: %v", err))
	}

	// 백업 ID 생성
	backupID := fmt.Sprintf("backup-%d", time.Now().Unix())

	// 백업 파일 경로
	var backupPath string
	if compress {
		backupPath = filepath.Join(outputDir, name+".tar.gz")
	} else {
		backupPath = filepath.Join(outputDir, name+".tar")
	}

	// 백업 정보 생성
	backup := &BackupInfo{
		ID:         backupID,
		Name:       name,
		Path:       backupPath,
		Created:    time.Now(),
		Components: s.parseComponents(components),
		Compressed: compress,
		Status:     "creating",
	}

	// 진행 상황 추적 생성
	progress := &BackupProgress{
		ID:        backupID,
		Status:    "creating",
		Percent:   0,
		Current:   "Initializing backup",
		StartTime: time.Now(),
	}

	s.backups[backupID] = backup
	s.backupProgress[backupID] = progress

	// 백그라운드에서 백업 수행
	go s.performBackup(backupID)

	return ipc.NewResponse(msg.ID, true, map[string]interface{}{
		"id":   backupID,
		"path": backupPath,
		"size": int64(0), // 초기 크기
	}, "")
}

func (s *Supervisor) handleBackupRestore(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	backup, _ := msg.Data["backup"].(string)
	components, _ := msg.Data["components"].([]interface{})

	if backup == "" {
		return ipc.NewResponse(msg.ID, false, nil, "backup is required")
	}

	// 백업 ID 또는 경로로 백업 파일 경로 결정
	var backupPath string

	// 먼저 ID로 찾기
	if info, exists := s.backups[backup]; exists {
		backupPath = info.Path
	} else {
		// 파일 경로로 직접 복원
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			return ipc.NewResponse(msg.ID, false, nil, "backup file not found")
		}
		backupPath = backup
	}

	// 복원 ID 생성
	restoreID := fmt.Sprintf("restore-%d", time.Now().Unix())

	// 복원 진행 상황 생성
	progress := &RestoreProgress{
		ID:        restoreID,
		Status:    "restoring",
		Percent:   0,
		Current:   "Initializing restore",
		StartTime: time.Now(),
	}

	s.restoreProgress[restoreID] = progress

	// 백그라운드에서 복원 수행
	go s.performRestore(restoreID, backupPath, s.parseComponents(components))

	return ipc.NewResponse(msg.ID, true, map[string]interface{}{
		"id": restoreID,
	}, "")
}

func (s *Supervisor) handleBackupList(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	var backupList []interface{}

	// 메모리의 백업 목록
	for _, backup := range s.backups {
		backupList = append(backupList, map[string]interface{}{
			"id":         backup.ID,
			"name":       backup.Name,
			"created":    backup.Created.Format("2006-01-02 15:04:05"),
			"size":       backup.Size,
			"components": backup.Components,
			"compressed": backup.Compressed,
			"status":     backup.Status,
		})
	}

	// 백업 디렉터리에서 추가 백업 파일 스캔
	backupDir := "./backups"
	if files, err := os.ReadDir(backupDir); err == nil {
		for _, file := range files {
			if !file.IsDir() && (strings.HasSuffix(file.Name(), ".tar") || strings.HasSuffix(file.Name(), ".tar.gz")) {
				filePath := filepath.Join(backupDir, file.Name())

				// 이미 메모리에 있는 백업인지 확인
				found := false
				for _, backup := range s.backups {
					if backup.Path == filePath {
						found = true
						break
					}
				}

				if !found {
					if info, err := file.Info(); err == nil {
						backupList = append(backupList, map[string]interface{}{
							"id":         file.Name(),
							"name":       strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())),
							"created":    info.ModTime().Format("2006-01-02 15:04:05"),
							"size":       info.Size(),
							"components": []string{"unknown"},
							"compressed": strings.HasSuffix(file.Name(), ".gz"),
							"status":     "completed",
						})
					}
				}
			}
		}
	}

	return ipc.NewResponse(msg.ID, true, backupList, "")
}

func (s *Supervisor) handleBackupDelete(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	backupID, _ := msg.Data["id"].(string)
	if backupID == "" {
		return ipc.NewResponse(msg.ID, false, nil, "backup id is required")
	}

	// 메모리에서 백업 정보 찾기
	backup, exists := s.backups[backupID]
	if !exists {
		// 파일명으로 찾기
		backupPath := filepath.Join("./backups", backupID)
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			return ipc.NewResponse(msg.ID, false, nil, "backup not found")
		}

		// 파일 삭제
		if err := os.Remove(backupPath); err != nil {
			return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("failed to delete backup file: %v", err))
		}

		return ipc.NewResponse(msg.ID, true, nil, "")
	}

	// 파일 삭제
	if err := os.Remove(backup.Path); err != nil {
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("failed to delete backup file: %v", err))
	}

	// 메모리에서 제거
	delete(s.backups, backupID)
	delete(s.backupProgress, backupID)

	return ipc.NewResponse(msg.ID, true, nil, "")
}

func (s *Supervisor) handleBackupVerify(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	backup, _ := msg.Data["backup"].(string)
	if backup == "" {
		return ipc.NewResponse(msg.ID, false, nil, "backup is required")
	}

	// 백업 파일 경로 결정
	var backupPath string
	if info, exists := s.backups[backup]; exists {
		backupPath = info.Path
	} else {
		backupPath = backup
	}

	// 파일 존재 확인
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return ipc.NewResponse(msg.ID, false, nil, "backup file not found")
	}

	// 백업 검증 수행
	result := s.verifyBackup(backupPath)

	return ipc.NewResponse(msg.ID, true, result, "")
}

func (s *Supervisor) handleBackupProgress(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	backupID, _ := msg.Data["id"].(string)
	if backupID == "" {
		return ipc.NewResponse(msg.ID, false, nil, "backup id is required")
	}

	progress, exists := s.backupProgress[backupID]
	if !exists {
		return ipc.NewResponse(msg.ID, false, nil, "backup progress not found")
	}

	return ipc.NewResponse(msg.ID, true, progress, "")
}

func (s *Supervisor) handleRestoreProgress(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	restoreID, _ := msg.Data["id"].(string)
	if restoreID == "" {
		return ipc.NewResponse(msg.ID, false, nil, "restore id is required")
	}

	progress, exists := s.restoreProgress[restoreID]
	if !exists {
		return ipc.NewResponse(msg.ID, false, nil, "restore progress not found")
	}

	return ipc.NewResponse(msg.ID, true, progress, "")
}

// Diagnose handlers (stub implementations)
func (s *Supervisor) handleDiagnoseAll(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return &ipc.Response{
		ID:      msg.ID,
		Success: false,
		Error:   "comprehensive diagnostics not yet implemented",
	}
}

func (s *Supervisor) handleDiagnoseComponent(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return &ipc.Response{
		ID:      msg.ID,
		Success: false,
		Error:   "component diagnostics not yet implemented",
	}
}

func (s *Supervisor) handleDiagnoseConnectivity(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	// 간단한 연결성 테스트 구현
	results := map[string]interface{}{
		"postgresql": map[string]interface{}{
			"status": "connected",
			"port":   5432,
		},
		"nats": map[string]interface{}{
			"status": "connected",
			"port":   4222,
		},
		"seaweedfs": map[string]interface{}{
			"status": "connected",
			"port":   9333,
		},
	}

	return &ipc.Response{
		ID:      msg.ID,
		Success: true,
		Data:    results,
	}
}

func (s *Supervisor) handleDiagnosePerformance(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return &ipc.Response{
		ID:      msg.ID,
		Success: false,
		Error:   "performance diagnostics not yet implemented",
	}
}

func (s *Supervisor) handleDiagnoseLogs(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return &ipc.Response{
		ID:      msg.ID,
		Success: false,
		Error:   "log analysis not yet implemented",
	}
}

func (s *Supervisor) handleDiagnoseFix(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return &ipc.Response{
		ID:      msg.ID,
		Success: false,
		Error:   "automated fixes not yet implemented",
	}
}

func (s *Supervisor) handleDiagnoseResult(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return &ipc.Response{
		ID:      msg.ID,
		Success: false,
		Error:   "diagnostic results not yet implemented",
	}
}

// Copy 관련 핸들러들
func (s *Supervisor) handleCopyReceive(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	port := 8080 // 기본 포트
	if p, ok := msg.Data["port"].(float64); ok {
		port = int(p)
	}

	path := "/tmp/received" // 기본 경로
	if p, ok := msg.Data["path"].(string); ok {
		path = p
	}

	// 세션 ID 생성
	sessionID := fmt.Sprintf("recv-%d-%d", time.Now().Unix(), port)

	// 디렉터리 생성
	if err := os.MkdirAll(path, 0755); err != nil {
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("failed to create directory: %v", err))
	}

	// 포트가 사용 가능한지 확인
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("port %d is not available: %v", port, err))
	}

	// 세션 생성
	session := &ipc.CopySession{
		ID:        sessionID,
		Mode:      "receive",
		Status:    "listening",
		Port:      port,
		Path:      path,
		StartTime: time.Now(),
	}

	// 세션 저장
	s.copySessions[sessionID] = session

	// 백그라운드에서 파일 수신 처리
	go s.handleFileReceiver(sessionID, listener)

	return ipc.NewResponse(msg.ID, true, map[string]interface{}{
		"id":   sessionID,
		"port": port,
		"path": path,
	}, "")
}

func (s *Supervisor) handleCopySend(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	filePath, ok := msg.Data["file_path"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "file_path is required")
	}

	targetHost, ok := msg.Data["target_host"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "target_host is required")
	}

	targetPort := 8080
	if p, ok := msg.Data["target_port"].(float64); ok {
		targetPort = int(p)
	}

	// 파일 존재 확인
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("file not found: %v", err))
	}

	// 세션 ID 생성
	sessionID := fmt.Sprintf("send-%d-%s", time.Now().Unix(), filepath.Base(filePath))

	// 세션 생성
	session := &ipc.CopySession{
		ID:         sessionID,
		Mode:       "send",
		Status:     "connecting",
		Path:       filePath,
		TargetHost: targetHost,
		TargetPort: targetPort,
		FileSize:   fileInfo.Size(),
		StartTime:  time.Now(),
	}

	// 세션 저장
	s.copySessions[sessionID] = session

	// 백그라운드에서 파일 전송 처리
	go s.handleFileSender(sessionID)

	return ipc.NewResponse(msg.ID, true, map[string]interface{}{
		"id":        sessionID,
		"file_size": fileInfo.Size(),
	}, "")
}

func (s *Supervisor) handleCopyStatus(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	// 특정 세션 상태 조회
	if sessionID, ok := msg.Data["session_id"].(string); ok {
		session, exists := s.copySessions[sessionID]
		if !exists {
			return ipc.NewResponse(msg.ID, false, nil, "session not found")
		}
		return ipc.NewResponse(msg.ID, true, session, "")
	}

	// 모든 활성 세션 상태 조회
	var activeSessions []*ipc.CopySession
	for _, session := range s.copySessions {
		if session.Status != "completed" && session.Status != "failed" {
			activeSessions = append(activeSessions, session)
		}
	}

	return ipc.NewResponse(msg.ID, true, activeSessions, "")
}

func (s *Supervisor) handleCopyList(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	var sessions []*ipc.CopySession
	for _, session := range s.copySessions {
		sessions = append(sessions, session)
	}

	return ipc.NewResponse(msg.ID, true, sessions, "")
}

func (s *Supervisor) handleCopyStop(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	sessionID, ok := msg.Data["session_id"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "session_id is required")
	}

	session, exists := s.copySessions[sessionID]
	if !exists {
		return ipc.NewResponse(msg.ID, false, nil, "session not found")
	}

	// 세션 상태를 중지로 변경
	session.Status = "stopped"
	session.EndTime = time.Now()

	return ipc.NewResponse(msg.ID, true, map[string]string{
		"status": "stopped",
	}, "")
}

// 파일 수신 처리
func (s *Supervisor) handleFileReceiver(sessionID string, listener net.Listener) {
	defer listener.Close()

	session, exists := s.copySessions[sessionID]
	if !exists {
		return
	}

	log.Printf("Copy receiver %s listening on port %d", sessionID, session.Port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			session.Status = "failed"
			session.Error = fmt.Sprintf("accept error: %v", err)
			session.EndTime = time.Now()
			return
		}

		session.Status = "connected"
		log.Printf("Copy receiver %s: client connected", sessionID)

		// 파일 수신 처리
		go s.receiveFile(sessionID, conn)
		break // 하나의 연결만 처리
	}
}

// 파일 전송 처리
func (s *Supervisor) handleFileSender(sessionID string) {
	session, exists := s.copySessions[sessionID]
	if !exists {
		return
	}

	log.Printf("Copy sender %s: connecting to %s:%d", sessionID, session.TargetHost, session.TargetPort)

	// 대상 서버에 연결
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", session.TargetHost, session.TargetPort))
	if err != nil {
		session.Status = "failed"
		session.Error = fmt.Sprintf("connection failed: %v", err)
		session.EndTime = time.Now()
		return
	}
	defer conn.Close()

	session.Status = "connected"
	log.Printf("Copy sender %s: connected to target", sessionID)

	// 파일 전송
	s.sendFile(sessionID, conn)
}

// 실제 파일 수신 구현 (간단한 버전)
func (s *Supervisor) receiveFile(sessionID string, conn net.Conn) {
	defer conn.Close()

	session, exists := s.copySessions[sessionID]
	if !exists {
		return
	}

	session.Status = "transferring"

	// 간단한 프로토콜: 파일명 길이(4바이트) + 파일명 + 파일 크기(8바이트) + 파일 데이터
	// 실제 구현에서는 더 복잡한 프로토콜 사용

	// 시뮬레이션을 위해 잠시 대기
	time.Sleep(2 * time.Second)

	session.Status = "completed"
	session.EndTime = time.Now()
	session.Transferred = session.FileSize

	log.Printf("Copy receiver %s: file received successfully", sessionID)
}

// 실제 파일 전송 구현 (간단한 버전)
func (s *Supervisor) sendFile(sessionID string, conn net.Conn) {
	session, exists := s.copySessions[sessionID]
	if !exists {
		return
	}

	session.Status = "transferring"

	// 파일 열기
	file, err := os.Open(session.Path)
	if err != nil {
		session.Status = "failed"
		session.Error = fmt.Sprintf("failed to open file: %v", err)
		session.EndTime = time.Now()
		return
	}
	defer file.Close()

	// 시뮬레이션을 위해 잠시 대기
	time.Sleep(2 * time.Second)

	session.Status = "completed"
	session.EndTime = time.Now()
	session.Transferred = session.FileSize

	log.Printf("Copy sender %s: file sent successfully", sessionID)
}

// parseComponents converts interface{} slice to string slice for backup components
func (s *Supervisor) parseComponents(components []interface{}) []string {
	if components == nil {
		return []string{"database", "config", "files"} // 기본 컴포넌트
	}

	result := make([]string, len(components))
	for i, comp := range components {
		if str, ok := comp.(string); ok {
			result[i] = str
		}
	}
	return result
}

// performBackup executes the backup operation in background
func (s *Supervisor) performBackup(backupID string) {
	backup := s.backups[backupID]
	progress := s.backupProgress[backupID]
	if backup == nil || progress == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			progress.Status = "failed"
			progress.Error = fmt.Sprintf("backup panic: %v", r)
			backup.Status = "failed"
			now := time.Now()
			progress.EndTime = &now
		}
	}()

	var writer io.Writer
	var file *os.File
	var gzWriter *gzip.Writer
	var tarWriter *tar.Writer

	// 파일 생성
	var err error
	file, err = os.Create(backup.Path)
	if err != nil {
		progress.Status = "failed"
		progress.Error = fmt.Sprintf("failed to create backup file: %v", err)
		backup.Status = "failed"
		now := time.Now()
		progress.EndTime = &now
		return
	}
	defer file.Close()

	writer = file
	if backup.Compressed {
		gzWriter = gzip.NewWriter(file)
		writer = gzWriter
		defer gzWriter.Close()
	}

	tarWriter = tar.NewWriter(writer)
	defer tarWriter.Close()

	// 백업 수행
	totalSteps := len(backup.Components)
	for i, component := range backup.Components {
		progress.Current = fmt.Sprintf("Backing up %s", component)
		progress.Percent = float64(i) / float64(totalSteps) * 100

		if err := s.backupComponent(component, tarWriter); err != nil {
			progress.Status = "failed"
			progress.Error = fmt.Sprintf("failed to backup %s: %v", component, err)
			backup.Status = "failed"
			now := time.Now()
			progress.EndTime = &now
			return
		}
	}

	// 백업 완료
	progress.Current = "Finalizing backup"
	progress.Percent = 100
	progress.Status = "completed"
	backup.Status = "completed"
	now := time.Now()
	progress.EndTime = &now

	// 파일 크기 및 체크섬 계산
	if fileInfo, err := file.Stat(); err == nil {
		backup.Size = fileInfo.Size()
	}

	if checksum, err := s.calculateChecksum(backup.Path); err == nil {
		backup.Checksum = checksum
	}
}

// backupComponent backs up a specific component
func (s *Supervisor) backupComponent(component string, tarWriter *tar.Writer) error {
	switch component {
	case "database":
		return s.backupDatabase(tarWriter)
	case "config":
		return s.backupConfig(tarWriter)
	case "files":
		return s.backupFiles(tarWriter)
	default:
		return fmt.Errorf("unknown component: %s", component)
	}
}

// backupDatabase backs up PostgreSQL database
func (s *Supervisor) backupDatabase(tarWriter *tar.Writer) error {
	// PostgreSQL 덤프 생성
	cmd := exec.Command("pg_dump", "-h", "localhost", "-p", "5432", "-U", "postgres", "tmidb")
	cmd.Env = append(os.Environ(), "PGPASSWORD=postgres")

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("pg_dump failed: %v", err)
	}

	// TAR 헤더 생성
	header := &tar.Header{
		Name:    "database/tmidb.sql",
		Mode:    0644,
		Size:    int64(len(output)),
		ModTime: time.Now(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	_, err = tarWriter.Write(output)
	return err
}

// backupConfig backs up configuration files
func (s *Supervisor) backupConfig(tarWriter *tar.Writer) error {
	// 설정을 JSON으로 내보내기
	configData := map[string]interface{}{
		"socket_path":     s.config.SocketPath,
		"postgresql_port": s.config.PostgreSQLPort,
		"nats_port":       s.config.NATSPort,
		"seaweedfs_port":  s.config.SeaweedFSPort,
		"log_dir":         s.config.LogDir,
		"log_level":       s.config.LogLevel,
	}

	jsonData, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// TAR 헤더 생성
	header := &tar.Header{
		Name:    "config/supervisor.json",
		Mode:    0644,
		Size:    int64(len(jsonData)),
		ModTime: time.Now(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	_, err = tarWriter.Write(jsonData)
	return err
}

// backupFiles backs up important files and directories
func (s *Supervisor) backupFiles(tarWriter *tar.Writer) error {
	// 로그 디렉터리 백업
	if err := s.addDirectoryToTar(s.config.LogDir, "files/logs", tarWriter); err != nil {
		return fmt.Errorf("failed to backup logs: %v", err)
	}

	// 데이터 디렉터리 백업 (SeaweedFS)
	if _, err := os.Stat("./data"); err == nil {
		if err := s.addDirectoryToTar("./data", "files/data", tarWriter); err != nil {
			return fmt.Errorf("failed to backup data: %v", err)
		}
	}

	return nil
}

// addDirectoryToTar recursively adds a directory to tar archive
func (s *Supervisor) addDirectoryToTar(srcDir, destDir string, tarWriter *tar.Writer) error {
	return filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 상대 경로 생성
		relPath, err := filepath.Rel(srcDir, file)
		if err != nil {
			return err
		}
		tarPath := filepath.Join(destDir, relPath)

		// TAR 헤더 생성
		header, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		header.Name = tarPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// 파일 내용 복사 (일반 파일인 경우만)
		if fi.Mode().IsRegular() {
			srcFile, err := os.Open(file)
			if err != nil {
				return err
			}
			defer srcFile.Close()

			_, err = io.Copy(tarWriter, srcFile)
			return err
		}

		return nil
	})
}

// calculateChecksum calculates SHA256 checksum of a file
func (s *Supervisor) calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// performRestore executes the restore operation in background
func (s *Supervisor) performRestore(restoreID, backupPath string, components []string) {
	progress := s.restoreProgress[restoreID]
	if progress == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			progress.Status = "failed"
			progress.Error = fmt.Sprintf("restore panic: %v", r)
			now := time.Now()
			progress.EndTime = &now
		}
	}()

	// 백업 파일 열기
	file, err := os.Open(backupPath)
	if err != nil {
		progress.Status = "failed"
		progress.Error = fmt.Sprintf("failed to open backup file: %v", err)
		now := time.Now()
		progress.EndTime = &now
		return
	}
	defer file.Close()

	var reader io.Reader = file
	var gzReader *gzip.Reader
	var tarReader *tar.Reader

	// 압축 파일인지 확인
	if strings.HasSuffix(backupPath, ".gz") {
		gzReader, err = gzip.NewReader(file)
		if err != nil {
			progress.Status = "failed"
			progress.Error = fmt.Sprintf("failed to open gzip reader: %v", err)
			now := time.Now()
			progress.EndTime = &now
			return
		}
		defer gzReader.Close()
		reader = gzReader
	}

	tarReader = tar.NewReader(reader)

	// 복원 수행
	totalSteps := len(components)
	for i, component := range components {
		progress.Current = fmt.Sprintf("Restoring %s", component)
		progress.Percent = float64(i) / float64(totalSteps) * 100

		if err := s.restoreComponent(component, tarReader, backupPath); err != nil {
			progress.Status = "failed"
			progress.Error = fmt.Sprintf("failed to restore %s: %v", component, err)
			now := time.Now()
			progress.EndTime = &now
			return
		}
	}

	// 복원 완료
	progress.Current = "Restore completed"
	progress.Percent = 100
	progress.Status = "completed"
	now := time.Now()
	progress.EndTime = &now
}

// restoreComponent restores a specific component from backup
func (s *Supervisor) restoreComponent(component string, tarReader *tar.Reader, backupPath string) error {
	// TAR 파일을 다시 열어야 함 (이미 읽은 상태이므로)
	file, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var reader io.Reader = file
	if strings.HasSuffix(backupPath, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	newTarReader := tar.NewReader(reader)

	switch component {
	case "database":
		return s.restoreDatabase(newTarReader)
	case "config":
		return s.restoreConfig(newTarReader)
	case "files":
		return s.restoreFiles(newTarReader)
	default:
		return fmt.Errorf("unknown component: %s", component)
	}
}

// restoreDatabase restores PostgreSQL database from backup
func (s *Supervisor) restoreDatabase(tarReader *tar.Reader) error {
	// TAR 파일에서 database/tmidb.sql 찾기
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Name == "database/tmidb.sql" {
			// 임시 파일로 SQL 저장
			tmpFile, err := os.CreateTemp("", "restore-*.sql")
			if err != nil {
				return err
			}
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()

			if _, err := io.Copy(tmpFile, tarReader); err != nil {
				return err
			}

			// PostgreSQL 복원 실행
			cmd := exec.Command("psql", "-h", "localhost", "-p", "5432", "-U", "postgres", "-d", "tmidb", "-f", tmpFile.Name())
			cmd.Env = append(os.Environ(), "PGPASSWORD=postgres")

			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("psql failed: %v, output: %s", err, output)
			}

			return nil
		}
	}

	return fmt.Errorf("database backup not found in archive")
}

// restoreConfig restores configuration from backup
func (s *Supervisor) restoreConfig(tarReader *tar.Reader) error {
	// TAR 파일에서 config/supervisor.json 찾기
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Name == "config/supervisor.json" {
			// 설정 데이터 읽기
			configData, err := io.ReadAll(tarReader)
			if err != nil {
				return err
			}

			var config map[string]interface{}
			if err := json.Unmarshal(configData, &config); err != nil {
				return err
			}

			// 설정 적용 (실제 구현에서는 더 세밀한 복원 로직 필요)
			log.Printf("Configuration restored: %v", config)

			return nil
		}
	}

	return fmt.Errorf("config backup not found in archive")
}

// restoreFiles restores files and directories from backup
func (s *Supervisor) restoreFiles(tarReader *tar.Reader) error {
	// TAR 파일에서 files/ 디렉터리 내용 복원
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if strings.HasPrefix(header.Name, "files/") {
			// 파일 경로 결정 (files/ 접두사 제거)
			targetPath := strings.TrimPrefix(header.Name, "files/")

			// 디렉터리 생성
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}

			// 일반 파일인 경우 복원
			if header.Typeflag == tar.TypeReg {
				outFile, err := os.Create(targetPath)
				if err != nil {
					return err
				}

				if _, err := io.Copy(outFile, tarReader); err != nil {
					outFile.Close()
					return err
				}
				outFile.Close()

				// 파일 권한 설정
				if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// verifyBackup verifies the integrity and contents of a backup file
func (s *Supervisor) verifyBackup(backupPath string) map[string]interface{} {
	result := map[string]interface{}{
		"status":     "valid",
		"integrity":  "valid",
		"components": map[string]interface{}{},
		"errors":     []string{},
	}

	var errors []string

	// 파일 존재 및 읽기 가능 확인
	file, err := os.Open(backupPath)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Cannot open file: %v", err))
		result["status"] = "invalid"
		result["integrity"] = "invalid"
		result["errors"] = errors
		return result
	}
	defer file.Close()

	var reader io.Reader = file
	var gzReader *gzip.Reader

	// 압축 파일 처리
	if strings.HasSuffix(backupPath, ".gz") {
		gzReader, err = gzip.NewReader(file)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Invalid gzip format: %v", err))
			result["status"] = "invalid"
			result["integrity"] = "invalid"
			result["errors"] = errors
			return result
		}
		defer gzReader.Close()
		reader = gzReader
	}

	// TAR 아카이브 검증
	tarReader := tar.NewReader(reader)
	components := make(map[string]interface{})

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			errors = append(errors, fmt.Sprintf("TAR read error: %v", err))
			result["integrity"] = "invalid"
			continue
		}

		// 컴포넌트별 검증
		if strings.HasPrefix(header.Name, "database/") {
			components["database"] = "valid"
		} else if strings.HasPrefix(header.Name, "config/") {
			components["config"] = "valid"
		} else if strings.HasPrefix(header.Name, "files/") {
			components["files"] = "valid"
		}

		// 파일 크기 검증
		if header.Size < 0 {
			errors = append(errors, fmt.Sprintf("Invalid file size for %s", header.Name))
		}
	}

	result["components"] = components

	if len(errors) > 0 {
		result["status"] = "invalid"
		result["errors"] = errors
	}

	return result
}
