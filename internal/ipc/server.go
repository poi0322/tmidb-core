package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const (
	DefaultSocketPath = "/bin/tmidb-supervisor.sock"
	MaxConnections    = 100
	ReadTimeout       = 1 * time.Second
	WriteTimeout      = 5 * time.Second
)

// Server IPC 서버 구조체
type Server struct {
	socketPath  string
	listener    net.Listener
	connections map[string]*Connection
	connMutex   sync.RWMutex
	handlers    map[MessageType]HandlerFunc
	logStreams  map[string]chan LogEntry
	streamMutex sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc

	// Go 1.24 기능: 자원 관리를 위한 cleanup 함수들
	cleanupFuncs []func()
	cleanupMutex sync.Mutex
}

// Connection 클라이언트 연결 구조체
type Connection struct {
	ID       string
	Conn     net.Conn
	Reader   *bufio.Reader
	Writer   *bufio.Writer
	LastSeen time.Time

	// Go 1.24 기능: 약한 참조를 통한 메모리 관리
	cleanup func()
}

// HandlerFunc 메시지 핸들러 함수 타입
type HandlerFunc func(conn *Connection, msg *Message) *Response

// NewServer 새로운 IPC 서버 생성
func NewServer(socketPath string) *Server {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}

	ctx, cancel := context.WithCancel(context.Background())

	server := &Server{
		socketPath:   socketPath,
		connections:  make(map[string]*Connection),
		handlers:     make(map[MessageType]HandlerFunc),
		logStreams:   make(map[string]chan LogEntry),
		ctx:          ctx,
		cancel:       cancel,
		cleanupFuncs: make([]func(), 0),
	}

	// Go 1.24 기능: 서버 종료 시 자동 정리를 위한 finalizer 등록
	runtime.SetFinalizer(server, (*Server).cleanup)

	return server
}

// Start 서버 시작
func (s *Server) Start() error {
	// 기존 소켓 파일 제거
	if err := s.removeSocketFile(); err != nil {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// 소켓 디렉토리 생성
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Unix Domain Socket 생성
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create unix socket: %w", err)
	}
	s.listener = listener

	// 소켓 파일 권한 설정
	if err := os.Chmod(s.socketPath, 0666); err != nil {
		log.Printf("Warning: failed to set socket permissions: %v", err)
	}

	log.Printf("🔌 IPC Server listening on %s", s.socketPath)

	// 연결 수락 고루틴 시작
	go s.acceptConnections()

	// 연결 정리 고루틴 시작
	go s.cleanupConnections()

	return nil
}

// Stop 서버 정지
func (s *Server) Stop() error {
	s.cancel()

	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			log.Printf("⚠️ Failed to close listener: %v", err)
		}
	}

	// 모든 연결 종료
	s.connMutex.Lock()
	for _, conn := range s.connections {
		if err := conn.Conn.Close(); err != nil {
			log.Printf("⚠️ Failed to close connection %s: %v", conn.ID, err)
		}
	}
	s.connMutex.Unlock()

	// 소켓 파일 제거
	return s.removeSocketFile()
}

// RegisterHandler 메시지 핸들러 등록
func (s *Server) RegisterHandler(msgType MessageType, handler HandlerFunc) {
	s.handlers[msgType] = handler
}

// BroadcastLogEntry 로그 엔트리를 모든 스트림에 브로드캐스트
func (s *Server) BroadcastLogEntry(entry LogEntry) {
	s.streamMutex.RLock()
	defer s.streamMutex.RUnlock()

	for _, stream := range s.logStreams {
		select {
		case stream <- entry:
		default:
			// 버퍼가 가득 찬 경우 무시 (논블로킹)
		}
	}
}

// acceptConnections 연결 수락 처리
func (s *Server) acceptConnections() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return // 서버가 종료되는 중
			}
			log.Printf("❌ Failed to accept connection: %v", err)
			continue
		}

		// 연결 수 제한 확인
		s.connMutex.RLock()
		connCount := len(s.connections)
		s.connMutex.RUnlock()

		if connCount >= MaxConnections {
			log.Printf("⚠️ Maximum connections reached, rejecting new connection")
			if err := conn.Close(); err != nil {
				log.Printf("⚠️ Failed to close rejected connection: %v", err)
			}
			continue
		}

		// 새 연결 처리
		go s.handleConnection(conn)
	}
}

