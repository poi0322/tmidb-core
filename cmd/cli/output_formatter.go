package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var outputFormat string

// OutputFormatter 출력 형식을 관리하는 구조체
type OutputFormatter struct {
	format string
}

// NewOutputFormatter 새로운 출력 포맷터 생성
func NewOutputFormatter(format string) *OutputFormatter {
	return &OutputFormatter{
		format: format,
	}
}

// Print 데이터를 지정된 형식으로 출력
func (f *OutputFormatter) Print(data interface{}) error {
	switch f.format {
	case "json":
		return f.printJSON(data)
	case "json-pretty":
		return f.printJSONPretty(data)
	default:
		// 기본은 구조체에 따라 다르게 처리
		return f.printDefault(data)
	}
}

// printJSON JSON 형식으로 출력 (한 줄)
func (f *OutputFormatter) printJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(data)
}

// printJSONPretty JSON 형식으로 출력 (들여쓰기)
func (f *OutputFormatter) printJSONPretty(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// printDefault 기본 형식으로 출력
func (f *OutputFormatter) printDefault(data interface{}) error {
	// 데이터 타입에 따라 다르게 처리
	switch v := data.(type) {
	case string:
		fmt.Println(v)
	case []byte:
		fmt.Println(string(v))
	default:
		// 그 외는 %+v로 출력
		fmt.Printf("%+v\n", v)
	}
	return nil
}

// setupGlobalFlags 전역 플래그 설정
func setupGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json, json-pretty)")
}

// getFormatter 현재 명령의 출력 포맷터 반환
func getFormatter(cmd *cobra.Command) *OutputFormatter {
	format, _ := cmd.Flags().GetString("output")
	if format == "" {
		format = outputFormat
	}
	return NewOutputFormatter(format)
}

// FormatProcessList 프로세스 목록을 JSON으로 포맷
func FormatProcessList(processes []interface{}) interface{} {
	type ProcessOutput struct {
		Name      string `json:"name"`
		Status    string `json:"status"`
		PID       int    `json:"pid"`
		Uptime    string `json:"uptime"`
		Memory    string `json:"memory"`
		CPU       string `json:"cpu"`
		StartTime string `json:"start_time,omitempty"`
	}

	var output []ProcessOutput
	for _, p := range processes {
		if proc, ok := p.(map[string]interface{}); ok {
			po := ProcessOutput{
				Name:   getString(proc, "name"),
				Status: getString(proc, "status"),
				PID:    getInt(proc, "pid"),
			}

			// Uptime과 Memory는 포맷팅
			if uptime, ok := proc["uptime"].(float64); ok {
				po.Uptime = formatDuration(time.Duration(uptime))
			}
			if memory, ok := proc["memory"].(float64); ok {
				po.Memory = formatBytes(int64(memory))
			}
			if cpu, ok := proc["cpu"].(float64); ok {
				po.CPU = fmt.Sprintf("%.1f%%", cpu)
			}
			if startTime, ok := proc["start_time"].(string); ok {
				po.StartTime = startTime
			}

			output = append(output, po)
		}
	}

	return output
}

// FormatLogEntry 로그 엔트리를 JSON으로 포맷
func FormatLogEntry(entry map[string]interface{}) interface{} {
	return map[string]string{
		"timestamp": getString(entry, "timestamp"),
		"process":   getString(entry, "process"),
		"level":     getString(entry, "level"),
		"message":   getString(entry, "message"),
	}
}

// FormatSystemHealth 시스템 헬스를 JSON으로 포맷
func FormatSystemHealth(health map[string]interface{}) interface{} {
	type HealthOutput struct {
		Status     string            `json:"status"`
		Uptime     string            `json:"uptime"`
		Components map[string]string `json:"components"`
		Resources  map[string]string `json:"resources"`
		LastCheck  string            `json:"last_check"`
		Errors     []string          `json:"errors,omitempty"`
	}

	output := HealthOutput{
		Status:     getString(health, "status"),
		Components: make(map[string]string),
		Resources:  make(map[string]string),
		Errors:     []string{},
	}

	// Uptime 포맷팅
	if uptime, ok := health["uptime"].(float64); ok {
		output.Uptime = formatDuration(time.Duration(uptime))
	}

	// Components
	if components, ok := health["components"].(map[string]interface{}); ok {
		for k, v := range components {
			output.Components[k] = fmt.Sprintf("%v", v)
		}
	}

	// Resources
	if resources, ok := health["resources"].(map[string]interface{}); ok {
		if cpu, ok := resources["cpu_usage"].(float64); ok {
			output.Resources["cpu"] = fmt.Sprintf("%.1f%%", cpu)
		}
		if mem, ok := resources["memory_usage"].(float64); ok {
			output.Resources["memory"] = fmt.Sprintf("%.1f%%", mem)
		}
		if disk, ok := resources["disk_usage"].(float64); ok {
			output.Resources["disk"] = fmt.Sprintf("%.1f%%", disk)
		}
	}

	// Errors
	if errors, ok := health["errors"].([]interface{}); ok {
		for _, err := range errors {
			output.Errors = append(output.Errors, fmt.Sprintf("%v", err))
		}
	}

	return output
}

// Helper functions
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0.0
}

// 초기화 시 전역 플래그 설정
func init() {
	setupGlobalFlags(rootCmd)
}
