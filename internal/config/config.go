package config

import (
	"fmt"
	"os"
)

// Config는 애플리케이션의 모든 설정을 담는 구조체입니다.
type Config struct {
	// 데이터베이스 관련 설정
	DatabaseURL      string // tmiDB 전용 사용자 연결 DSN
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string // 초기 설정을 위한 Postgres 관리자 비밀번호
	PostgresDBName   string
	TmiDBUser        string
	TmiDBPassword    string

	// NATS 관련 설정
	NatsURL string

	// 기타
	IsProduction  bool
	EncryptionKey string

	// 초기 설정 관련 (환경변수)
	InitMode       string // "setup" 또는 빈 문자열
	InitOrg        string // 초기 조직명
	InitUser       string // 초기 관리자 사용자명
	InitPassword   string // 초기 관리자 비밀번호
	InitAdminToken string // 초기 관리자 토큰
}

// Load는 환경 변수에서 설정을 로드합니다.
func Load() (*Config, error) {
	// .env 파일을 더 이상 로드하지 않고, 환경 변수를 직접 사용합니다.
	// if err := godotenv.Load(); err != nil {
	// 	log.Println("No .env file found, using environment variables")
	// }

	cfg := &Config{
		PostgresHost:     getEnv("DB_HOST", "localhost"),
		PostgresPort:     getEnv("DB_PORT", "5432"),
		PostgresUser:     getEnv("POSTGRES_USER", "postgres"),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", "postgres"),
		PostgresDBName:   getEnv("POSTGRES_DB", "tmidb"),
		TmiDBUser:        getEnv("TMIDB_USER", "admin"),
		TmiDBPassword:    getEnv("TMIDB_PASSWORD", "admin"), // 이 비밀번호는 안전하게 관리해야 합니다.
		NatsURL:          getEnv("NATS_URL", "nats://localhost:4222"),
		IsProduction:     getEnvAsBool("IS_PRODUCTION", false),
		EncryptionKey:    getEnv("ENCRYPTION_KEY", "e8e1694709a47355153cf11794252386a683d789a781b5399583643f82862e63"), // 32바이트 AES 키(64 hex chars)

		// 초기 설정 환경변수
		InitMode:       getEnv("TMIDB_INIT_MODE", ""),
		InitOrg:        getEnv("TMIDB_INIT_ORG", ""),
		InitUser:       getEnv("TMIDB_INIT_USER", ""),
		InitPassword:   getEnv("TMIDB_INIT_PASSWORD", ""),
		InitAdminToken: getEnv("TMIDB_INIT_ADMIN_TOKEN", ""),
	}

	cfg.DatabaseURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.TmiDBUser, cfg.TmiDBPassword, cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDBName)

	return cfg, nil
}

// getEnv는 환경 변수를 읽거나, 없을 경우 기본값을 반환합니다.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvAsBool는 환경 변수를 bool 값으로 읽습니다.
func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := getEnv(key, "")
	if valueStr == "true" || valueStr == "1" {
		return true
	}
	if valueStr == "false" || valueStr == "0" {
		return false
	}
	return defaultValue
}
