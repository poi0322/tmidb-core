package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/tmidb/tmidb-core/internal/ipc"
)

// MockSupervisor는 테스트용 Supervisor입니다
type MockSupervisor struct {
	server     *ipc.Server
	logEnabled map[string]bool
	processes  []ipc.ProcessInfo
}

// NewMockSupervisor creates a new mock supervisor for testing
func NewMockSupervisor(socketPath string) *MockSupervisor {
	if socketPath == "" {
		socketPath = "/tmp/tmidb-test-supervisor.sock"
	}

	return &MockSupervisor{
		server: ipc.NewServer(socketPath),
		logEnabled: map[string]bool{
			"api":           true,
			"data-manager":  true,
			"data-consumer": true,
			"postgresql":    true,
			"nats":          true,
			"seaweedfs":     true,
		},
		processes: []ipc.ProcessInfo{
			{
				Name:      "api",
				Status:    "running",
				PID:       12345,
				Uptime:    2 * time.Hour,
				Memory:    50 * 1024 * 1024,
				CPU:       12.5,
				Enabled:   true,
				StartTime: time.Now().Add(-2 * time.Hour),
			},
			{
				Name:      "data-manager",
				Status:    "running",
				PID:       12346,
				Uptime:    2 * time.Hour,
				Memory:    30 * 1024 * 1024,
				CPU:       8.3,
				Enabled:   true,
				StartTime: time.Now().Add(-2 * time.Hour),
			},
			{
				Name:      "data-consumer",
				Status:    "stopped",
				PID:       0,
				Uptime:    0,
				Memory:    0,
				CPU:       0,
				Enabled:   false,
				StartTime: time.Time{},
			},
		},
	}
}

// Start starts the mock supervisor
func (m *MockSupervisor) Start() error {
	// Register handlers
	m.server.RegisterHandler(ipc.MessageTypeLogEnable, m.handleLogEnable)
	m.server.RegisterHandler(ipc.MessageTypeLogDisable, m.handleLogDisable)
	m.server.RegisterHandler(ipc.MessageTypeLogStatus, m.handleLogStatus)
	m.server.RegisterHandler(ipc.MessageTypeGetLogs, m.handleGetLogs)
	m.server.RegisterHandler(ipc.MessageTypeProcessList, m.handleProcessList)
	m.server.RegisterHandler(ipc.MessageTypeProcessStatus, m.handleProcessStatus)
	m.server.RegisterHandler(ipc.MessageTypeProcessStart, m.handleProcessStart)
	m.server.RegisterHandler(ipc.MessageTypeProcessStop, m.handleProcessStop)
	m.server.RegisterHandler(ipc.MessageTypeProcessRestart, m.handleProcessRestart)
	m.server.RegisterHandler(ipc.MessageTypeSystemHealth, m.handleSystemHealth)
	m.server.RegisterHandler(ipc.MessageTypeSystemStats, m.handleSystemStats)

	return m.server.Start()
}

// Stop stops the mock supervisor
func (m *MockSupervisor) Stop() error {
	return m.server.Stop()
}

// Handler implementations
func (m *MockSupervisor) handleLogEnable(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	if _, exists := m.logEnabled[component]; !exists {
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("unknown component: %s", component))
	}

	m.logEnabled[component] = true
	return ipc.NewResponse(msg.ID, true, nil, "")
}

func (m *MockSupervisor) handleLogDisable(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	if _, exists := m.logEnabled[component]; !exists {
		return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("unknown component: %s", component))
	}

	m.logEnabled[component] = false
	return ipc.NewResponse(msg.ID, true, nil, "")
}

func (m *MockSupervisor) handleLogStatus(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return ipc.NewResponse(msg.ID, true, m.logEnabled, "")
}

