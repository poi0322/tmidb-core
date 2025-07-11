package main

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmidb/tmidb-core/internal/ipc"
)

var (
	logLevel   string
	logSince   string
	logUntil   string
	logPattern string
	logLines   int
	logOutput  string
)

// 로그 관련 명령어들
var logsCmd = &cobra.Command{
	Use:   "logs [component]",
	Short: "Show logs for components",
	Long:  "Show logs for all components or a specific component. Use -f to follow logs in real-time.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := "all"
		if len(args) > 0 {
			component = args[0]
		}

		follow, _ := cmd.Flags().GetBool("follow")

		if follow {
			// Follow 모드
			fmt.Printf("📄 Following logs for: %s (Press Ctrl+C to stop)\n", component)

			// 로그 스트림 시작
			logChan, err := client.StreamLogs(component)
			if err != nil {
				fmt.Printf("❌ Failed to start log stream: %v\n", err)
				os.Exit(1)
			}

			// 신호 처리
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			// 로그 출력 루프
			for {
				select {
				case logEntry, ok := <-logChan:
					if !ok {
						fmt.Println("📄 Log stream ended")
						return
					}
					fmt.Printf("[%s] %s: %s\n",
						logEntry.Timestamp.Format("15:04:05"),
						logEntry.Process,
						logEntry.Message)
				case <-sigChan:
					fmt.Println("\n📄 Log following stopped")
					return
				}
			}
		} else {
			// 일반 로그 표시 (최근 로그)
			fmt.Printf("📄 Recent logs for: %s\n", component)

			// 최근 로그 요청
			resp, err := client.SendMessage(ipc.MessageTypeGetLogs, map[string]any{
				"component": component,
				"lines":     50, // 최근 50줄
			})
			if err != nil {
				fmt.Printf("❌ Failed to get logs: %v\n", err)
				os.Exit(1)
			}

			if !resp.Success {
				fmt.Printf("❌ Error: %s\n", resp.Error)
				os.Exit(1)
			}

			// 로그 출력
			if logs, ok := resp.Data.([]any); ok {
				for _, log := range logs {
					if logMap, ok := log.(map[string]any); ok {
						timestamp := logMap["timestamp"].(string)
						process := logMap["process"].(string)
						message := logMap["message"].(string)
						fmt.Printf("[%s] %s: %s\n", timestamp, process, message)
					}
				}
			}
		}
	},
}

var logsEnableCmd = &cobra.Command{
	Use:   "enable [component]",
	Short: "Enable logs for a specific component",
	Long:  "Enable log output for a specific component (api, data-manager, data-consumer, postgresql, nats, seaweedfs)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🔊 Enabling logs for component: %s\n", component)

		if err := client.EnableLogs(component); err != nil {
			fmt.Printf("❌ Failed to enable logs for %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Logs enabled for %s\n", component)
	},
}

var logsDisableCmd = &cobra.Command{
	Use:   "disable [component]",
	Short: "Disable logs for a specific component",
	Long:  "Disable log output for a specific component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🔇 Disabling logs for component: %s\n", component)

		if err := client.DisableLogs(component); err != nil {
			fmt.Printf("❌ Failed to disable logs for %s: %v\n", component, err)
			os.Exit(1)
		}

		fmt.Printf("✅ Logs disabled for %s\n", component)
	},
}

var logsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show log status for all components",
	Long:  "Display which components have logging enabled or disabled",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📊 Component Log Status:")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("%-18s │ %-15s │ %-20s\n", "COMPONENT", "LOG STATUS", "DESCRIPTION")
		fmt.Println("──────────────────┼─────────────────┼────────────────────")

		status, err := client.GetLogStatus()
		if err != nil {
			fmt.Printf("❌ Failed to get log status: %v\n", err)
			os.Exit(1)
		}

		// 정렬된 순서로 출력
		components := []string{"postgresql", "nats", "seaweedfs", "api", "data-manager", "data-consumer"}
		for _, component := range components {
			if enabled, exists := status[component]; exists {
				var statusIcon, statusText, description string
				if enabled {
					statusIcon = "🔊"
					statusText = "Enabled"
					description = "Logging active"
				} else {
					statusIcon = "🔇"
					statusText = "Disabled"
					description = "Logging paused"
				}
				fmt.Printf("%-18s │ %s %-12s │ %-20s\n", component, statusIcon, statusText, description)
			}
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
}

