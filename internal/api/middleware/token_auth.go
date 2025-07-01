package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmidb/tmidb-core/internal/database"
)

// TokenClaims는 토큰에 포함된 정보를 나타냅니다
type TokenClaims struct {
	UserID     int    `json:"user_id"`
	OrgID      int    `json:"org_id"`
	Username   string `json:"username"`
	Role       string `json:"role"`        // "admin", "viewer"
	TokenType  string `json:"token_type"`  // "permanent", "temporary"
	Categories []string `json:"categories"` // 접근 가능한 카테고리 목록
	ExpiresAt  int64  `json:"expires_at"`
}

// CategoryPermissionFunc는 카테고리 권한을 확인하는 함수 타입입니다
type CategoryPermissionFunc func(c *fiber.Ctx) string

// TokenAuthRequired는 토큰 기반 인증을 요구하는 미들웨어입니다
func TokenAuthRequired(permission string, categoryFunc CategoryPermissionFunc) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Authorization 헤더에서 토큰 추출
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{
				"error": "Missing authorization token",
				"code":  "AUTH_TOKEN_MISSING",
			})
		}

		// Bearer 토큰 형식 확인
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || strings.ToLower(tokenParts[0]) != "bearer" {
			return c.Status(401).JSON(fiber.Map{
				"error": "Invalid authorization format. Use: Bearer <token>",
				"code":  "AUTH_FORMAT_INVALID",
			})
		}

		token := tokenParts[1]

		// 토큰 검증
		claims, err := validateToken(token)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{
				"error": "Invalid or expired token",
				"code":  "AUTH_TOKEN_INVALID",
				"details": err.Error(),
			})
		}

		// 토큰 만료 확인
		if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
			return c.Status(401).JSON(fiber.Map{
				"error": "Token has expired",
				"code":  "AUTH_TOKEN_EXPIRED",
			})
		}

		// 카테고리별 권한 확인 (필요한 경우)
		if categoryFunc != nil {
			category := categoryFunc(c)
			if category != "" && !hasCategoryAccess(claims, category) {
				return c.Status(403).JSON(fiber.Map{
					"error": "Access denied to category: " + category,
					"code":  "AUTH_CATEGORY_DENIED",
				})
			}
		}

		// 권한 레벨 확인
		if !hasPermission(claims, permission) {
			return c.Status(403).JSON(fiber.Map{
				"error": "Insufficient permissions",
				"code":  "AUTH_PERMISSION_DENIED",
				"required": permission,
				"user_role": claims.Role,
			})
		}

		// 컨텍스트에 사용자 정보 저장
		c.Locals("user_id", claims.UserID)
		c.Locals("org_id", claims.OrgID)
		c.Locals("username", claims.Username)
		c.Locals("user_role", claims.Role)
		c.Locals("token_categories", claims.Categories)

		return c.Next()
	}
}

// validateToken은 토큰을 검증하고 클레임을 반환합니다
func validateToken(token string) (*TokenClaims, error) {
	// 데이터베이스에서 토큰 정보 조회
	db := database.GetDB()
	
	var claims TokenClaims
	var isActive bool
	var expiresAt *time.Time
	
	query := `
		SELECT 
			u.id, u.org_id, u.username, u.role,
			t.token_type, t.categories, t.expires_at, t.is_active
		FROM auth_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE t.token_hash = $1
	`
	
	err := db.QueryRow(query, hashToken(token)).Scan(
		&claims.UserID, &claims.OrgID, &claims.Username, &claims.Role,
		&claims.TokenType, &claims.Categories, &expiresAt, &isActive,
	)
	
	if err != nil {
		return nil, err
	}
	
	if !isActive {
		return nil, fiber.NewError(401, "Token has been disabled")
	}
	
	if expiresAt != nil {
		claims.ExpiresAt = expiresAt.Unix()
	}
	
	return &claims, nil
}

// hasPermission은 사용자가 필요한 권한을 가지고 있는지 확인합니다
func hasPermission(claims *TokenClaims, permission string) bool {
	switch permission {
	case "read":
		return claims.Role == "admin" || claims.Role == "viewer"
	case "write":
		return claims.Role == "admin" || claims.Role == "writer"
	case "admin":
		return claims.Role == "admin"
	default:
		return false
	}
}

// hasCategoryAccess는 특정 카테고리에 대한 접근 권한을 확인합니다
func hasCategoryAccess(claims *TokenClaims, category string) bool {
	// 관리자는 모든 카테고리 접근 가능
	if claims.Role == "admin" {
		return true
	}
	
	// 카테고리 제한이 없으면 모든 카테고리 접근 가능
	if len(claims.Categories) == 0 {
		return true
	}
	
	// 특정 카테고리 접근 권한 확인
	for _, allowedCategory := range claims.Categories {
		if allowedCategory == category || allowedCategory == "*" {
			return true
		}
	}
	
	return false
}

// hashToken은 토큰을 해싱합니다 (보안을 위해)
func hashToken(token string) string {
	// TODO: 실제 환경에서는 강력한 해싱 알고리즘 사용
	// 여기서는 기존 crypto 패키지 활용
	return database.HashPassword(token) // 기존 함수 재활용
}

// GetTokenClaims는 컨텍스트에서 토큰 클레임 정보를 가져옵니다
func GetTokenClaims(c *fiber.Ctx) *TokenClaims {
	return &TokenClaims{
		UserID:     c.Locals("user_id").(int),
		OrgID:      c.Locals("org_id").(int),
		Username:   c.Locals("username").(string),
		Role:       c.Locals("user_role").(string),
		Categories: c.Locals("token_categories").([]string),
	}
}

// GetOrgIDFromToken은 토큰에서 조직 ID를 가져옵니다 (기존 미들웨어와 호환성)
func GetOrgIDFromToken(c *fiber.Ctx) (int, error) {
	orgID := c.Locals("org_id")
	if orgID == nil {
		return 0, fiber.NewError(401, "Organization ID not found in token")
	}
	return orgID.(int), nil
} 