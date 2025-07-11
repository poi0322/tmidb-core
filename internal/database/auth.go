package database

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tmidb/tmidb-core/internal/config"
	"golang.org/x/crypto/bcrypt"
)

// CreateAdminUser는 관리자 사용자를 생성합니다 (초기 설정용)
// 이 함수는 이제 CreateOrgAndAdminUser로 대체될 수 있지만, 이전 로직과의 호환성을 위해 남겨둘 수 있습니다.
func CreateAdminUser(username, password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	_, err = CoreDB.Exec("INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'admin')", username, string(hashedPassword))
	if err != nil {
		return "", err
	}
	// TODO: 이 사용자를 위한 토큰 생성 로직 추가 필요
	return "temp_token", nil // 임시 토큰 반환
}

// AuthenticateUser는 사용자를 인증하고 성공 시 사용자 ID, 조직 ID, 역할을 반환합니다.
func AuthenticateUser(username, password string) (userID, orgID, role string, err error) {
	var storedHash string
	err = CoreDB.QueryRow("SELECT user_id, org_id, password_hash, role FROM users WHERE username = $1 AND is_active = TRUE", username).Scan(&userID, &orgID, &storedHash, &role)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", "", fmt.Errorf("user not found or not active")
		}
		return "", "", "", err
	}

	err = bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
	if err != nil {
		return "", "", "", fmt.Errorf("invalid password")
	}

	return userID, orgID, role, nil
}

// CreateOrgAndAdminUser는 새 조직과 해당 조직의 관리자를 원자적으로 생성하고,
// 생성된 관리자 사용자를 위한 초기 액세스 토큰을 반환합니다.
func CreateOrgAndAdminUser(orgName, username, password string, cfg *config.Config) (string, error) {
	// 1. organization 데이터베이스 생성 및 admin user 생성.
	// 이 과정에서 생성된 orgID와 adminUserID가 반환됩니다.
	orgID, adminUserID, err := CreateOrganizationDatabase(orgName, username, password, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to create organization database: %w", err)
	}

	// 2. 생성된 관리자 사용자를 위한 초기 사용자 액세스 토큰 생성.
	// 이 토큰은 `user_access_tokens`에 저장되어 스키마와 일치합니다.
	accessToken, _, err := CreateUserToken(adminUserID, orgID, "Initial admin setup token")
	if err != nil {
		return "", fmt.Errorf("failed to create initial admin user token: %w", err)
	}

	return accessToken, nil
}

// GenerateAndSaveAuthToken는 새로운 API 토큰을 생성, 암호화, 저장합니다.
func GenerateAndSaveAuthToken(db DBTX, orgID, description string, isAdmin bool, ownerID string, tokenType string, expiresAt *time.Time) (string, error) {
	// 1. 원본 토큰 생성 (32 bytes -> 64 hex chars)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("could not generate token: %w", err)
	}
	tokenString := hex.EncodeToString(tokenBytes)

	// 2. 토큰 해시
	tokenHash := hashToken(tokenString)

	// 3. 권한 설정
	var permissions string
	if isAdmin {
		permissions = `{"admin": true}`
	} else {
		permissions = `{"read": ["*"], "write": []}`
	}

	// 4. 데이터베이스에 저장
	_, err := db.Exec(`
		INSERT INTO auth_tokens (org_id, user_id, token_hash, description, permissions, is_admin, is_active, token_type, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, TRUE, $7, $8)
	`, orgID, ownerID, tokenHash, description, permissions, isAdmin, tokenType, expiresAt)
	if err != nil {
		return "", fmt.Errorf("could not save token to database: %w", err)
	}

	return tokenString, nil
}