// handleConnection 개별 연결 처리
func (s *Server) handleConnection(netConn net.Conn) {
	connID := generateID()

	conn := &Connection{
		ID:       connID,
		Conn:     netConn,
		Reader:   bufio.NewReader(netConn),
		Writer:   bufio.NewWriter(netConn),
		LastSeen: time.Now(),
	}

	// Go 1.24 기능: 연결별 정리 함수 설정
	conn.cleanup = func() {
		if err := netConn.Close(); err != nil {
			log.Printf("⚠️ Failed to close netConn for %s: %v", connID, err)
		}
		s.removeConnection(connID)
	}
	// 연결 정리를 위한 finalizer 설정
	runtime.SetFinalizer(conn, func(c *Connection) {
		if c.cleanup != nil {
			c.cleanup()
		}
	})

	// 연결 등록
	s.connMutex.Lock()
	s.connections[connID] = conn
	s.connMutex.Unlock()

	log.Printf("📱 New IPC connection: %s", connID)

	defer func() {
		conn.cleanup()
		log.Printf("📱 IPC connection closed: %s", connID)
	}()

	// 메시지 처리 루프
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// 읽기 타임아웃 설정
		if err := netConn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
			log.Printf("⚠️ Failed to set read deadline: %v", err)
			return
		}

		// 메시지 읽기
		line, err := conn.Reader.ReadString('\n')
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return // 타임아웃 시 연결 종료 (CLI는 한 번의 요청-응답만 필요)
			}
			return // 연결 종료
		}

		// 메시지 파싱
		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			log.Printf("❌ Failed to parse message: %v", err)
			continue
		}

		// 마지막 활동 시간 업데이트
		conn.LastSeen = time.Now()

		// 메시지 처리
		s.handleMessage(conn, &msg)

		// 로그 스트림이 아닌 일반 명령어의 경우 응답 후 연결 종료
		if msg.Type != MessageTypeLogStream {
			return
		}
	}
}

// handleMessage 메시지 처리
func (s *Server) handleMessage(conn *Connection, msg *Message) {
	handler, exists := s.handlers[msg.Type]
	if !exists {
		response := NewResponse(msg.ID, false, nil, "Unknown message type")
		s.sendResponse(conn, response)
		return
	}

	// 핸들러 실행
	response := handler(conn, msg)
	if response != nil {
		s.sendResponse(conn, response)
	}
}

// sendResponse 응답 전송
func (s *Server) sendResponse(conn *Connection, response *Response) {
	data, err := response.ToJSON()
	if err != nil {
		log.Printf("❌ Failed to marshal response: %v", err)
		return
	}

	// 쓰기 타임아웃 설정
	if err := conn.Conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		log.Printf("⚠️ Failed to set write deadline for response: %v", err)
		return
	}

	// 응답 전송
	_, err = conn.Writer.Write(append(data, '\n'))
	if err != nil {
		log.Printf("❌ Failed to send response: %v", err)
		return
	}

	if err := conn.Writer.Flush(); err != nil {
		log.Printf("❌ Failed to flush writer: %v", err)
	}
}

// cleanupConnections 비활성 연결 정리
func (s *Server) cleanupConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupInactiveConnections()
		}
	}
}

// cleanupInactiveConnections 비활성 연결 정리
func (s *Server) cleanupInactiveConnections() {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute)

	for id, conn := range s.connections {
		if conn.LastSeen.Before(cutoff) {
			log.Printf("🧹 Cleaning up inactive connection: %s", id)
			if err := conn.Conn.Close(); err != nil {
				log.Printf("⚠️ Failed to close inactive connection %s: %v", id, err)
			}
			delete(s.connections, id)
		}
	}
}

// removeConnection 연결 제거
func (s *Server) removeConnection(connID string) {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	delete(s.connections, connID)

	// 로그 스트림도 정리
	s.streamMutex.Lock()
	delete(s.logStreams, connID)
	s.streamMutex.Unlock()
}

// removeSocketFile 소켓 파일 제거
func (s *Server) removeSocketFile() error {
	if _, err := os.Stat(s.socketPath); err == nil {
		if err := os.Remove(s.socketPath); err != nil {
			return fmt.Errorf("failed to remove socket file %s: %w", s.socketPath, err)
		}
	}
	return nil
}

// cleanup Go 1.24 기능: 서버 정리
func (s *Server) cleanup() {
	s.cleanupMutex.Lock()
	defer s.cleanupMutex.Unlock()

	// 등록된 모든 정리 함수 실행
	for _, cleanupFunc := range s.cleanupFuncs {
		cleanupFunc()
	}

	if err := s.Stop(); err != nil {
		log.Printf("⚠️ Failed to stop IPC server during cleanup: %v", err)
	}
}

// AddCleanupFunc 정리 함수 추가
func (s *Server) AddCleanupFunc(fn func()) {
	s.cleanupMutex.Lock()
	defer s.cleanupMutex.Unlock()

	s.cleanupFuncs = append(s.cleanupFuncs, fn)
}

// GetConnectionCount 현재 연결 수 반환
func (s *Server) GetConnectionCount() int {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	return len(s.connections)
}

// CreateLogStream 로그 스트림 생성
func (s *Server) CreateLogStream(connID string) chan LogEntry {
	s.streamMutex.Lock()
	defer s.streamMutex.Unlock()

	stream := make(chan LogEntry, 1000) // 버퍼 크기: 1000
	s.logStreams[connID] = stream

	return stream
}

// RemoveLogStream 로그 스트림 제거
func (s *Server) RemoveLogStream(connID string) {
	s.streamMutex.Lock()
	defer s.streamMutex.Unlock()

	if stream, exists := s.logStreams[connID]; exists {
		close(stream)
		delete(s.logStreams, connID)
	}
}