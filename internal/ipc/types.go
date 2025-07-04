package ipc

import (
	"encoding/json"
	"time"
)

// MessageType IPC 메시지 타입
type MessageType string

const (
	// 로그 관련
	MessageTypeLogEnable  MessageType = "log_enable"
	MessageTypeLogDisable MessageType = "log_disable"
	MessageTypeLogStatus  MessageType = "log_status"
	MessageTypeLogStream  MessageType = "log_stream"
	MessageTypeLogConfig  MessageType = "log_config"
	MessageTypeGetLogs    MessageType = "get_logs"

	// 프로세스 관련
	MessageTypeProcessList    MessageType = "process_list"
	MessageTypeProcessStatus  MessageType = "process_status"
	MessageTypeProcessStart   MessageType = "process_start"
	MessageTypeProcessStop    MessageType = "process_stop"
	MessageTypeProcessRestart MessageType = "process_restart"

	// 시스템 관련
	MessageTypeSystemHealth MessageType = "system_health"
	MessageTypeSystemStats  MessageType = "system_stats"

	// 설정 관련
	MessageTypeConfigGet      MessageType = "config_get"
	MessageTypeConfigSet      MessageType = "config_set"
	MessageTypeConfigList     MessageType = "config_list"
	MessageTypeConfigReset    MessageType = "config_reset"
	MessageTypeConfigImport   MessageType = "config_import"
	MessageTypeConfigValidate MessageType = "config_validate"

	// 백업 관련
	MessageTypeBackupCreate    MessageType = "backup_create"
	MessageTypeBackupRestore   MessageType = "backup_restore"
	MessageTypeBackupList      MessageType = "backup_list"
	MessageTypeBackupDelete    MessageType = "backup_delete"
	MessageTypeBackupVerify    MessageType = "backup_verify"
	MessageTypeBackupProgress  MessageType = "backup_progress"
	MessageTypeRestoreProgress MessageType = "restore_progress"

	// 진단 관련
	MessageTypeDiagnoseAll          MessageType = "diagnose_all"
	MessageTypeDiagnoseComponent    MessageType = "diagnose_component"
	MessageTypeDiagnoseConnectivity MessageType = "diagnose_connectivity"
	MessageTypeDiagnosePerformance  MessageType = "diagnose_performance"
	MessageTypeDiagnoseLogs         MessageType = "diagnose_logs"
	MessageTypeDiagnoseFix          MessageType = "diagnose_fix"
	MessageTypeDiagnoseResult       MessageType = "diagnose_result"

	// 복사 관련
	MessageTypeCopyReceive MessageType = "copy_receive"
	MessageTypeCopySend    MessageType = "copy_send"
	MessageTypeCopyStatus  MessageType = "copy_status"
	MessageTypeCopyList    MessageType = "copy_list"
	MessageTypeCopyStop    MessageType = "copy_stop"

	// 응답
	MessageTypeResponse MessageType = "response"
	MessageTypeError    MessageType = "error"
)

// Message IPC 메시지 구조체
type Message struct {
	ID        string                 `json:"id"`
	Type      MessageType            `json:"type"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// Response IPC 응답 구조체
type Response struct {
	ID      string      `json:"id"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// LogEntry 로그 엔트리 구조체
type LogEntry struct {
	Process   string    `json:"process"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// ProcessInfo 프로세스 정보 구조체
type ProcessInfo struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Status    string            `json:"status"`
	PID       int               `json:"pid"`
	Uptime    time.Duration     `json:"uptime"`
	Memory    int64             `json:"memory"`
	CPU       float64           `json:"cpu"`
	Enabled   bool              `json:"enabled"`
	Logs      bool              `json:"logs"`
	StartTime time.Time         `json:"start_time"`
	Config    map[string]string `json:"config,omitempty"`
}

// LogConfig 로그 설정 구조체
type LogConfig struct {
	Enabled       bool          `json:"enabled"`
	Level         string        `json:"level"`          // debug, info, warn, error
	MaxFileSize   int64         `json:"max_file_size"`  // MB
	MaxFiles      int           `json:"max_files"`      // 보관할 파일 수
	MaxAge        time.Duration `json:"max_age"`        // 보관 기간
	Compress      bool          `json:"compress"`       // 압축 여부
	OutputFile    string        `json:"output_file"`    // 로그 파일 경로
	OutputConsole bool          `json:"output_console"` // 콘솔 출력 여부
	BufferSize    int           `json:"buffer_size"`    // 버퍼 크기
	FlushInterval time.Duration `json:"flush_interval"` // 플러시 간격
}

// SystemHealth 시스템 헬스 정보
type SystemHealth struct {
	Status     string            `json:"status"`
	Uptime     time.Duration     `json:"uptime"`
	Components map[string]string `json:"components"`
	Resources  SystemResources   `json:"resources"`
	LastCheck  time.Time         `json:"last_check"`
	Errors     []string          `json:"errors,omitempty"`
}

// SystemResources 시스템 리소스 정보
type SystemResources struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	NetworkIO   int64   `json:"network_io"`
	DiskIO      int64   `json:"disk_io"`
}

// CopySession 복사 세션 정보
type CopySession struct {
	ID          string    `json:"id"`
	Mode        string    `json:"mode"`        // "receive" or "send"
	Status      string    `json:"status"`      // "listening", "connected", "transferring", "completed", "failed"
	Port        int       `json:"port"`        // 수신 포트
	Path        string    `json:"path"`        // 수신 경로 또는 전송 파일 경로
	TargetHost  string    `json:"target_host"` // 전송 대상 호스트 (send 모드)
	TargetPort  int       `json:"target_port"` // 전송 대상 포트 (send 모드)
	FileSize    int64     `json:"file_size"`   // 파일 크기
	Transferred int64     `json:"transferred"` // 전송된 바이트
	Speed       float64   `json:"speed"`       // 전송 속도 (MB/s)
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// CopyProgress 복사 진행 상태
type CopyProgress struct {
	SessionID   string  `json:"session_id"`
	Progress    float64 `json:"progress"`    // 0-100 퍼센트
	Transferred int64   `json:"transferred"` // 전송된 바이트
	Total       int64   `json:"total"`       // 총 바이트
	Speed       float64 `json:"speed"`       // 현재 속도 (MB/s)
	ETA         int64   `json:"eta"`         // 예상 완료 시간 (초)
}

// NewMessage 새로운 메시지 생성
func NewMessage(msgType MessageType, data map[string]interface{}) *Message {
	return &Message{
		ID:        generateID(),
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// NewResponse 새로운 응답 생성
func NewResponse(id string, success bool, data interface{}, err string) *Response {
	return &Response{
		ID:      id,
		Success: success,
		Data:    data,
		Error:   err,
	}
}

// ToJSON 메시지를 JSON으로 변환
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON JSON에서 메시지로 변환
func (m *Message) FromJSON(data []byte) error {
	return json.Unmarshal(data, m)
}

// ToJSON 응답을 JSON으로 변환
func (r *Response) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// FromJSON JSON에서 응답으로 변환
func (r *Response) FromJSON(data []byte) error {
	return json.Unmarshal(data, r)
}

// generateID 고유 ID 생성
func generateID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

// randomString 랜덤 문자열 생성
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