// AuthenticateToken은 제공된 토큰이 유효한지 확인하고 권한을 반환합니다.
func AuthenticateToken(tokenString string) (bool, map[string]any, error) {
	// 1. 사용자 액세스 토큰으로 확인 (user_access_tokens)
	tokenHash := hashToken(tokenString)
	var userID, orgID string
	var isActive bool

	err := CoreDB.QueryRow(`
		SELECT user_id, org_id, is_active 
		FROM user_access_tokens 
		WHERE token_hash = $1 AND is_active = TRUE
	`, tokenHash).Scan(&userID, &orgID, &isActive)

	if err == nil && isActive {
		// 사용자 토큰으로 인증 성공
		return true, map[string]any{
			"type":    "user_token",
			"user_id": userID,
			"org_id":  orgID,
			"admin":   false,
		}, nil
	}

	// 2. 관리자 토큰으로 확인 (auth_tokens)
	tokenHash = hashToken(tokenString)

	var permissions string
	var isAdmin bool
	err = CoreDB.QueryRow(`
		SELECT org_id, permissions, is_admin, is_active 
		FROM auth_tokens 
		WHERE token_hash = $1 AND is_active = TRUE
	`, tokenHash).Scan(&orgID, &permissions, &isAdmin, &isActive)

	if err == nil && isActive {
		// 관리자 토큰으로 인증 성공
		var perms map[string]any
		if permissions != "" {
			if err := json.Unmarshal([]byte(permissions), &perms); err != nil {
				perms = map[string]any{"admin": isAdmin}
			}
		} else {
			perms = map[string]any{"admin": isAdmin}
		}

		return true, map[string]any{
			"type":        "admin_token",
			"org_id":      orgID,
			"admin":       isAdmin,
			"permissions": perms,
		}, nil
	}

	// 3. 환경변수 초기 토큰 확인 (개발/테스트용)
	cfg, err := config.Load()
	if err == nil && cfg.InitAdminToken != "" && tokenString == cfg.InitAdminToken {
		// 환경변수 토큰으로 인증 성공 - 기본 조직 찾기
		var defaultOrgID string
		err = CoreDB.QueryRow("SELECT org_id FROM organizations ORDER BY created_at LIMIT 1").Scan(&defaultOrgID)
		if err != nil {
			defaultOrgID = "00000000-0000-4000-8000-000000000000" // 기본 UUID
		}

		return true, map[string]any{
			"type":   "env_token",
			"org_id": defaultOrgID,
			"admin":  true,
		}, nil
	}

	return false, nil, nil
}

