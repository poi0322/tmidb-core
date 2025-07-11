package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmidb/tmidb-core/internal/ipc"
)

// 진단 명령어
var diagnoseCmd = &cobra.Command{
	Use:   "diagnose",
	Short: "Diagnose and troubleshoot tmiDB issues",
	Long:  "Run diagnostics to identify and troubleshoot issues with tmiDB components",
}

var diagnoseAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Run complete system diagnostics",
	Long:  "Perform comprehensive diagnostics on all tmiDB components",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🔍 Running complete system diagnostics...")
		fmt.Println("This may take a few minutes...")
		fmt.Println()

		// 진단 요청
		resp, err := client.SendMessage(ipc.MessageTypeDiagnoseAll, nil)
		if err != nil {
			fmt.Printf("❌ Failed to run diagnostics: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 진단 결과 표시
		if report, ok := resp.Data.(map[string]any); ok {
			displayDiagnosticReport(report)
		}
	},
}

var diagnoseComponentCmd = &cobra.Command{
	Use:   "component <name>",
	Short: "Diagnose specific component",
	Long:  "Run diagnostics on a specific tmiDB component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		component := args[0]
		fmt.Printf("🔍 Diagnosing component: %s\n", component)

		resp, err := client.SendMessage(ipc.MessageTypeDiagnoseComponent, map[string]any{
			"component": component,
		})
		if err != nil {
			fmt.Printf("❌ Failed to diagnose component: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 컴포넌트 진단 결과 표시
		if report, ok := resp.Data.(map[string]any); ok {
			displayComponentDiagnostic(component, report)
		}
	},
}

var diagnoseConnectivityCmd = &cobra.Command{
	Use:   "connectivity",
	Short: "Check connectivity between components",
	Long:  "Test network connectivity and communication between tmiDB components",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🌐 Checking component connectivity...")

		resp, err := client.SendMessage(ipc.MessageTypeDiagnoseConnectivity, nil)
		if err != nil {
			fmt.Printf("❌ Failed to check connectivity: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 연결성 테스트 결과 표시
		if results, ok := resp.Data.(map[string]any); ok {
			displayConnectivityResults(results)
		}
	},
}

var diagnosePerformanceCmd = &cobra.Command{
	Use:   "performance",
	Short: "Analyze system performance",
	Long:  "Run performance diagnostics and identify bottlenecks",
	Run: func(cmd *cobra.Command, args []string) {
		duration, _ := cmd.Flags().GetDuration("duration")

		fmt.Printf("📊 Running performance diagnostics for %v...\n", duration)
		fmt.Println("Collecting metrics...")

		// 성능 진단 시작
		resp, err := client.SendMessage(ipc.MessageTypeDiagnosePerformance, map[string]any{
			"duration": duration.Seconds(),
		})
		if err != nil {
			fmt.Printf("❌ Failed to run performance diagnostics: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 진행 상황 모니터링
		if diagID, ok := resp.Data.(map[string]any)["id"].(string); ok {
			if err := monitorDiagnosticProgress(diagID, duration); err != nil {
				fmt.Printf("❌ Diagnostic monitoring error: %v\n", err)
				return
			}

			// 결과 가져오기
			resultResp, err := client.SendMessage(ipc.MessageTypeDiagnoseResult, map[string]any{
				"id": diagID,
			})
			if err != nil {
				fmt.Printf("❌ Failed to get results: %v\n", err)
				return
			}

			if results, ok := resultResp.Data.(map[string]any); ok {
				displayPerformanceResults(results)
			}
		}
	},
}

var diagnoseLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Analyze logs for errors and issues",
	Long:  "Scan logs to identify errors, warnings, and potential issues",
	Run: func(cmd *cobra.Command, args []string) {
		hours, _ := cmd.Flags().GetInt("hours")

		fmt.Printf("📄 Analyzing logs from last %d hours...\n", hours)

		resp, err := client.SendMessage(ipc.MessageTypeDiagnoseLogs, map[string]any{
			"hours": hours,
		})
		if err != nil {
			fmt.Printf("❌ Failed to analyze logs: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 로그 분석 결과 표시
		if analysis, ok := resp.Data.(map[string]any); ok {
			displayLogAnalysis(analysis)
		}
	},
}

var diagnoseFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Attempt to fix common issues",
	Long:  "Run automated fixes for common tmiDB issues",
	Run: func(cmd *cobra.Command, args []string) {
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if dryRun {
			fmt.Println("🔧 Running diagnostic fixes (DRY RUN)...")
			fmt.Println("No changes will be made.")
		} else {
			fmt.Println("🔧 Running diagnostic fixes...")
			fmt.Println("⚠️  This will attempt to fix identified issues.")

			if !cmd.Flag("yes").Changed {
				fmt.Print("\nContinue? (yes/no): ")
				var response string
				fmt.Scanln(&response)
				if response != "yes" {
					fmt.Println("❌ Fix cancelled")
					return
				}
			}
		}

		resp, err := client.SendMessage(ipc.MessageTypeDiagnoseFix, map[string]any{
			"dry_run": dryRun,
		})
		if err != nil {
			fmt.Printf("❌ Failed to run fixes: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 수정 결과 표시
		if results, ok := resp.Data.(map[string]any); ok {
			displayFixResults(results, dryRun)
		}
	},
}

// 진단 리포트 표시
func displayDiagnosticReport(report map[string]any) {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("        DIAGNOSTIC REPORT              ")
	fmt.Println("═══════════════════════════════════════")

	// 전체 상태
	status := getString(report, "status")
	statusIcon := "❌"
	if status == "healthy" {
		statusIcon = "✅"
	} else if status == "warning" {
		statusIcon = "⚠️"
	}

	fmt.Printf("\n%s Overall Status: %s\n", statusIcon, strings.ToUpper(status))
	fmt.Printf("Generated: %s\n", getString(report, "timestamp"))

	// 컴포넌트별 상태
	if components, ok := report["components"].(map[string]any); ok {
		fmt.Println("\n📋 Component Status:")
		for comp, data := range components {
			if compData, ok := data.(map[string]any); ok {
				compStatus := getString(compData, "status")
				icon := "❌"
				if compStatus == "healthy" {
					icon = "✅"
				} else if compStatus == "warning" {
					icon = "⚠️"
				}
				fmt.Printf("   %s %-15s: %s\n", icon, comp, compStatus)
			}
		}
	}

	// 발견된 문제
	if issues, ok := report["issues"].([]any); ok && len(issues) > 0 {
		fmt.Printf("\n🚨 Issues Found (%d):\n", len(issues))
		for i, issue := range issues {
			if issueMap, ok := issue.(map[string]any); ok {
				severity := getString(issueMap, "severity")
				severityIcon := "ℹ️"
				if severity == "critical" {
					severityIcon = "🔴"
				} else if severity == "warning" {
					severityIcon = "🟡"
				}

				fmt.Printf("\n   %d. %s [%s] %s\n", i+1, severityIcon, severity, getString(issueMap, "title"))
				fmt.Printf("      Component: %s\n", getString(issueMap, "component"))
				fmt.Printf("      Details: %s\n", getString(issueMap, "details"))

				if solution := getString(issueMap, "solution"); solution != "" {
					fmt.Printf("      💡 Solution: %s\n", solution)
				}
			}
		}
	} else {
		fmt.Println("\n✅ No issues found!")
	}

	// 권장사항
	if recommendations, ok := report["recommendations"].([]any); ok && len(recommendations) > 0 {
		fmt.Printf("\n💡 Recommendations (%d):\n", len(recommendations))
		for _, rec := range recommendations {
			fmt.Printf("   • %v\n", rec)
		}
	}

	fmt.Println("\n═══════════════════════════════════════")
}

// 컴포넌트 진단 결과 표시
func displayComponentDiagnostic(component string, report map[string]any) {
	fmt.Printf("\n🔍 Diagnostic Results for: %s\n", component)
	fmt.Println(strings.Repeat("─", 40))

	// 상태
	status := getString(report, "status")
	statusIcon := "❌"
	if status == "healthy" {
		statusIcon = "✅"
	} else if status == "warning" {
		statusIcon = "⚠️"
	}

	fmt.Printf("\n%s Status: %s\n", statusIcon, status)

	// 체크 항목
	if checks, ok := report["checks"].([]any); ok {
		fmt.Println("\n📋 Diagnostic Checks:")
		for _, check := range checks {
			if checkMap, ok := check.(map[string]any); ok {
				checkName := getString(checkMap, "name")
				checkStatus := getString(checkMap, "status")
				checkIcon := "❌"
				if checkStatus == "passed" {
					checkIcon = "✅"
				} else if checkStatus == "warning" {
					checkIcon = "⚠️"
				}

				fmt.Printf("   %s %s: %s\n", checkIcon, checkName, checkStatus)

				if message := getString(checkMap, "message"); message != "" {
					fmt.Printf("      %s\n", message)
				}
			}
		}
	}

	// 메트릭
	if metrics, ok := report["metrics"].(map[string]any); ok {
		fmt.Println("\n📊 Metrics:")
		for key, value := range metrics {
			fmt.Printf("   %-20s: %v\n", key, value)
		}
	}
}

// 연결성 테스트 결과 표시
func displayConnectivityResults(results map[string]any) {
	fmt.Println("\n🌐 Connectivity Test Results")
	fmt.Println(strings.Repeat("─", 40))

	// 연결 매트릭스
	if matrix, ok := results["matrix"].(map[string]any); ok {
		fmt.Println("\nConnection Matrix:")
		fmt.Printf("%-15s", "FROM \\ TO")

		// 헤더
		components := []string{"api", "data-manager", "data-consumer", "postgresql", "nats", "seaweedfs"}
		for _, comp := range components {
			fmt.Printf("%-12s", comp)
		}
		fmt.Println()

		// 매트릭스 데이터
		for _, from := range components {
			fmt.Printf("%-15s", from)
			if fromData, ok := matrix[from].(map[string]any); ok {
				for _, to := range components {
					if from == to {
						fmt.Printf("%-12s", "-")
					} else if status, ok := fromData[to].(string); ok {
						icon := "❌"
						if status == "connected" {
							icon = "✅"
						}
						fmt.Printf("%-12s", icon)
					} else {
						fmt.Printf("%-12s", "?")
					}
				}
			}
			fmt.Println()
		}
	}

	// 연결 문제
	if issues, ok := results["issues"].([]any); ok && len(issues) > 0 {
		fmt.Printf("\n❌ Connection Issues (%d):\n", len(issues))
		for _, issue := range issues {
			fmt.Printf("   • %v\n", issue)
		}
	} else {
		fmt.Println("\n✅ All connections are healthy!")
	}
}

// 성능 진단 결과 표시
func displayPerformanceResults(results map[string]any) {
	fmt.Println("\n📊 Performance Diagnostic Results")
	fmt.Println(strings.Repeat("═", 50))

	// 요약
	if summary, ok := results["summary"].(map[string]any); ok {
		fmt.Println("\n📌 Summary:")
		fmt.Printf("   Duration: %v\n", summary["duration"])
		fmt.Printf("   Samples: %v\n", summary["samples"])
		fmt.Printf("   Overall Score: %v/100\n", summary["score"])
	}

	// 컴포넌트별 성능
	if components, ok := results["components"].(map[string]any); ok {
		fmt.Println("\n🔧 Component Performance:")
		for comp, data := range components {
			if perfData, ok := data.(map[string]any); ok {
				fmt.Printf("\n   %s:\n", comp)
				fmt.Printf("      CPU Usage:    %.1f%% (avg) / %.1f%% (max)\n",
					getFloat(perfData, "cpu_avg"), getFloat(perfData, "cpu_max"))
				fmt.Printf("      Memory Usage: %s (avg) / %s (max)\n",
					formatBytes(int64(getFloat(perfData, "mem_avg"))),
					formatBytes(int64(getFloat(perfData, "mem_max"))))
				fmt.Printf("      Response Time: %.2fms (avg) / %.2fms (p99)\n",
					getFloat(perfData, "response_avg"), getFloat(perfData, "response_p99"))
			}
		}
	}

	// 병목 현상
	if bottlenecks, ok := results["bottlenecks"].([]any); ok && len(bottlenecks) > 0 {
		fmt.Printf("\n⚠️  Bottlenecks Detected (%d):\n", len(bottlenecks))
		for _, bottleneck := range bottlenecks {
			if b, ok := bottleneck.(map[string]any); ok {
				fmt.Printf("   • %s: %s\n", getString(b, "component"), getString(b, "issue"))
				fmt.Printf("     Impact: %s\n", getString(b, "impact"))
				fmt.Printf("     Recommendation: %s\n", getString(b, "recommendation"))
			}
		}
	}

	// 권장 사항
	if recommendations, ok := results["optimization"].([]any); ok && len(recommendations) > 0 {
		fmt.Printf("\n💡 Optimization Suggestions (%d):\n", len(recommendations))
		for i, rec := range recommendations {
			fmt.Printf("   %d. %v\n", i+1, rec)
		}
	}
}

// 로그 분석 결과 표시
func displayLogAnalysis(analysis map[string]any) {
	fmt.Println("\n📄 Log Analysis Results")
	fmt.Println(strings.Repeat("─", 40))

	// 요약
	if summary, ok := analysis["summary"].(map[string]any); ok {
		fmt.Println("\n📊 Summary:")
		fmt.Printf("   Total Logs Analyzed: %v\n", summary["total"])
		fmt.Printf("   Time Range: %v\n", summary["time_range"])
		fmt.Printf("   Error Rate: %.2f%%\n", getFloat(summary, "error_rate"))
		fmt.Printf("   Warning Rate: %.2f%%\n", getFloat(summary, "warning_rate"))
	}

	// 에러 패턴
	if patterns, ok := analysis["error_patterns"].([]any); ok && len(patterns) > 0 {
		fmt.Printf("\n🔴 Error Patterns (%d):\n", len(patterns))
		for _, pattern := range patterns {
			if p, ok := pattern.(map[string]any); ok {
				fmt.Printf("\n   Pattern: %s\n", getString(p, "pattern"))
				fmt.Printf("   Count: %v\n", p["count"])
				fmt.Printf("   Components: %v\n", p["components"])
				fmt.Printf("   First Seen: %v\n", p["first_seen"])
				fmt.Printf("   Last Seen: %v\n", p["last_seen"])
			}
		}
	}

	// 이상 징후
	if anomalies, ok := analysis["anomalies"].([]any); ok && len(anomalies) > 0 {
		fmt.Printf("\n⚠️  Anomalies Detected (%d):\n", len(anomalies))
		for _, anomaly := range anomalies {
			fmt.Printf("   • %v\n", anomaly)
		}
	}

	// 권장 사항
	if actions, ok := analysis["recommended_actions"].([]any); ok && len(actions) > 0 {
		fmt.Printf("\n💡 Recommended Actions:\n")
		for i, action := range actions {
			fmt.Printf("   %d. %v\n", i+1, action)
		}
	}
}

// 수정 결과 표시
func displayFixResults(results map[string]any, dryRun bool) {
	if dryRun {
		fmt.Println("\n🔧 Fix Results (DRY RUN)")
		fmt.Println("The following actions WOULD be performed:")
	} else {
		fmt.Println("\n🔧 Fix Results")
	}
	fmt.Println(strings.Repeat("─", 40))

	// 수정 작업
	if fixes, ok := results["fixes"].([]any); ok {
		successCount := 0
		for _, fix := range fixes {
			if fixMap, ok := fix.(map[string]any); ok {
				status := getString(fixMap, "status")
				icon := "❌"
				if status == "success" || (dryRun && status == "pending") {
					icon = "✅"
					successCount++
				} else if status == "skipped" {
					icon = "⏭️"
				}

				fmt.Printf("\n%s %s\n", icon, getString(fixMap, "description"))
				fmt.Printf("   Component: %s\n", getString(fixMap, "component"))

				if !dryRun && status == "success" {
					fmt.Printf("   Result: %s\n", getString(fixMap, "result"))
				} else if status == "failed" {
					fmt.Printf("   Error: %s\n", getString(fixMap, "error"))
				}
			}
		}

		fmt.Printf("\n📊 Summary: %d/%d fixes %s\n",
			successCount, len(fixes),
			map[bool]string{true: "would be applied", false: "applied successfully"}[dryRun])
	}

	// 추가 조치 필요
	if manual, ok := results["manual_actions"].([]any); ok && len(manual) > 0 {
		fmt.Printf("\n⚠️  Manual Actions Required:\n")
		for i, action := range manual {
			fmt.Printf("   %d. %v\n", i+1, action)
		}
	}
}

// 진단 진행 상황 모니터링
func monitorDiagnosticProgress(diagID string, duration time.Duration) error {
	fmt.Println()
	startTime := time.Now()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0

	for {
		select {
		case <-ticker.C:
			elapsed := time.Since(startTime)
			progress := float64(elapsed) / float64(duration) * 100
			if progress > 100 {
				progress = 100
			}

			fmt.Printf("\r%s Collecting metrics... %.0f%% [%s]",
				spinner[i%len(spinner)], progress, formatDuration(duration-elapsed))

			i++

			if elapsed >= duration {
				fmt.Printf("\r✅ Metrics collection completed!%s\n", strings.Repeat(" ", 20))
				return nil
			}
		}
	}
}

func init() {
	// 플래그 설정
	diagnosePerformanceCmd.Flags().Duration("duration", 30*time.Second, "Duration for performance diagnostics")
	diagnoseLogsCmd.Flags().Int("hours", 24, "Number of hours to analyze")
	diagnoseFixCmd.Flags().Bool("dry-run", false, "Show what would be fixed without making changes")
	diagnoseFixCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	// 서브커맨드 추가
	diagnoseCmd.AddCommand(diagnoseAllCmd)
	diagnoseCmd.AddCommand(diagnoseComponentCmd)
	diagnoseCmd.AddCommand(diagnoseConnectivityCmd)
	diagnoseCmd.AddCommand(diagnosePerformanceCmd)
	diagnoseCmd.AddCommand(diagnoseLogsCmd)
	diagnoseCmd.AddCommand(diagnoseFixCmd)

	// 루트 명령어에 추가
	rootCmd.AddCommand(diagnoseCmd)
}
