package logger

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tmidb/tmidb-core/internal/ipc"
)

// LogLevel 로그 레벨
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

var logLevelNames = map[LogLevel]string{
	LogLevelDebug: "DEBUG",
	LogLevelInfo:  "INFO",
	LogLevelWarn:  "WARN",
	LogLevelError: "ERROR",
}

// Manager 로그 관리자
type Manager struct {
	config     *LogConfig
	writers    map[string]*ProcessWriter
	writersMux sync.RWMutex
	ipcServer  *ipc.Server
	ctx        context.Context
	cancel     context.CancelFunc

	// 로그 저장 정책
	policies    map[string]*RetentionPolicy
	policiesMux sync.RWMutex

	// 로그 스트림 관리
	streams    map[string]bool // 컴포넌트별 스트림 활성화 상태
	streamsMux sync.RWMutex

	// 콘솔 출력 동기화
	consoleMux sync.Mutex

	// Go 1.24 기능: 자원 관리
	cleanupFuncs []func()
	cleanupMux   sync.Mutex
}

// LogConfig 로그 설정
type LogConfig struct {
	BaseDir       string        `json:"base_dir"`
	Level         LogLevel      `json:"level"`
	MaxFileSize   int64         `json:"max_file_size"` // MB
	MaxFiles      int           `json:"max_files"`
	MaxAge        time.Duration `json:"max_age"`
	Compress      bool          `json:"compress"`
	BufferSize    int           `json:"buffer_size"`
	FlushInterval time.Duration `json:"flush_interval"`
	ConsoleOutput bool          `json:"console_output"`
}

// RetentionPolicy 로그 보관 정책
type RetentionPolicy struct {
	Component   string        `json:"component"`
	MaxFileSize int64         `json:"max_file_size"`
	MaxFiles    int           `json:"max_files"`
	MaxAge      time.Duration `json:"max_age"`
	Compress    bool          `json:"compress"`
	Enabled     bool          `json:"enabled"`
}

// ProcessWriter 프로세스별 로그 라이터
type ProcessWriter struct {
	component     string
	file          *os.File
	writer        *bufio.Writer
	gzipWriter    *gzip.Writer
	currentSize   int64
	rotationCount int
	lastFlush     time.Time
	policy        *RetentionPolicy
	buffer        []byte
	bufferMux     sync.Mutex
}

// NewManager 새로운 로그 관리자 생성
func NewManager(config *LogConfig, ipcServer *ipc.Server) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	if config == nil {
		config = &LogConfig{
			BaseDir:       "./data/logs",
			Level:         LogLevelInfo,
			MaxFileSize:   100, // 100MB
			MaxFiles:      10,
			MaxAge:        24 * time.Hour * 7, // 7일
			Compress:      true,
			BufferSize:    8192,
			FlushInterval: 5 * time.Second,
			ConsoleOutput: true,
		}
	}

	manager := &Manager{
		config:       config,
		writers:      make(map[string]*ProcessWriter),
		ipcServer:    ipcServer,
		ctx:          ctx,
		cancel:       cancel,
		policies:     make(map[string]*RetentionPolicy),
		streams:      make(map[string]bool),
		cleanupFuncs: make([]func(), 0),
	}

	// Go 1.24 기능: 자원 정리를 위한 finalizer 설정
	runtime.SetFinalizer(manager, func(m *Manager) {
		m.cleanup()
	})

	return manager
}

