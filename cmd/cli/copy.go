package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tmidb/tmidb-core/internal/ipc"

	"github.com/spf13/cobra"
)

// Copy 관련 명령어들
var copyCmd = &cobra.Command{
	Use:   "copy",
	Short: "File copy operations between tmiDB instances",
	Long:  "Start copy receiver, send files, and manage copy sessions",
}

var copyReceiveCmd = &cobra.Command{
	Use:   "receive [--port PORT] [--path PATH]",
	Short: "Start copy receiver to accept incoming files",
	Long:  "Start a copy receiver that listens on specified port and saves files to specified path",
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")
		path, _ := cmd.Flags().GetString("path")

		data := map[string]interface{}{
			"port": port,
			"path": path,
		}

		resp, err := client.SendMessage(ipc.MessageTypeCopyReceive, data)
		if err != nil {
			fmt.Printf("❌ Failed to start copy receiver: %v\n", err)
			os.Exit(1)
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if sessionData, ok := resp.Data.(map[string]interface{}); ok {
			sessionID := sessionData["id"].(string)
			actualPort := int(sessionData["port"].(float64))
			actualPath := sessionData["path"].(string)

			fmt.Printf("🎯 Copy receiver started successfully\n")
			fmt.Printf("📡 Session ID: %s\n", sessionID)
			fmt.Printf("🔌 Listening on port: %d\n", actualPort)
			fmt.Printf("📁 Saving files to: %s\n", actualPath)
			fmt.Printf("💡 Use 'tmidb-cli copy send <file> <host>:%d' to send files\n", actualPort)
		}
	},
}

var copySendCmd = &cobra.Command{
	Use:   "send <file/directory> <target-host:port>",
	Short: "Send file or directory to copy receiver",
	Long:  "Send a file or directory to a running copy receiver on target host",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		target := args[1]

		// target 파싱 (host:port)
		parts := strings.Split(target, ":")
		if len(parts) != 2 {
			fmt.Printf("❌ Invalid target format. Use host:port\n")
			os.Exit(1)
		}

		targetHost := parts[0]
		targetPort, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Printf("❌ Invalid port number: %s\n", parts[1])
			os.Exit(1)
		}

		data := map[string]interface{}{
			"file_path":   filePath,
			"target_host": targetHost,
			"target_port": targetPort,
		}

		resp, err := client.SendMessage(ipc.MessageTypeCopySend, data)
		if err != nil {
			fmt.Printf("❌ Failed to send file: %v\n", err)
			os.Exit(1)
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if sessionData, ok := resp.Data.(map[string]interface{}); ok {
			sessionID := sessionData["id"].(string)
			fileSize := int64(sessionData["file_size"].(float64))

			fmt.Printf("🚀 File transfer started\n")
			fmt.Printf("📡 Session ID: %s\n", sessionID)
			fmt.Printf("📁 File: %s\n", filePath)
			fmt.Printf("🎯 Target: %s:%d\n", targetHost, targetPort)
			fmt.Printf("📊 Size: %s\n", formatBytes(fileSize))
			fmt.Printf("💡 Use 'tmidb-cli copy status %s' to monitor progress\n", sessionID)
		}
	},
}

var copyStatusCmd = &cobra.Command{
	Use:   "status [session-id]",
	Short: "Show copy session status",
	Long:  "Display status of all copy sessions or specific session",
	Run: func(cmd *cobra.Command, args []string) {
		var sessionID string
		if len(args) > 0 {
			sessionID = args[0]
		}

		data := map[string]interface{}{}
		if sessionID != "" {
			data["session_id"] = sessionID
		}

		resp, err := client.SendMessage(ipc.MessageTypeCopyStatus, data)
		if err != nil {
			fmt.Printf("❌ Failed to get copy status: %v\n", err)
			os.Exit(1)
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			os.Exit(1)
		}

		// 단일 세션 상태 표시
		if sessionID != "" {
			if sessionData, ok := resp.Data.(map[string]interface{}); ok {
				displaySingleSession(sessionData)
			}
			return
		}

		// 모든 세션 상태 표시
		if sessions, ok := resp.Data.([]interface{}); ok {
			if len(sessions) == 0 {
				fmt.Println("📭 No active copy sessions")
				return
			}

			fmt.Println("📋 Active Copy Sessions:")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Printf("%-12s %-8s %-12s %-8s %-20s %-10s %-10s\n",
				"SESSION", "MODE", "STATUS", "PORT", "PATH/TARGET", "PROGRESS", "SPEED")
			fmt.Println("────────────────────────────────────────────────────────────────────────────────────────")

			for _, session := range sessions {
				if sessionMap, ok := session.(map[string]interface{}); ok {
					displaySessionRow(sessionMap)
				}
			}
		}
	},
}

var copyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all copy sessions",
	Long:  "Display list of all copy sessions with basic information",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := client.SendMessage(ipc.MessageTypeCopyList, nil)
		if err != nil {
			fmt.Printf("❌ Failed to list copy sessions: %v\n", err)
			os.Exit(1)
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if sessions, ok := resp.Data.([]interface{}); ok {
			if len(sessions) == 0 {
				fmt.Println("📭 No copy sessions found")
				return
			}

			fmt.Printf("📋 Copy Sessions (%d total):\n", len(sessions))
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			for _, session := range sessions {
				if sessionMap, ok := session.(map[string]interface{}); ok {
					displaySessionSummary(sessionMap)
				}
			}
		}
	},
}