// GetAuthTokens는 특정 조직의 모든 인증 토큰을 조회합니다.
func GetAuthTokens(orgID string) ([]AuthToken, error) {
	rows, err := CoreDB.Query(`
		SELECT token_id, user_id, org_id, token_hash, description, permissions, is_admin, is_active, expires_at, created_at
		FROM auth_tokens 
		WHERE org_id = $1 AND token_type = 'api'
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []AuthToken
	for rows.Next() {
		var token AuthToken
		if err := rows.Scan(
			&token.TokenID,
			&token.UserID,
			&token.OrgID,
			&token.EncryptedToken,
			&token.Description,
			&token.Permissions,
			&token.IsAdmin,
			&token.IsAdmin,
			&token.IsActive,
			&token.ExpiresAt,
			&token.CreatedAt,
		); err != nil {
			fmt.Printf("Error scanning token row: %v\n", err)
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// DeleteAuthToken은 특정 조직에서 토큰 ID를 기반으로 토큰을 삭제합니다.
func DeleteAuthToken(tokenID, orgID string) error {
	_, err := CoreDB.Exec("DELETE FROM auth_tokens WHERE token_id = $1 AND org_id = $2", tokenID, orgID)
	return err
}

// AuthToken은 인증 토큰의 구조체입니다.
type AuthToken struct {
	TokenID        string         `json:"token_id"`
	UserID         string         `json:"user_id"`
	OrgID          string         `json:"org_id"`
	EncryptedToken string         `json:"-"` // JSON에 포함되지 않음
	DecryptedToken string         `json:"token,omitempty"`
	Description    sql.NullString `json:"description"`
	Permissions    sql.NullString `json:"permissions"`
	IsAdmin        bool           `json:"is_admin"`
	IsActive       bool           `json:"is_active"`
	ExpiresAt      sql.NullTime   `json:"expires_at"`
	CreatedAt      time.Time      `json:"created_at"`
}

// GenerateSessionToken는 세션 토큰을 생성합니다.
func GenerateSessionToken() (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(tokenBytes), nil
}

// hashToken는 토큰을 해시합니다.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// User는 사용자 구조체입니다.
type User struct {
	UserID    string    `json:"user_id"`
	OrgID     string    `json:"org_id"`
	Username  string    `json:"username"`
	Password  string    `json:"password,omitempty"`
	Role      string    `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetUsers는 특정 조직의 모든 사용자를 조회합니다.
func GetUsers(orgID string) ([]User, error) {
	rows, err := CoreDB.Query(`
		SELECT user_id, org_id, username, role, is_active, created_at, updated_at
		FROM users 
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.UserID, &user.OrgID, &user.Username, &user.Role, &user.IsActive, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

// CreateUser는 새 사용자를 생성합니다.
func CreateUser(user User) (*User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	err = CoreDB.QueryRow(`
		INSERT INTO users (org_id, username, password_hash, role, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING user_id, created_at, updated_at
	`, user.OrgID, user.Username, string(hashedPassword), user.Role, user.IsActive).Scan(&user.UserID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return nil, err
	}

	user.Password = "" // 비밀번호는 반환하지 않음
	return &user, nil
}

// UpdateUser는 사용자 정보를 업데이트합니다.
func UpdateUser(user User) (*User, error) {
	var hashedPassword string
	if user.Password != "" {
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		hashedPassword = string(hashedBytes)
	}

	var query string
	var args []interface{}

	if hashedPassword != "" {
		query = `
			UPDATE users 
			SET username = $1, password_hash = $2, role = $3, is_active = $4, updated_at = now()
			WHERE user_id = $5 AND org_id = $6
			RETURNING created_at, updated_at
		`
		args = []interface{}{user.Username, hashedPassword, user.Role, user.IsActive, user.UserID, user.OrgID}
	} else {
		query = `
			UPDATE users 
			SET username = $1, role = $2, is_active = $3, updated_at = now()
			WHERE user_id = $4 AND org_id = $5
			RETURNING created_at, updated_at
		`
		args = []interface{}{user.Username, user.Role, user.IsActive, user.UserID, user.OrgID}
	}

	err := CoreDB.QueryRow(query, args...).Scan(&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}

	user.Password = "" // 비밀번호는 반환하지 않음
	return &user, nil
}

// DeleteUser는 특정 조직에서 사용자를 삭제합니다.
func DeleteUser(id, orgID string) error {
	_, err := CoreDB.Exec("DELETE FROM users WHERE user_id = $1 AND org_id = $2", id, orgID)
	return err
}

// CreateUserToken는 사용자를 위한 새 API 토큰을 생성합니다.
func CreateUserToken(userID, orgID, description string) (string, *AuthToken, error) {
	// 1. 원본 토큰 생성
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, fmt.Errorf("could not generate token: %w", err)
	}
	tokenString := hex.EncodeToString(tokenBytes)

	// 2. 토큰 해시
	tokenHash := hashToken(tokenString)

	// 3. 데이터베이스에 저장
	var token AuthToken
	err := CoreDB.QueryRow(`
		INSERT INTO user_access_tokens (user_id, org_id, token_hash, description, is_active, expires_at)
		VALUES ($1, $2, $3, $4, TRUE, $5)
		RETURNING token_id, created_at
	`, userID, orgID, tokenHash, description, time.Now().AddDate(1, 0, 0)).Scan(&token.TokenID, &token.CreatedAt)

	if err != nil {
		return "", nil, fmt.Errorf("could not save token to database: %w", err)
	}

	token.UserID = userID
	token.OrgID = orgID
	token.DecryptedToken = tokenString
	token.Description.String = description
	token.Description.Valid = true
	token.IsActive = true

	return tokenString, &token, nil
}

// GetUserTokens는 특정 사용자의 모든 토큰을 조회합니다.
func GetUserTokens(userID, orgID string) ([]AuthToken, error) {
	rows, err := CoreDB.Query(`
		SELECT token_id, user_id, org_id, token_hash, description, is_active, expires_at, created_at
		FROM user_access_tokens 
		WHERE user_id = $1 AND org_id = $2
		ORDER BY created_at DESC
	`, userID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTokens(rows)
}

// GetAllUserTokens는 특정 조직의 모든 사용자 토큰을 조회합니다.
func GetAllUserTokens(orgID string) ([]AuthToken, error) {
	rows, err := CoreDB.Query(`
		SELECT token_id, user_id, org_id, token_hash, description, is_active, expires_at, created_at
		FROM user_access_tokens 
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTokens(rows)
}

// DeleteUserToken는 특정 사용자의 토큰을 삭제합니다.
func DeleteUserToken(tokenID, userID, orgID string) error {
	_, err := CoreDB.Exec("DELETE FROM user_access_tokens WHERE token_id = $1 AND user_id = $2 AND org_id = $3", tokenID, userID, orgID)
	return err
}

// DeleteUserTokenAsAdmin는 관리자 권한으로 토큰을 삭제합니다.
func DeleteUserTokenAsAdmin(tokenID, orgID string) error {
	_, err := CoreDB.Exec("DELETE FROM user_access_tokens WHERE token_id = $1 AND org_id = $2", tokenID, orgID)
	return err
}

// scanTokens는 토큰 행들을 스캔합니다.
func scanTokens(rows *sql.Rows) ([]AuthToken, error) {
	var tokens []AuthToken
	for rows.Next() {
		var token AuthToken
		if err := rows.Scan(
			&token.TokenID,
			&token.UserID,
			&token.OrgID,
			&token.EncryptedToken, // 실제로는 token_hash
			&token.Description,
			&token.Permissions,
			&token.IsAdmin,
			&token.IsActive,
			&token.ExpiresAt,
			&token.CreatedAt,
		); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// GetUserByID는 사용자 ID로 특정 사용자를 조회합니다.
func GetUserByID(db DBTX, userID string) (*User, error) {
	var user User
	err := db.QueryRow(`
		SELECT user_id, username, role, is_active, created_at, updated_at
		FROM users 
		WHERE user_id = $1
	`, userID).Scan(
		&user.UserID,
		&user.Username,
		&user.Role,
		&user.IsActive,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user with ID %s not found", userID)
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	// 사용자가 속한 조직 ID 조회 (user_organizations 테이블에서)
	err = db.QueryRow("SELECT org_id FROM user_organizations WHERE user_id = $1 LIMIT 1", userID).Scan(&user.OrgID)
	if err != nil && err != sql.ErrNoRows {
		// orgID를 찾지 못하는 것은 치명적인 오류는 아닐 수 있으므로, 로그만 남기고 진행할 수 있습니다.
		// 하지만 여기서는 오류로 처리합니다.
		return nil, fmt.Errorf("failed to get organization for user %s: %w", userID, err)
	}

	return &user, nil
}

// GetAuthTokenByID는 토큰 ID로 특정 토큰을 조회합니다.
func GetAuthTokenByID(db DBTX, tokenID string) (*AuthToken, error) {
	var token AuthToken
	err := db.QueryRow(`
		SELECT token_id, org_id, encrypted_token, description, permissions, is_admin, is_active, expires_at, created_at
		FROM auth_tokens 
		WHERE token_id = $1
	`, tokenID).Scan(
		&token.TokenID,
		&token.OrgID,
		&token.EncryptedToken,
		&token.Description,
		&token.Permissions,
		&token.IsAdmin,
		&token.IsActive,
		&token.ExpiresAt,
		&token.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("token with ID %s not found", tokenID)
		}
		return nil, fmt.Errorf("failed to get token by id: %w", err)
	}
	return &token, nil
}