func (m *MockSupervisor) handleGetLogs(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component := "all"
	if comp, ok := msg.Data["component"].(string); ok {
		component = comp
	}

	lines := 50
	if l, ok := msg.Data["lines"].(float64); ok {
		lines = int(l)
	}

	// Mock log entries
	logs := []map[string]interface{}{}
	for i := 0; i < lines && i < 10; i++ {
		logs = append(logs, map[string]interface{}{
			"timestamp": time.Now().Add(-time.Duration(i) * time.Minute).Format("15:04:05"),
			"process":   component,
			"message":   fmt.Sprintf("Mock log entry %d for %s", i+1, component),
		})
	}

	return ipc.NewResponse(msg.ID, true, logs, "")
}

func (m *MockSupervisor) handleProcessList(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	return ipc.NewResponse(msg.ID, true, m.processes, "")
}

func (m *MockSupervisor) handleProcessStatus(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	for _, proc := range m.processes {
		if proc.Name == component {
			return ipc.NewResponse(msg.ID, true, proc, "")
		}
	}

	return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("component %s not found", component))
}

func (m *MockSupervisor) handleProcessStart(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	for i, proc := range m.processes {
		if proc.Name == component {
			if proc.Status == "running" {
				return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("%s is already running", component))
			}
			m.processes[i].Status = "running"
			m.processes[i].PID = 10000 + i
			m.processes[i].StartTime = time.Now()
			m.processes[i].Enabled = true
			return ipc.NewResponse(msg.ID, true, nil, "")
		}
	}

	return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("component %s not found", component))
}

func (m *MockSupervisor) handleProcessStop(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	for i, proc := range m.processes {
		if proc.Name == component {
			if proc.Status == "stopped" {
				return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("%s is already stopped", component))
			}
			m.processes[i].Status = "stopped"
			m.processes[i].PID = 0
			m.processes[i].Uptime = 0
			m.processes[i].Enabled = false
			return ipc.NewResponse(msg.ID, true, nil, "")
		}
	}

	return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("component %s not found", component))
}

func (m *MockSupervisor) handleProcessRestart(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component name required")
	}

	for i, proc := range m.processes {
		if proc.Name == component {
			m.processes[i].Status = "running"
			m.processes[i].PID = 20000 + i
			m.processes[i].StartTime = time.Now()
			m.processes[i].Uptime = 0
			m.processes[i].Enabled = true
			return ipc.NewResponse(msg.ID, true, nil, "")
		}
	}

	return ipc.NewResponse(msg.ID, false, nil, fmt.Sprintf("component %s not found", component))
}

func (m *MockSupervisor) handleSystemHealth(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	components := make(map[string]string)
	for _, proc := range m.processes {
		components[proc.Name] = proc.Status
	}

	health := ipc.SystemHealth{
		Status:     "healthy",
		Uptime:     2 * time.Hour,
		Components: components,
		Resources: ipc.SystemResources{
			CPUUsage:    25.5,
			MemoryUsage: 45.2,
			DiskUsage:   60.0,
			NetworkIO:   1024 * 1024,
			DiskIO:      512 * 1024,
		},
		LastCheck: time.Now(),
		Errors:    []string{},
	}

	// Convert to map for JSON marshaling
	healthData, _ := json.Marshal(health)
	var healthMap map[string]interface{}
	json.Unmarshal(healthData, &healthMap)

	return ipc.NewResponse(msg.ID, true, healthMap, "")
}

func (m *MockSupervisor) handleSystemStats(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	stats := map[string]interface{}{
		"processes":       len(m.processes),
		"running":         2,
		"stopped":         1,
		"errors":          0,
		"ipc_connections": m.server.GetConnectionCount(),
	}

	return ipc.NewResponse(msg.ID, true, stats, "")
}

// Test function to run mock supervisor
func TestMockSupervisor(t *testing.T) {
	// Skip if not explicitly requested
	if testing.Short() {
		t.Skip("Skipping mock supervisor test in short mode")
	}

	supervisor := NewMockSupervisor("/tmp/tmidb-test-supervisor.sock")

	if err := supervisor.Start(); err != nil {
		t.Fatalf("Failed to start mock supervisor: %v", err)
	}
	defer supervisor.Stop()

	log.Println("Mock supervisor started on /tmp/tmidb-test-supervisor.sock")
	log.Println("Run CLI tests against this socket...")

	// Keep running for manual testing
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	<-ctx.Done()
}
