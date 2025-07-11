package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/tmidb/tmidb-core/internal/database"
)

// LoginRequest는 로그인 요청 구조체입니다.
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse는 로그인 응답 구조체입니다.
type LoginResponse struct {
	Success      bool   `json:"success"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	UserInfo     struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
		Role     string `json:"role"`
		OrgID    string `json:"org_id"`
	} `json:"user_info,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Login은 사용자 로그인을 처리합니다.
func Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(LoginResponse{
			Success: false,
			Error:   "Invalid request body",
		})
	}

	// 입력 검증
	if req.Username == "" || req.Password == "" {
		return c.Status(400).JSON(LoginResponse{
			Success: false,
			Error:   "Username and password are required",
		})
	}

	// 사용자 인증
	userID, orgID, role, err := database.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		return c.Status(401).JSON(LoginResponse{
			Success: false,
			Error:   "Invalid username or password",
		})
	}

	// 액세스 토큰 생성
	accessToken, _, err := database.CreateUserToken(userID, orgID, "Login session token")
	if err != nil {
		return c.Status(500).JSON(LoginResponse{
			Success: false,
			Error:   "Failed to generate access token",
		})
	}

	// 성공 응답
	response := LoginResponse{
		Success:     true,
		AccessToken: accessToken,
		ExpiresIn:   3600, // 1시간
	}

	response.UserInfo.UserID = userID
	response.UserInfo.Username = req.Username
	response.UserInfo.Role = role
	response.UserInfo.OrgID = orgID

	return c.JSON(response)
}

// ValidateToken은 토큰 유효성을 검증합니다.
func ValidateToken(c *fiber.Ctx) error {
	// Authorization 헤더에서 토큰 추출
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "Authorization header required",
		})
	}

	// Bearer 토큰 형식 확인
	if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid authorization header format",
		})
	}

	token := authHeader[7:]

	// 토큰 검증 (현재는 기본 구현)
	valid, permissions, err := database.AuthenticateToken(token)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Token validation failed",
		})
	}

	if !valid {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid or expired token",
		})
	}

	return c.JSON(fiber.Map{
		"success":     true,
		"valid":       true,
		"permissions": permissions,
		"message":     "Token is valid",
	})
}

// Logout은 사용자 로그아웃을 처리합니다.
func Logout(c *fiber.Ctx) error {
	// TODO: 토큰 무효화 로직 구현
	// 현재는 클라이언트 측에서 토큰을 제거하도록 안내

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Logged out successfully",
	})
}

// RefreshToken은 토큰을 갱신합니다.
func RefreshToken(c *fiber.Ctx) error {
	// TODO: 리프레시 토큰 로직 구현
	return c.Status(501).JSON(fiber.Map{
		"success": false,
		"error":   "Refresh token not implemented yet",
	})
}