// Start 로그 관리자 시작
func (m *Manager) Start() error {
	// 로그 디렉토리 생성
	if err := os.MkdirAll(m.config.BaseDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// IPC 핸들러 등록
	m.registerIPCHandlers()

	// 주기적 작업 시작
	go m.periodicTasks()

	log.Printf("📝 Log Manager started (dir: %s)", m.config.BaseDir)
	return nil
}

// Stop 로그 관리자 정지
func (m *Manager) Stop() error {
	m.cancel()

	// 모든 라이터 종료
			m.writersMux.Lock()
	for _, writer := range m.writers {
		if err := writer.Close(); err != nil {
			log.Printf("⚠️ Failed to close log writer for %s: %v", writer.component, err)
		}
	}
	m.writersMux.Unlock()

	return nil
}

// GetWriter 프로세스별 로그 라이터 가져오기
func (m *Manager) GetWriter(component string) (*ProcessWriter, error) {
	m.writersMux.RLock()
	if writer, exists := m.writers[component]; exists {
		m.writersMux.RUnlock()
		return writer, nil
	}
	m.writersMux.RUnlock()

	// 새 라이터 생성
	return m.createWriter(component)
}

// WriteLog 로그 작성
func (m *Manager) WriteLog(component string, level LogLevel, message string) error {
	// 레벨 필터링
	if level < m.config.Level {
		return nil
	}

	writer, err := m.GetWriter(component)
	if err != nil {
		return err
	}

	// 로그 엔트리 생성
	entry := ipc.LogEntry{
		Process:   component,
		Level:     logLevelNames[level],
		Message:   message,
		Timestamp: time.Now(),
	}

	// JSON 형태로 직렬화
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// 파일에 쓰기
	if err := writer.Write(data); err != nil {
		return err
	}

	// 콘솔 출력 (동기화)
	if m.config.ConsoleOutput {
		m.consoleMux.Lock()
		color := getComponentColor(entry.Process)
		levelColor := getLevelColor(entry.Level)

		fmt.Printf("%s[%s] %s%s%s: %s%s\n",
			color,
			entry.Timestamp.Format("15:04:05"),
			color,
			entry.Process,
			levelColor,
			entry.Message,
			"\033[0m") // reset color
		m.consoleMux.Unlock()
	}

	// IPC 스트림으로 브로드캐스트
	m.streamsMux.RLock()
	if m.streams[component] {
		if m.ipcServer != nil {
			m.ipcServer.BroadcastLogEntry(entry)
		}
	}
	m.streamsMux.RUnlock()

	return nil
}

// SetLogPolicy 로그 정책 설정
func (m *Manager) SetLogPolicy(component string, policy *RetentionPolicy) {
	m.policiesMux.Lock()
	defer m.policiesMux.Unlock()

	policy.Component = component
	m.policies[component] = policy

	// 기존 라이터 업데이트
	m.writersMux.RLock()
	if writer, exists := m.writers[component]; exists {
		writer.policy = policy
	}
	m.writersMux.RUnlock()
}

// GetLogPolicy 로그 정책 조회
func (m *Manager) GetLogPolicy(component string) *RetentionPolicy {
	m.policiesMux.RLock()
	defer m.policiesMux.RUnlock()

	if policy, exists := m.policies[component]; exists {
		return policy
	}

	// 기본 정책 반환
	return &RetentionPolicy{
		Component:   component,
		MaxFileSize: m.config.MaxFileSize,
		MaxFiles:    m.config.MaxFiles,
		MaxAge:      m.config.MaxAge,
		Compress:    m.config.Compress,
		Enabled:     true,
	}
}

// EnableStream 로그 스트림 활성화
func (m *Manager) EnableStream(component string) {
	m.streamsMux.Lock()
	defer m.streamsMux.Unlock()

	m.streams[component] = true
}

// DisableStream 로그 스트림 비활성화
func (m *Manager) DisableStream(component string) {
	m.streamsMux.Lock()
	defer m.streamsMux.Unlock()

	m.streams[component] = false
}

// GetStreamStatus 스트림 상태 조회
func (m *Manager) GetStreamStatus() map[string]bool {
	m.streamsMux.RLock()
	defer m.streamsMux.RUnlock()

	result := make(map[string]bool)
	for component, enabled := range m.streams {
		result[component] = enabled
	}

	return result
}

// createWriter 새 프로세스 라이터 생성
func (m *Manager) createWriter(component string) (*ProcessWriter, error) {
	m.writersMux.Lock()
	defer m.writersMux.Unlock()

	// 중복 생성 방지
	if writer, exists := m.writers[component]; exists {
		return writer, nil
	}

	// 로그 파일 경로
	logDir := filepath.Join(m.config.BaseDir, component)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory for %s: %w", component, err)
	}

	filename := filepath.Join(logDir, fmt.Sprintf("%s.log", component))

	// 파일 열기
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file for %s: %w", component, err)
	}

	writer := &ProcessWriter{
		component:   component,
		file:        file,
		writer:      bufio.NewWriterSize(file, m.config.BufferSize),
		currentSize: 0,
		lastFlush:   time.Now(),
		policy:      m.GetLogPolicy(component),
		buffer:      make([]byte, 0, m.config.BufferSize),
	}

	// 현재 파일 크기 확인
	if info, err := file.Stat(); err == nil {
		writer.currentSize = info.Size()
	}

	m.writers[component] = writer

	// Go 1.24 기능: 라이터 정리 함수 등록
	m.addCleanupFunc(func() {
		writer.Close()
	})

	return writer, nil
}

