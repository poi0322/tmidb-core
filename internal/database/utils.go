package database

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	// invalidDBNameChars는 데이터베이스 이름에 사용할 수 없는 문자를 찾기 위한 정규식입니다.
	invalidDBNameChars = regexp.MustCompile(`[^a-zA-Z0-9_]`)
)

// getEnvOrDefault는 환경변수 값을 가져오거나 기본값을 반환합니다.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GenerateRandomString은 지정된 길이의 암호학적으로 안전한 랜덤 문자열을 생성합니다.
func GenerateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// SanitizeDBName은 문자열에서 데이터베이스 이름으로 사용할 수 없는 문자를 제거하고 소문자로 변환합니다.
func SanitizeDBName(name string) string {
	// 모든 유효하지 않은 문자를 빈 문자열로 대체
	sanitized := invalidDBNameChars.ReplaceAllString(name, "")
	// 소문자로 변환
	return strings.ToLower(sanitized)
}
