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

		// 신호 처리
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

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
					fmt.Printf("\r\033[K📊 Processes: %v | Running: %v | Stopped: %v | Errors: %v | IPC Connections: %v",
						stats["processes"], stats["running"], stats["stopped"], stats["errors"], stats["ipc_connections"])
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
		fmt.Println("🏥 Service Health Monitor:")

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
		fmt.Println("🏥 Performing health check...")

		if err := client.Ping(); err != nil {
			fmt.Printf("❌ Supervisor is not responding: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ Supervisor is healthy")

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
		fmt.Println("📊 tmiDB-Core Component Status:")

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

		// 각 컴포넌트 상태 표시
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

func init() {
	// 모니터링 명령어 구성
	monitorCmd.AddCommand(monitorSystemCmd)
	monitorCmd.AddCommand(monitorServicesCmd)
	monitorCmd.AddCommand(monitorHealthCmd)

	// 루트 명령어에 추가
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(monitorCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
}