// Write 로그 데이터 쓰기
func (pw *ProcessWriter) Write(data []byte) error {
	pw.bufferMux.Lock()
	defer pw.bufferMux.Unlock()

	// 로테이션 확인
	if pw.currentSize+int64(len(data)) > pw.policy.MaxFileSize*1024*1024 {
		if err := pw.rotate(); err != nil {
			return err
		}
	}

	// 데이터 쓰기
	n, err := pw.writer.Write(data)
	if err != nil {
		return err
	}

	// 개행 문자 추가
	if _, err := pw.writer.WriteString("\n"); err != nil {
		return err
	}

	pw.currentSize += int64(n + 1)

	// 주기적 플러시 또는 버퍼 가득 참
	now := time.Now()
	if now.Sub(pw.lastFlush) > time.Duration(5)*time.Second || pw.writer.Buffered() >= pw.writer.Size()-100 {
		if err := pw.writer.Flush(); err != nil {
			return err
		}
		pw.lastFlush = now
	}

	return nil
}

// rotate 로그 파일 로테이션
func (pw *ProcessWriter) rotate() error {
	// 현재 파일 닫기
	if pw.gzipWriter != nil {
		pw.gzipWriter.Close()
		pw.gzipWriter = nil
	}

			if err := pw.writer.Flush(); err != nil {
			log.Printf("⚠️ Failed to flush writer during rotation: %v", err)
			return err
		}

		if err := pw.file.Close(); err != nil {
			log.Printf("⚠️ Failed to close file during rotation: %v", err)
			return err
		}

	// 파일 이동 (압축 포함)
	logDir := filepath.Dir(pw.file.Name())
	baseName := strings.TrimSuffix(filepath.Base(pw.file.Name()), ".log")

	// 기존 로테이션 파일들 이동
	for i := pw.policy.MaxFiles - 2; i >= 0; i-- {
		oldFile := filepath.Join(logDir, fmt.Sprintf("%s.%d.log", baseName, i))
		newFile := filepath.Join(logDir, fmt.Sprintf("%s.%d.log", baseName, i+1))

		if pw.policy.Compress {
			oldFile += ".gz"
			newFile += ".gz"
		}

					if _, err := os.Stat(oldFile); err == nil {
				if err := os.Rename(oldFile, newFile); err != nil {
					log.Printf("⚠️ Failed to rename old log file %s to %s: %v", oldFile, newFile, err)
				}
			}
	}

	// 현재 파일을 .0으로 이동 (압축 포함)
	rotatedFile := filepath.Join(logDir, fmt.Sprintf("%s.0.log", baseName))
	if pw.policy.Compress {
		if err := pw.compressFile(pw.file.Name(), rotatedFile+".gz"); err != nil {
			log.Printf("❌ Failed to compress log file: %v", err)
		} else {
			if err := os.Remove(pw.file.Name()); err != nil {
									log.Printf("⚠️ Failed to remove original log file after compression: %v", err)
				}
			}
		} else {
			if err := os.Rename(pw.file.Name(), rotatedFile); err != nil {
				log.Printf("⚠️ Failed to rename current log file to rotated file: %v", err)
			}
		}

	// 새 파일 생성
	file, err := os.OpenFile(pw.file.Name(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	pw.file = file
	pw.writer = bufio.NewWriterSize(file, 8192)
	pw.currentSize = 0
	pw.rotationCount++

	return nil
}

// compressFile 파일 압축
func (pw *ProcessWriter) compressFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()

	_, err = io.Copy(gzWriter, srcFile)
	return err
}

// Close 라이터 종료
func (pw *ProcessWriter) Close() error {
	pw.bufferMux.Lock()
	defer pw.bufferMux.Unlock()

	if pw.gzipWriter != nil {
		pw.gzipWriter.Close()
	}

	if pw.writer != nil {
		pw.writer.Flush()
	}

	if pw.file != nil {
		return pw.file.Close()
	}

	return nil
}

// periodicTasks 주기적 작업
func (m *Manager) periodicTasks() {
	ticker := time.NewTicker(24 * time.Hour) // 하루에 한 번만 정리
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupOldLogs()
		}
	}
}

// cleanupOldLogs 오래된 로그 정리
func (m *Manager) cleanupOldLogs() {
	m.policiesMux.RLock()
	policies := make(map[string]*RetentionPolicy)
	for k, v := range m.policies {
		policies[k] = v
	}
	m.policiesMux.RUnlock()

	for component, policy := range policies {
		if !policy.Enabled {
			continue
		}

		logDir := filepath.Join(m.config.BaseDir, component)
		m.cleanupComponentLogs(logDir, policy)
	}
}