var copyStopCmd = &cobra.Command{
	Use:   "stop <session-id>",
	Short: "Stop copy session",
	Long:  "Stop a running copy session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sessionID := args[0]

		data := map[string]interface{}{
			"session_id": sessionID,
		}

		resp, err := client.SendMessage(ipc.MessageTypeCopyStop, data)
		if err != nil {
			fmt.Printf("❌ Failed to stop copy session: %v\n", err)
			os.Exit(1)
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			os.Exit(1)
		}

		fmt.Printf("✅ Copy session %s stopped successfully\n", sessionID)
	},
}

// 헬퍼 함수들
func displaySingleSession(sessionData map[string]interface{}) {
	id := getCopyString(sessionData, "id")
	mode := getCopyString(sessionData, "mode")
	status := getCopyString(sessionData, "status")
	port := getCopyInt(sessionData, "port")
	path := getCopyString(sessionData, "path")
	targetHost := getCopyString(sessionData, "target_host")
	targetPort := getCopyInt(sessionData, "target_port")
	fileSize := getCopyInt64(sessionData, "file_size")
	transferred := getCopyInt64(sessionData, "transferred")
	speed := getCopyFloat64(sessionData, "speed")

	fmt.Printf("📡 Copy Session Details:\n")
	fmt.Printf("🆔 Session ID: %s\n", id)
	fmt.Printf("🔄 Mode: %s\n", mode)
	fmt.Printf("📊 Status: %s\n", getCopyStatusIcon(status)+status)

	if mode == "receive" {
		fmt.Printf("🔌 Port: %d\n", port)
		fmt.Printf("📁 Save Path: %s\n", path)
	} else {
		fmt.Printf("📁 File Path: %s\n", path)
		fmt.Printf("🎯 Target: %s:%d\n", targetHost, targetPort)
	}

	if fileSize > 0 {
		progress := float64(transferred) / float64(fileSize) * 100
		fmt.Printf("📊 Progress: %.1f%% (%s / %s)\n", progress, formatBytes(transferred), formatBytes(fileSize))
		fmt.Printf("🚀 Speed: %.2f MB/s\n", speed)

		if speed > 0 && transferred < fileSize {
			eta := float64(fileSize-transferred) / (speed * 1024 * 1024)
			fmt.Printf("⏱️ ETA: %s\n", formatDuration(time.Duration(eta)*time.Second))
		}
	}
}

func displaySessionRow(sessionData map[string]interface{}) {
	id := getCopyString(sessionData, "id")
	mode := getCopyString(sessionData, "mode")
	status := getCopyString(sessionData, "status")
	port := getCopyInt(sessionData, "port")
	path := getCopyString(sessionData, "path")
	targetHost := getCopyString(sessionData, "target_host")
	targetPort := getCopyInt(sessionData, "target_port")
	fileSize := getCopyInt64(sessionData, "file_size")
	transferred := getCopyInt64(sessionData, "transferred")
	speed := getCopyFloat64(sessionData, "speed")

	// 짧은 ID 표시
	shortID := id
	if len(id) > 8 {
		shortID = id[:8] + "..."
	}

	// 경로/타겟 정보
	pathTarget := path
	if mode == "send" && targetHost != "" {
		pathTarget = fmt.Sprintf("%s:%d", targetHost, targetPort)
	}
	if len(pathTarget) > 18 {
		pathTarget = pathTarget[:15] + "..."
	}

	// 진행률 계산
	progress := "N/A"
	if fileSize > 0 {
		pct := float64(transferred) / float64(fileSize) * 100
		progress = fmt.Sprintf("%.1f%%", pct)
	}

	// 속도 표시
	speedStr := "N/A"
	if speed > 0 {
		speedStr = fmt.Sprintf("%.1fMB/s", speed)
	}

	fmt.Printf("%-12s %-8s %-12s %-8d %-20s %-10s %-10s\n",
		shortID, mode, status, port, pathTarget, progress, speedStr)
}

func displaySessionSummary(sessionData map[string]interface{}) {
	id := getCopyString(sessionData, "id")
	mode := getCopyString(sessionData, "mode")
	status := getCopyString(sessionData, "status")
	path := getCopyString(sessionData, "path")
	startTime := getCopyString(sessionData, "start_time")

	statusIcon := getCopyStatusIcon(status)
	fmt.Printf("%s %s (%s) - %s\n", statusIcon, id, mode, status)
	fmt.Printf("   📁 %s\n", path)
	fmt.Printf("   🕐 Started: %s\n", startTime)
	fmt.Println()
}

func getCopyStatusIcon(status string) string {
	switch status {
	case "listening":
		return "👂 "
	case "connected":
		return "🔗 "
	case "transferring":
		return "📤 "
	case "completed":
		return "✅ "
	case "failed":
		return "❌ "
	default:
		return "⚪ "
	}
}

func getCopyString(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getCopyInt(data map[string]interface{}, key string) int {
	if val, ok := data[key]; ok {
		if num, ok := val.(float64); ok {
			return int(num)
		}
	}
	return 0
}

func getCopyInt64(data map[string]interface{}, key string) int64 {
	if val, ok := data[key]; ok {
		if num, ok := val.(float64); ok {
			return int64(num)
		}
	}
	return 0
}

func getCopyFloat64(data map[string]interface{}, key string) float64 {
	if val, ok := data[key]; ok {
		if num, ok := val.(float64); ok {
			return num
		}
	}
	return 0.0
}

func init() {
	// copy receive 플래그
	copyReceiveCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	copyReceiveCmd.Flags().StringP("path", "d", "/tmp/received", "Directory to save received files")

	// copy 하위 명령어 추가
	copyCmd.AddCommand(copyReceiveCmd)
	copyCmd.AddCommand(copySendCmd)
	copyCmd.AddCommand(copyStatusCmd)
	copyCmd.AddCommand(copyListCmd)
	copyCmd.AddCommand(copyStopCmd)
}