// 로그 필터 명령어
var logsFilterCmd = &cobra.Command{
	Use:   "filter [component]",
	Short: "Filter logs with advanced options",
	Long: `Filter logs with level, time range, and pattern matching.
	
Examples:
  # Show only ERROR and WARN logs
  tmidb-cli logs filter --level=error
  
  # Show logs from last hour
  tmidb-cli logs filter --since=1h
  
  # Show logs with pattern matching
  tmidb-cli logs filter --pattern="failed|error"
  
  # Combine filters
  tmidb-cli logs filter api --level=warn --since=30m --pattern="connection"`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := "all"
		if len(args) > 0 {
			component = args[0]
		}

		fmt.Printf("📋 Filtering logs for: %s\n", component)

		// 필터 옵션 표시
		if logLevel != "" {
			fmt.Printf("  Level: %s and above\n", strings.ToUpper(logLevel))
		}
		if logSince != "" {
			fmt.Printf("  Since: %s ago\n", logSince)
		}
		if logUntil != "" {
			fmt.Printf("  Until: %s ago\n", logUntil)
		}
		if logPattern != "" {
			fmt.Printf("  Pattern: %s\n", logPattern)
		}

		// 시간 파싱
		var sinceTime, untilTime *time.Time
		if logSince != "" {
			duration, err := parseDuration(logSince)
			if err != nil {
				fmt.Printf("❌ Invalid since duration: %v\n", err)
				return
			}
			t := time.Now().Add(-duration)
			sinceTime = &t
		}
		if logUntil != "" {
			duration, err := parseDuration(logUntil)
			if err != nil {
				fmt.Printf("❌ Invalid until duration: %v\n", err)
				return
			}
			t := time.Now().Add(-duration)
			untilTime = &t
		}

		// 패턴 컴파일
		var patternRegex *regexp.Regexp
		if logPattern != "" {
			var err error
			patternRegex, err = regexp.Compile(logPattern)
			if err != nil {
				fmt.Printf("❌ Invalid pattern: %v\n", err)
				return
			}
		}

		// 로그 요청
		filters := map[string]any{
			"component": component,
			"lines":     logLines,
		}
		if logLevel != "" {
			filters["level"] = logLevel
		}
		if sinceTime != nil {
			filters["since"] = sinceTime.Unix()
		}
		if untilTime != nil {
			filters["until"] = untilTime.Unix()
		}

		resp, err := client.SendMessage(ipc.MessageTypeGetLogs, filters)
		if err != nil {
			fmt.Printf("❌ Failed to get logs: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 로그 필터링 및 출력
		if logs, ok := resp.Data.([]any); ok {
			filteredCount := 0
			for _, log := range logs {
				if logMap, ok := log.(map[string]any); ok {
					// 로그 레벨 필터링
					if logLevel != "" && !matchLogLevel(logMap["level"].(string), logLevel) {
						continue
					}

					// 패턴 매칭
					message := logMap["message"].(string)
					if patternRegex != nil && !patternRegex.MatchString(message) {
						continue
					}

					// 출력
					timestamp := logMap["timestamp"].(string)
					process := logMap["process"].(string)
					level := logMap["level"].(string)

					if logOutput == "json" {
						fmt.Printf(`{"timestamp":"%s","process":"%s","level":"%s","message":"%s"}`+"\n",
							timestamp, process, level, message)
					} else {
						levelColor := getLogLevelColor(level)
						fmt.Printf("[%s] %s%s%s %s: %s\n",
							timestamp, levelColor, level, colorReset, process, message)
					}
					filteredCount++
				}
			}
			fmt.Printf("\n📊 Displayed %d logs (filtered from %d)\n", filteredCount, len(logs))
		}
	},
}

// 로그 검색 명령어
var logsSearchCmd = &cobra.Command{
	Use:   "search <pattern> [component]",
	Short: "Search logs with regex pattern",
	Long:  "Search through logs using regular expression patterns",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		pattern := args[0]
		component := "all"
		if len(args) > 1 {
			component = args[1]
		}

		fmt.Printf("🔍 Searching logs in %s for pattern: %s\n", component, pattern)

		// 패턴 컴파일
		patternRegex, err := regexp.Compile(pattern)
		if err != nil {
			fmt.Printf("❌ Invalid regex pattern: %v\n", err)
			return
		}

		// 로그 요청
		resp, err := client.SendMessage(ipc.MessageTypeGetLogs, map[string]any{
			"component": component,
			"lines":     1000, // 검색을 위해 더 많은 로그 가져오기
		})
		if err != nil {
			fmt.Printf("❌ Failed to get logs: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 검색 및 출력
		if logs, ok := resp.Data.([]any); ok {
			matches := 0
			for _, log := range logs {
				if logMap, ok := log.(map[string]any); ok {
					message := logMap["message"].(string)
					if patternRegex.MatchString(message) {
						timestamp := logMap["timestamp"].(string)
						process := logMap["process"].(string)
						level := logMap["level"].(string)

						// 매칭된 부분 하이라이트
						highlighted := patternRegex.ReplaceAllString(message, "\033[1;33m$0\033[0m")

						levelColor := getLogLevelColor(level)
						fmt.Printf("[%s] %s%s%s %s: %s\n",
							timestamp, levelColor, level, colorReset, process, highlighted)
						matches++
					}
				}
			}
			fmt.Printf("\n📊 Found %d matches\n", matches)
		}
	},
}

// 로그 레벨 색상
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

func getLogLevelColor(level string) string {
	switch strings.ToUpper(level) {
	case "ERROR":
		return colorRed
	case "WARN":
		return colorYellow
	case "INFO":
		return colorBlue
	case "DEBUG":
		return colorGray
	default:
		return ""
	}
}

func matchLogLevel(logLevel, filterLevel string) bool {
	levels := map[string]int{
		"DEBUG": 0,
		"INFO":  1,
		"WARN":  2,
		"ERROR": 3,
	}

	logLevelNum, ok1 := levels[strings.ToUpper(logLevel)]
	filterLevelNum, ok2 := levels[strings.ToUpper(filterLevel)]

	if !ok1 || !ok2 {
		return true // 알 수 없는 레벨은 통과
	}

	return logLevelNum >= filterLevelNum
}

func parseDuration(s string) (time.Duration, error) {
	// 단순한 형식 지원: 1h, 30m, 45s
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration format")
	}

	unit := s[len(s)-1]
	value := s[:len(s)-1]

	var multiplier time.Duration
	switch unit {
	case 'h':
		multiplier = time.Hour
	case 'm':
		multiplier = time.Minute
	case 's':
		multiplier = time.Second
	case 'd':
		// 일 단위 지원
		multiplier = 24 * time.Hour
	default:
		return 0, fmt.Errorf("unknown time unit: %c", unit)
	}

	var num int
	if _, err := fmt.Sscanf(value, "%d", &num); err != nil {
		return 0, err
	}

	return time.Duration(num) * multiplier, nil
}

func init() {
	// 로그 명령어 구성
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output (similar to tail -f)")
	logsCmd.AddCommand(logsEnableCmd)
	logsCmd.AddCommand(logsDisableCmd)
	logsCmd.AddCommand(logsStatusCmd)

	// filter 명령어 플래그
	logsFilterCmd.Flags().StringVar(&logLevel, "level", "", "Minimum log level (debug, info, warn, error)")
	logsFilterCmd.Flags().StringVar(&logSince, "since", "", "Show logs since duration ago (e.g., 1h, 30m, 2d)")
	logsFilterCmd.Flags().StringVar(&logUntil, "until", "", "Show logs until duration ago")
	logsFilterCmd.Flags().StringVar(&logPattern, "pattern", "", "Filter logs by regex pattern")
	logsFilterCmd.Flags().IntVar(&logLines, "lines", 100, "Number of log lines to retrieve")
	logsFilterCmd.Flags().StringVar(&logOutput, "output", "text", "Output format (text, json)")

	// search 명령어 플래그
	logsSearchCmd.Flags().IntVar(&logLines, "lines", 1000, "Number of log lines to search through")
	logsSearchCmd.Flags().StringVar(&logOutput, "output", "text", "Output format (text, json)")

	// logs 명령어에 추가
	logsCmd.AddCommand(logsFilterCmd)
	logsCmd.AddCommand(logsSearchCmd)

	rootCmd.AddCommand(logsCmd)
}