// cleanupComponentLogs 컴포넌트별 로그 정리
func (m *Manager) cleanupComponentLogs(logDir string, policy *RetentionPolicy) {
	files, err := filepath.Glob(filepath.Join(logDir, "*.log*"))
	if err != nil {
		return
	}

	// 파일 정보 수집
	type fileInfo struct {
		path string
		info os.FileInfo
	}

	var fileInfos []fileInfo
	for _, file := range files {
		if info, err := os.Stat(file); err == nil {
			fileInfos = append(fileInfos, fileInfo{file, info})
		}
	}

	// 수정 시간 기준 정렬 (오래된 것부터)
	sort.Slice(fileInfos, func(i, j int) bool {
		return fileInfos[i].info.ModTime().Before(fileInfos[j].info.ModTime())
	})

	// 오래된 파일 삭제 (나이 기준)
	cutoff := time.Now().Add(-policy.MaxAge)
	for _, fi := range fileInfos {
		if fi.info.ModTime().Before(cutoff) {
			os.Remove(fi.path)
			log.Printf("🗑️ Removed old log file: %s", fi.path)
		}
	}

	// 파일 수 제한
	if len(fileInfos) > policy.MaxFiles {
		for i := 0; i < len(fileInfos)-policy.MaxFiles; i++ {
			os.Remove(fileInfos[i].path)
			log.Printf("🗑️ Removed excess log file: %s", fileInfos[i].path)
		}
	}
}

// registerIPCHandlers IPC 핸들러 등록
func (m *Manager) registerIPCHandlers() {
	if m.ipcServer == nil {
		return
	}

	// 로그 활성화/비활성화
	m.ipcServer.RegisterHandler(ipc.MessageTypeLogEnable, m.handleLogEnable)
	m.ipcServer.RegisterHandler(ipc.MessageTypeLogDisable, m.handleLogDisable)
	m.ipcServer.RegisterHandler(ipc.MessageTypeLogStatus, m.handleLogStatus)
	m.ipcServer.RegisterHandler(ipc.MessageTypeLogConfig, m.handleLogConfig)
}

// handleLogEnable 로그 활성화 핸들러
func (m *Manager) handleLogEnable(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component parameter required")
	}

	m.EnableStream(component)
	return ipc.NewResponse(msg.ID, true, map[string]any{
		"component": component,
		"enabled":   true,
	}, "")
}

// handleLogDisable 로그 비활성화 핸들러
func (m *Manager) handleLogDisable(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component parameter required")
	}

	m.DisableStream(component)
	return ipc.NewResponse(msg.ID, true, map[string]any{
		"component": component,
		"enabled":   false,
	}, "")
}

// handleLogStatus 로그 상태 핸들러
func (m *Manager) handleLogStatus(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	status := m.GetStreamStatus()
	return ipc.NewResponse(msg.ID, true, status, "")
}

// handleLogConfig 로그 설정 핸들러
func (m *Manager) handleLogConfig(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
	component, ok := msg.Data["component"].(string)
	if !ok {
		return ipc.NewResponse(msg.ID, false, nil, "component parameter required")
	}

	policy := m.GetLogPolicy(component)
	return ipc.NewResponse(msg.ID, true, policy, "")
}

// cleanup Go 1.24 기능: 자원 정리
func (m *Manager) cleanup() {
	m.cleanupMux.Lock()
	defer m.cleanupMux.Unlock()

	for _, cleanupFunc := range m.cleanupFuncs {
		cleanupFunc()
	}

	if err := m.Stop(); err != nil {
		log.Printf("⚠️ Failed to stop log manager during cleanup: %v", err)
	}
}

// addCleanupFunc 정리 함수 추가
func (m *Manager) addCleanupFunc(fn func()) {
	m.cleanupMux.Lock()
	defer m.cleanupMux.Unlock()

	m.cleanupFuncs = append(m.cleanupFuncs, fn)
}

// getComponentColor returns ANSI color code for different components
func getComponentColor(component string) string {
	colors := map[string]string{
		"api":           "\033[32m", // Green
		"data-manager":  "\033[34m", // Blue
		"data-consumer": "\033[35m", // Magenta
		"postgresql":    "\033[36m", // Cyan
		"nats":          "\033[33m", // Yellow
		"seaweedfs":     "\033[31m", // Red
		"supervisor":    "\033[37m", // White
	}

	if color, exists := colors[component]; exists {
		return color
	}
	return "\033[37m" // Default white
}

// getLevelColor returns ANSI color code for different log levels
func getLevelColor(level string) string {
	switch level {
	case "DEBUG":
		return "\033[90m" // Dark gray
	case "INFO":
		return "\033[37m" // White
	case "WARN":
		return "\033[93m" // Bright yellow
	case "ERROR":
		return "\033[91m" // Bright red
	default:
		return "\033[37m" // Default white
	}
}
