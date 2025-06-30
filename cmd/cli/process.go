package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmidb/tmidb-core/internal/ipc"
)

// 프로세스 관련 명령어들
var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Manage tmiDB processes",
	Long:  "Start, stop, restart, and monitor tmiDB processes",
}

var processListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tmiDB processes",
	Long:  "Display all running tmiDB processes with their status",
	Run: func(cmd *cobra.Command, args []string) {
		processes, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process list: %v\n", err)
			os.Exit(1)
		}

		// 출력 형식 확인
		formatter := getFormatter(cmd)
		
		// JSON 출력인 경우
		if formatter.format == "json" || formatter.format == "json-pretty" {
			// ProcessInfo를 JSON 호환 형식으로 변환
			var processData []interface{}
			for _, process := range processes {
				processMap := map[string]interface{}{
					"name":       process.Name,
					"status":     process.Status,
					"pid":        process.PID,
					"uptime":     process.Uptime.Nanoseconds(),
					"memory":     process.Memory,
					"cpu":        process.CPU,
					"enabled":    process.Enabled,
					"start_time": process.StartTime.Format("2006-01-02T15:04:05Z07:00"),
				}
				processData = append(processData, processMap)
			}
			
			formatted := FormatProcessList(processData)
			formatter.Print(formatted)
			return
		}

		// 기본 텍스트 출력
		fmt.Println("📋 tmiDB Processes:")
		fmt.Printf("%-20s %-12s %-8s %-12s %-10s %-8s\n",
			"NAME", "STATUS", "PID", "UPTIME", "MEMORY", "CPU")
		fmt.Println(strings.Repeat("-", 80))

		for _, process := range processes {
			uptime := formatDuration(process.Uptime)
			memory := formatBytes(process.Memory)

			fmt.Printf("%-20s %-12s %-8d %-12s %-10s %.1f%%\n",
				process.Name,
				process.Status,
				process.PID,
				uptime,
				memory,
				process.CPU)
		}
	},
}

var processStatusCmd = &cobra.Command{
	Use:   "status [component]",
	Short: "Show status of a specific component",
	Long:  "Display detailed status information for a specific component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🔍 Status for component: %s\n", component)

		processes, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process list: %v\n", err)
			os.Exit(1)
		}

		var found *ipc.ProcessInfo
		for _, process := range processes {
			if process.Name == component {
				found = &process
				break
			}
		}

		if found == nil {
			fmt.Printf("❌ Component %s not found\n", component)
			os.Exit(1)
		}

		fmt.Printf("  Status: %s\n", found.Status)
		fmt.Printf("  PID: %d\n", found.PID)
		fmt.Printf("  Uptime: %s\n", formatDuration(found.Uptime))
		fmt.Printf("  Memory: %s\n", formatBytes(found.Memory))
		fmt.Printf("  CPU: %.1f%%\n", found.CPU)
		fmt.Printf("  Auto Restart: %t\n", found.Enabled)
		fmt.Printf("  Start Time: %s\n", found.StartTime.Format("2006-01-02 15:04:05"))
	},
}

var processRestartCmd = &cobra.Command{
	Use:   "restart [component]",
	Short: "Restart a specific component",
	Long:  "Stop and start a specific component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🔄 Restarting component: %s\n", component)

		if err := client.RestartProcess(component); err != nil {
			fmt.Printf("❌ Failed to restart %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Component %s restarted successfully\n", component)
	},
}

var processStopCmd = &cobra.Command{
	Use:   "stop [component]",
	Short: "Stop a specific component",
	Long:  "Stop a running component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🛑 Stopping component: %s\n", component)

		if err := client.StopProcess(component); err != nil {
			fmt.Printf("❌ Failed to stop %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Component %s stopped successfully\n", component)
	},
}

var processStartCmd = &cobra.Command{
	Use:   "start [component]",
	Short: "Start a specific component",
	Long:  "Start a stopped component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🚀 Starting component: %s\n", component)

		if err := client.StartProcess(component); err != nil {
			fmt.Printf("❌ Failed to start %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Component %s started successfully\n", component)
	},
}

// --- From process_groups.go ---

// 프로세스 그룹 정의
var processGroups = map[string][]string{
	"core": {"postgresql", "nats", "seaweedfs"},
	"app":  {"api", "data-manager", "data-consumer"},
	"data": {"data-manager", "data-consumer"},
	"all":  {"postgresql", "nats", "seaweedfs", "api", "data-manager", "data-consumer"},
}

// 프로세스 의존성 정의 (프로세스: 의존하는 프로세스들)
var processDependencies = map[string][]string{
	"api":           {"postgresql", "nats"},
	"data-manager":  {"postgresql", "nats", "seaweedfs"},
	"data-consumer": {"nats"},
}

// 프로세스 그룹 명령어
var processGroupCmd = &cobra.Command{
	Use:   "group",
	Short: "Manage process groups",
	Long:  "Start, stop, or restart groups of related processes",
}

var processGroupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available process groups",
	Long:  "Display all defined process groups and their components",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📋 Process Groups:")
		fmt.Println()

		for group, processes := range processGroups {
			fmt.Printf("🔸 %s:\n", group)
			for _, proc := range processes {
				// 현재 상태 확인
				status := getProcessStatus(proc)
				statusIcon := "❌"
				if status == "running" {
					statusIcon = "✅"
				}
				fmt.Printf("   %s %s\n", statusIcon, proc)
			}
			fmt.Println()
		}
	},
}

var processGroupStartCmd = &cobra.Command{
	Use:   "start <group>",
	Short: "Start all processes in a group",
	Long:  "Start all processes in a group respecting dependencies",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		group := args[0]
		processes, exists := processGroups[group]
		if !exists {
			fmt.Printf("❌ Unknown process group: %s\n", group)
			fmt.Println("Available groups: core, app, data, all")
			return
		}

		fmt.Printf("🚀 Starting process group: %s\n", group)

		// 의존성 순서대로 정렬
		sortedProcesses := sortByDependencies(processes)

		// 순차적으로 시작
		for _, proc := range sortedProcesses {
			fmt.Printf("  Starting %s...", proc)

			if err := client.StartProcess(proc); err != nil {
				if strings.Contains(err.Error(), "already running") {
					fmt.Printf(" ⚠️  Already running\n")
				} else {
					fmt.Printf(" ❌ Failed: %v\n", err)
				}
			} else {
				fmt.Printf(" ✅ Started\n")
				// 프로세스가 완전히 시작될 시간을 줌
				time.Sleep(2 * time.Second)
			}
		}

		fmt.Println("\n✅ Process group start completed")
	},
}

var processGroupStopCmd = &cobra.Command{
	Use:   "stop <group>",
	Short: "Stop all processes in a group",
	Long:  "Stop all processes in a group in reverse dependency order",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		group := args[0]
		processes, exists := processGroups[group]
		if !exists {
			fmt.Printf("❌ Unknown process group: %s\n", group)
			fmt.Println("Available groups: core, app, data, all")
			return
		}

		fmt.Printf("🛑 Stopping process group: %s\n", group)

		// 의존성 역순으로 정렬
		sortedProcesses := sortByDependencies(processes)
		// 역순으로 변경
		for i, j := 0, len(sortedProcesses)-1; i < j; i, j = i+1, j-1 {
			sortedProcesses[i], sortedProcesses[j] = sortedProcesses[j], sortedProcesses[i]
		}

		// 순차적으로 중지
		for _, proc := range sortedProcesses {
			fmt.Printf("  Stopping %s...", proc)

			if err := client.StopProcess(proc); err != nil {
				if strings.Contains(err.Error(), "already stopped") {
					fmt.Printf(" ⚠️  Already stopped\n")
				} else {
					fmt.Printf(" ❌ Failed: %v\n", err)
				}
			} else {
				fmt.Printf(" ✅ Stopped\n")
			}
		}

		fmt.Println("\n✅ Process group stop completed")
	},
}

var processGroupRestartCmd = &cobra.Command{
	Use:   "restart <group>",
	Short: "Restart all processes in a group",
	Long:  "Stop and start all processes in a group respecting dependencies",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		group := args[0]
		_, exists := processGroups[group]
		if !exists {
			fmt.Printf("❌ Unknown process group: %s\n", group)
			fmt.Println("Available groups: core, app, data, all")
			return
		}

		fmt.Printf("🔄 Restarting process group: %s\n", group)

		// 먼저 중지
		fmt.Println("\n📌 Phase 1: Stopping processes...")
		processGroupStopCmd.Run(cmd, args)

		// 잠시 대기
		fmt.Println("\n⏳ Waiting for processes to fully stop...")
		time.Sleep(3 * time.Second)

		// 다시 시작
		fmt.Println("\n📌 Phase 2: Starting processes...")
		processGroupStartCmd.Run(cmd, args)
	},
}

