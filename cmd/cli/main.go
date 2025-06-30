package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmidb/tmidb-core/internal/ipc"
)

// Global IPC client
var client *ipc.Client

var rootCmd = &cobra.Command{
	Use:   "tmidb-cli",
	Short: "tmiDB CLI tool for managing tmiDB-Core components",
	Long: `tmiDB CLI is a command-line tool for managing and monitoring 
tmiDB-Core components including logging, process control, and system monitoring.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// IPC 클라이언트 초기화
		socketPath := os.Getenv("TMIDB_SOCKET_PATH")
		client = ipc.NewClient(socketPath)
		if err := client.Connect(); err != nil {
			fmt.Printf("❌ Failed to connect to supervisor: %v\n", err)
			fmt.Println("💡 Make sure tmidb-supervisor is running")
			os.Exit(1)
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// IPC 클라이언트 정리
		if client != nil {
			client.Close()
		}
	},
}

// 모니터링 관련 명령어들
var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor system resources and health",
	Long:  "Monitor system resources, service health, and performance",
}

var monitorSystemCmd = &cobra.Command{
	Use:   "system",
	Short: "Monitor system resources",
	Long:  "Display real-time system resource usage",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📊 System Resource Monitor (Press Ctrl+C to stop)")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// 신호 처리
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		// 초기 헤더 출력
		fmt.Printf("%-20s %-15s %-15s %-15s %-15s %-15s\n", 
			"TIME", "PROCESSES", "CPU", "MEMORY", "DISK", "IPC CONN")
		fmt.Println("────────────────────────────────────────────────────────────────────────────────────────")

		for {
			select {
			case <-ticker.C:
				resp, err := client.SendMessage(ipc.MessageTypeSystemStats, nil)
				if err != nil {
					fmt.Printf("❌ Failed to get system stats: %v\n", err)
					continue
				}

				if !resp.Success {
					fmt.Printf("❌ Error: %s\n", resp.Error)
					continue
				}

				// 통계 출력
				if stats, ok := resp.Data.(map[string]interface{}); ok {
					currentTime := time.Now().Format("15:04:05")
					
					// 프로세스 정보
					processes := getIntValue(stats, "processes")
					running := getIntValue(stats, "running")
					stopped := getIntValue(stats, "stopped")
					errors := getIntValue(stats, "errors")
					processInfo := fmt.Sprintf("%d (%d↑ %d↓ %d⚠)", processes, running, stopped, errors)
					
					// 리소스 정보
					cpuUsage := getFloatValue(stats, "cpu_usage")
					memoryUsage := getFloatValue(stats, "memory_usage")
					diskUsage := getFloatValue(stats, "disk_usage")
					ipcConn := getIntValue(stats, "ipc_connections")
					
					cpuInfo := fmt.Sprintf("%.1f%%", cpuUsage)
					memInfo := fmt.Sprintf("%.1f%%", memoryUsage)
					diskInfo := fmt.Sprintf("%.1f%%", diskUsage)
					ipcInfo := fmt.Sprintf("%d", ipcConn)
					
					fmt.Printf("%-20s %-15s %-15s %-15s %-15s %-15s\n",
						currentTime, processInfo, cpuInfo, memInfo, diskInfo, ipcInfo)
				}
			case <-sigChan:
				fmt.Println("\n📊 System monitoring stopped")
				return
			}
		}
	},
}

var monitorServicesCmd = &cobra.Command{
	Use:   "services",
	Short: "Monitor service health",
	Long:  "Display health status of all services",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := client.SendMessage(ipc.MessageTypeSystemHealth, nil)
		if err != nil {
			fmt.Printf("❌ Failed to get system health: %v\n", err)
			os.Exit(1)
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			os.Exit(1)
		}

		// JSON을 SystemHealth로 변환
		healthData, _ := json.Marshal(resp.Data)
		var health ipc.SystemHealth
		if err := json.Unmarshal(healthData, &health); err != nil {
			fmt.Printf("❌ Failed to parse health data: %v\n", err)
			os.Exit(1)
		}

		// 출력 포맷터 가져오기
		formatter := getFormatter(cmd)
		
		// JSON/YAML 출력인 경우
		if format, _ := cmd.Flags().GetString("output"); format == "json" || format == "json-pretty" || format == "yaml" {
			if err := formatter.Print(health); err != nil {
				fmt.Printf("❌ Failed to format output: %v\n", err)
				os.Exit(1)
			}
			return
		}

		// 기본 텍스트 출력
		fmt.Println("🏥 Service Health Monitor:")
		fmt.Printf("Overall Status: %s\n", health.Status)
		fmt.Printf("Uptime: %s\n", formatDuration(health.Uptime))
		fmt.Printf("Last Check: %s\n", health.LastCheck.Format("2006-01-02 15:04:05"))

		fmt.Println("\nComponent Status:")
		for component, status := range health.Components {
			statusIcon := "✅"
			if status != "running" {
				statusIcon = "❌"
			}
			fmt.Printf("  %s %-20s: %s\n", statusIcon, component, status)
		}

		if len(health.Errors) > 0 {
			fmt.Println("\nErrors:")
			for _, err := range health.Errors {
				fmt.Printf("  ❌ %s\n", err)
			}
		}
	},
}

var monitorHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check overall system health",
	Long:  "Perform a quick health check of all components",
	Run: func(cmd *cobra.Command, args []string) {
		if err := client.Ping(); err != nil {
			fmt.Printf("❌ Supervisor is not responding: %v\n", err)
			os.Exit(1)
		}

		// 프로세스 상태 확인
		processes, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process status: %v\n", err)
			os.Exit(1)
		}

		healthy := 0
		total := len(processes)

		for _, process := range processes {
			if process.Status == "running" {
				healthy++
			}
		}

		healthSummary := map[string]interface{}{
			"supervisor_status": "healthy",
			"total_components":  total,
			"healthy_components": healthy,
			"unhealthy_components": total - healthy,
			"health_percentage": float64(healthy) / float64(total) * 100,
			"components": processes,
		}

		// 출력 포맷터 가져오기
		formatter := getFormatter(cmd)
		
		// JSON/YAML 출력인 경우
		if format, _ := cmd.Flags().GetString("output"); format == "json" || format == "json-pretty" || format == "yaml" {
			if err := formatter.Print(healthSummary); err != nil {
				fmt.Printf("❌ Failed to format output: %v\n", err)
				os.Exit(1)
			}
			return
		}

		// 기본 텍스트 출력
		fmt.Println("🏥 Performing health check...")
		fmt.Println("✅ Supervisor is healthy")
		fmt.Printf("📊 System Health: %d/%d components running\n", healthy, total)

		if healthy == total {
			fmt.Println("✅ All components are healthy")
		} else {
			fmt.Printf("⚠️ %d components need attention\n", total-healthy)
		}
	},
}

// 시스템 상태 명령어
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all tmiDB components",
	Long:  "Display status, uptime, and resource usage for all tmiDB components",
	Run: func(cmd *cobra.Command, args []string) {
		processes, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process list: %v\n", err)
			os.Exit(1)
		}

		// 기본 컴포넌트 목록 (실제 프로세스가 없어도 표시)
		components := []string{"api", "data-manager", "data-consumer", "postgresql", "nats", "seaweedfs"}
		processMap := make(map[string]*ipc.ProcessInfo)

		// 실제 프로세스 정보를 맵에 저장
		for i := range processes {
			processMap[processes[i].Name] = &processes[i]
		}

		// 출력 포맷터 가져오기
		formatter := getFormatter(cmd)
		
		// JSON/YAML 출력인 경우 구조화된 데이터 출력
		if format, _ := cmd.Flags().GetString("output"); format == "json" || format == "json-pretty" || format == "yaml" {
			statusData := make(map[string]interface{})
			for _, component := range components {
				if process, exists := processMap[component]; exists {
					statusData[component] = map[string]interface{}{
						"status":     process.Status,
						"pid":        process.PID,
						"uptime":     process.Uptime.String(),
						"memory":     process.Memory,
						"cpu":        process.CPU,
						"start_time": process.StartTime,
					}
				} else {
					statusData[component] = map[string]interface{}{
						"status": "not found",
						"pid":    0,
						"uptime": "0s",
						"memory": 0,
						"cpu":    0.0,
					}
				}
			}
			if err := formatter.Print(statusData); err != nil {
				fmt.Printf("❌ Failed to format output: %v\n", err)
				os.Exit(1)
			}
			return
		}

		// 기본 텍스트 출력
		fmt.Println("📊 tmiDB-Core Component Status:")
		for _, component := range components {
			fmt.Printf("🔍 %s:\n", component)

			if process, exists := processMap[component]; exists {
				// 실제 프로세스 정보 표시
				fmt.Printf("  Status: %s\n", process.Status)
				fmt.Printf("  PID: %d\n", process.PID)
				fmt.Printf("  Uptime: %s\n", formatDuration(process.Uptime))
				fmt.Printf("  Memory: %s\n", formatBytes(process.Memory))
				fmt.Printf("  CPU: %.1f%%\n", process.CPU)
			} else {
				// 컴포넌트가 없는 경우
				fmt.Printf("  Status: not found\n")
				fmt.Printf("  PID: -\n")
				fmt.Printf("  Uptime: -\n")
				fmt.Printf("  Memory: -\n")
				fmt.Printf("  CPU: -\n")
			}
			fmt.Println()
		}
	},
}

// Service 권한 관리 명령어
var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage service permissions and control",
	Long:  "Control service start/stop/restart permissions and log access",
}

var serviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all services and their permissions",
	Long:  "Display all services with their current status and permission settings",
	Run: func(cmd *cobra.Command, args []string) {
		processes, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process list: %v\n", err)
			os.Exit(1)
		}

		// 출력 포맷터 가져오기
		formatter := getFormatter(cmd)
		
		// JSON/YAML 출력인 경우
		if format, _ := cmd.Flags().GetString("output"); format == "json" || format == "json-pretty" || format == "yaml" {
			serviceData := make(map[string]interface{})
			for _, proc := range processes {
				serviceData[proc.Name] = map[string]interface{}{
					"status":       proc.Status,
					"pid":          proc.PID,
					"type":         getServiceType(proc.Name),
					"permissions": map[string]bool{
						"start":   true,
						"stop":    true,
						"restart": true,
						"logs":    true,
					},
					"uptime":     proc.Uptime.String(),
					"memory":     proc.Memory,
					"cpu":        proc.CPU,
					"start_time": proc.StartTime,
				}
			}
			if err := formatter.Print(serviceData); err != nil {
				fmt.Printf("❌ Failed to format output: %v\n", err)
				os.Exit(1)
			}
			return
		}

		// 기본 텍스트 출력
		fmt.Println("🔐 Service Permissions and Status:")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("%-15s %-10s %-8s %-10s %-12s %-10s %-10s\n", 
			"SERVICE", "STATUS", "PID", "TYPE", "PERMISSIONS", "UPTIME", "MEMORY")
		fmt.Println("────────────────────────────────────────────────────────────────────────────────────────")

		for _, proc := range processes {
			statusIcon := getStatusIcon(proc.Status)
			serviceType := getServiceType(proc.Name)
			permissions := "START|STOP|RESTART|LOGS"
			uptime := formatDuration(proc.Uptime)
			memory := formatBytes(proc.Memory)

			fmt.Printf("%-15s %s%-8s %-8d %-10s %-12s %-10s %-10s\n",
				proc.Name, statusIcon, proc.Status, proc.PID, serviceType, permissions, uptime, memory)
		}
	},
}

var serviceControlCmd = &cobra.Command{
	Use:   "control [start|stop|restart] [service-name]",
	Short: "Control service lifecycle",
	Long:  "Start, stop, or restart a specific service",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		action := args[0]
		serviceName := args[1]

		switch action {
		case "start":
			err := startService(serviceName)
			if err != nil {
				fmt.Printf("❌ Failed to start service %s: %v\n", serviceName, err)
				os.Exit(1)
			}
			fmt.Printf("✅ Service %s started successfully\n", serviceName)

		case "stop":
			err := stopService(serviceName)
			if err != nil {
				fmt.Printf("❌ Failed to stop service %s: %v\n", serviceName, err)
				os.Exit(1)
			}
			fmt.Printf("✅ Service %s stopped successfully\n", serviceName)

		case "restart":
			err := restartService(serviceName)
			if err != nil {
				fmt.Printf("❌ Failed to restart service %s: %v\n", serviceName, err)
				os.Exit(1)
			}
			fmt.Printf("✅ Service %s restarted successfully\n", serviceName)

		default:
			fmt.Printf("❌ Invalid action: %s. Use start, stop, or restart\n", action)
			os.Exit(1)
		}
	},
}

var serviceLogsCmd = &cobra.Command{
	Use:   "logs [service-name]",
	Short: "View service logs",
	Long:  "Display logs for a specific service",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		serviceName := args[0]
		lines, _ := cmd.Flags().GetInt("lines")
		follow, _ := cmd.Flags().GetBool("follow")

		if follow {
			fmt.Printf("📜 Following logs for %s (Press Ctrl+C to stop):\n", serviceName)
			// 실시간 로그 스트리밍 구현
			if err := streamServiceLogs(serviceName); err != nil {
				fmt.Printf("❌ Failed to stream logs: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Printf("📜 Recent logs for %s:\n", serviceName)
			if err := getServiceLogs(serviceName, lines); err != nil {
				fmt.Printf("❌ Failed to get logs: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

// 유틸리티 함수들
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}

func formatBytes(bytes int64) string {
	if bytes == 0 {
		return "0B"
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f%s", float64(bytes)/float64(div), units[exp])
}

// 헬퍼 함수들
func getIntValue(data map[string]interface{}, key string) int {
	if val, ok := data[key]; ok {
		if intVal, ok := val.(float64); ok {
			return int(intVal)
		}
		if intVal, ok := val.(int); ok {
			return intVal
		}
	}
	return 0
}

func getFloatValue(data map[string]interface{}, key string) float64 {
	if val, ok := data[key]; ok {
		if floatVal, ok := val.(float64); ok {
			return floatVal
		}
		if intVal, ok := val.(int); ok {
			return float64(intVal)
		}
	}
	return 0.0
}

func getServiceType(serviceName string) string {
	switch serviceName {
	case "postgresql", "nats", "seaweedfs":
		return "external"
	case "api", "data-manager", "data-consumer":
		return "internal"
	default:
		return "unknown"
	}
}

func getStatusIcon(status string) string {
	switch status {
	case "running":
		return "🟢 "
	case "stopped":
		return "🔴 "
	case "error":
		return "🟡 "
	default:
		return "⚪ "
	}
}

func startService(serviceName string) error {
	data := map[string]interface{}{"name": serviceName}
	resp, err := client.SendMessage(ipc.MessageTypeProcessStart, data)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Error)
	}
	return nil
}

func stopService(serviceName string) error {
	data := map[string]interface{}{"name": serviceName}
	resp, err := client.SendMessage(ipc.MessageTypeProcessStop, data)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Error)
	}
	return nil
}

func restartService(serviceName string) error {
	data := map[string]interface{}{"name": serviceName}
	resp, err := client.SendMessage(ipc.MessageTypeProcessRestart, data)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Error)
	}
	return nil
}

func getServiceLogs(serviceName string, lines int) error {
	data := map[string]interface{}{
		"component": serviceName,
		"lines":     lines,
	}
	resp, err := client.SendMessage(ipc.MessageTypeGetLogs, data)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Error)
	}

	// 로그 출력
	if logs, ok := resp.Data.([]interface{}); ok {
		for _, logEntry := range logs {
			if logMap, ok := logEntry.(map[string]interface{}); ok {
				timestamp := logMap["timestamp"]
				message := logMap["message"]
				fmt.Printf("[%v] %v\n", timestamp, message)
			}
		}
	}
	return nil
}

func streamServiceLogs(serviceName string) error {
	// 실시간 로그 스트리밍 구현
	data := map[string]interface{}{
		"component": serviceName,
		"action":    "start",
	}
	resp, err := client.SendMessage(ipc.MessageTypeLogStream, data)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Error)
	}

	// 실제 스트리밍은 IPC 연결을 통해 구현해야 함
	fmt.Println("Log streaming started (simplified implementation)")
	return nil
}

func init() {
	// 모든 명령어에 output 플래그 추가
	addOutputFlag := func(cmd *cobra.Command) {
		cmd.Flags().StringP("output", "o", "default", "Output format (default, json, json-pretty, yaml)")
	}

	// 모니터링 명령어에 플래그 추가
	addOutputFlag(monitorSystemCmd)
	addOutputFlag(monitorServicesCmd)
	addOutputFlag(monitorHealthCmd)
	addOutputFlag(statusCmd)
	addOutputFlag(serviceListCmd)

	// Service logs 명령어에 플래그 추가
	serviceLogsCmd.Flags().IntP("lines", "n", 50, "Number of lines to show")
	serviceLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output")

	// Service 명령어 구성
	serviceCmd.AddCommand(serviceListCmd)
	serviceCmd.AddCommand(serviceControlCmd)
	serviceCmd.AddCommand(serviceLogsCmd)

	// 모니터링 명령어 구성
	monitorCmd.AddCommand(monitorSystemCmd)
	monitorCmd.AddCommand(monitorServicesCmd)
	monitorCmd.AddCommand(monitorHealthCmd)

	// 루트 명령어에 추가
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(monitorCmd)
	rootCmd.AddCommand(serviceCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
}