var processGroupStatusCmd = &cobra.Command{
	Use:   "status <group>",
	Short: "Show status of all processes in a group",
	Long:  "Display detailed status for all processes in a specific group",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		group := args[0]
		processes, exists := processGroups[group]
		if !exists {
			fmt.Printf("❌ Unknown process group: %s\n", group)
			fmt.Println("Available groups: core, app, data, all")
			return
		}

		fmt.Printf("📊 Status for process group: %s\n", group)
		fmt.Println()

		// 프로세스 목록 가져오기
		processList, err := client.GetProcessList()
		if err != nil {
			fmt.Printf("❌ Failed to get process list: %v\n", err)
			return
		}

		// 프로세스 맵 생성
		processMap := make(map[string]*ipc.ProcessInfo)
		for i := range processList {
			processMap[processList[i].Name] = &processList[i]
		}

		// 그룹의 각 프로세스 상태 표시
		runningCount := 0
		totalCount := len(processes)

		for _, procName := range processes {
			if proc, exists := processMap[procName]; exists && proc.Status == "running" {
				runningCount++
			}
		}

		// 요약 상태
		healthStatus := "❌ Unhealthy"
		if runningCount == totalCount {
			healthStatus = "✅ Healthy"
		} else if runningCount > 0 {
			healthStatus = "⚠️  Partial"
		}

		fmt.Printf("Group Health: %s (%d/%d running)\n", healthStatus, runningCount, totalCount)
		fmt.Println("\nProcess Details:")
		fmt.Printf("%-20s %-12s %-8s %-12s %-10s\n", "NAME", "STATUS", "PID", "UPTIME", "MEMORY")
		fmt.Println(strings.Repeat("-", 60))

		for _, procName := range processes {
			if proc, exists := processMap[procName]; exists {
				fmt.Printf("%-20s %-12s %-8d %-12s %-10s\n",
					proc.Name,
					proc.Status,
					proc.PID,
					formatDuration(proc.Uptime),
					formatBytes(proc.Memory))
			} else {
				fmt.Printf("%-20s %-12s %-8s %-12s %-10s\n",
					procName, "not found", "-", "-", "-")
			}
		}
	},
}

// 프로세스를 의존성 순서대로 정렬
func sortByDependencies(processes []string) []string {
	// 간단한 구현: core 서비스를 먼저, 그 다음 app 서비스
	coreServices := []string{"postgresql", "nats", "seaweedfs"}
	appServices := []string{"api", "data-manager", "data-consumer"}

	sorted := []string{}

	// Core 서비스 먼저
	for _, proc := range processes {
		for _, core := range coreServices {
			if proc == core {
				sorted = append(sorted, proc)
				break
			}
		}
	}

	// App 서비스 나중에
	for _, proc := range processes {
		for _, app := range appServices {
			if proc == app {
				sorted = append(sorted, proc)
				break
			}
		}
	}

	return sorted
}

// 프로세스 상태 조회 헬퍼
func getProcessStatus(name string) string {
	processes, err := client.GetProcessList()
	if err != nil {
		return "unknown"
	}

	for _, proc := range processes {
		if proc.Name == name {
			return proc.Status
		}
	}

	return "not found"
}

// 일괄 프로세스 제어 명령어
var processBatchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Execute batch operations on multiple processes",
	Long:  "Start, stop, or restart multiple processes at once",
}

var processBatchStartCmd = &cobra.Command{
	Use:   "start <process1> [process2] ...",
	Short: "Start multiple processes",
	Long:  "Start multiple processes in the order specified",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("🚀 Starting %d processes...\n", len(args))

		successCount := 0
		for _, proc := range args {
			fmt.Printf("  Starting %s...", proc)

			if err := client.StartProcess(proc); err != nil {
				if strings.Contains(err.Error(), "already running") {
					fmt.Printf(" ⚠️  Already running\n")
					successCount++
				} else {
					fmt.Printf(" ❌ Failed: %v\n", err)
				}
			} else {
				fmt.Printf(" ✅ Started\n")
				successCount++
			}
		}

		fmt.Printf("\n📊 Results: %d/%d processes started successfully\n", successCount, len(args))
	},
}

var processBatchStopCmd = &cobra.Command{
	Use:   "stop <process1> [process2] ...",
	Short: "Stop multiple processes",
	Long:  "Stop multiple processes in the order specified",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("🛑 Stopping %d processes...\n", len(args))

		successCount := 0
		for _, proc := range args {
			fmt.Printf("  Stopping %s...", proc)

			if err := client.StopProcess(proc); err != nil {
				if strings.Contains(err.Error(), "already stopped") {
					fmt.Printf(" ⚠️  Already stopped\n")
					successCount++
				} else {
					fmt.Printf(" ❌ Failed: %v\n", err)
				}
			} else {
				fmt.Printf(" ✅ Stopped\n")
				successCount++
			}
		}

		fmt.Printf("\n📊 Results: %d/%d processes stopped successfully\n", successCount, len(args))
	},
}

func init() {
	// 프로세스 명령어 구성
	processCmd.AddCommand(processListCmd)
	processCmd.AddCommand(processStatusCmd)
	processCmd.AddCommand(processRestartCmd)
	processCmd.AddCommand(processStopCmd)
	processCmd.AddCommand(processStartCmd)

	// 그룹 명령어 추가
	processGroupCmd.AddCommand(processGroupListCmd)
	processGroupCmd.AddCommand(processGroupStartCmd)
	processGroupCmd.AddCommand(processGroupStopCmd)
	processGroupCmd.AddCommand(processGroupRestartCmd)
	processGroupCmd.AddCommand(processGroupStatusCmd)

	// 배치 명령어 추가
	processBatchCmd.AddCommand(processBatchStartCmd)
	processBatchCmd.AddCommand(processBatchStopCmd)

	// process 명령어에 추가
	processCmd.AddCommand(processGroupCmd)
	processCmd.AddCommand(processBatchCmd)

	rootCmd.AddCommand(processCmd)
}
