package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/tmidb/tmidb-core/internal/database"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
)

// 인증 관련 상수
const (
	HEADER_AUTHORIZATION = "Authorization"
	HEADER_BEARER_PREFIX = "Bearer "
	ADMIN_PERMISSION     = "admin"
)

// HashToken은 클라이언트가 보낸 토큰을 SHA256으로 해싱합니다.
func HashToken(token string) string {
	hasher := sha256.New()
	hasher.Write([]byte(token))
	return hex.EncodeToString(hasher.Sum(nil))
}

// UserTokenAuthRequired는 사용자 액세스 토큰의 유효성을 검사하는 미들웨어입니다.
// 토큰이 유효하면 사용자 ID, 조직 ID, 역할을 컨텍스트에 저장합니다.
func UserTokenAuthRequired() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get(HEADER_AUTHORIZATION)
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Authorization header is required"})
		}

		if !strings.HasPrefix(authHeader, HEADER_BEARER_PREFIX) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token format, must be Bearer token"})
		}

		token := strings.TrimPrefix(authHeader, HEADER_BEARER_PREFIX)
		tokenHash := HashToken(token)

		var userID, orgID, userRole string
		err := database.CoreDB.QueryRow(`
            SELECT u.user_id, u.org_id, u.role
            FROM users u
            JOIN user_access_tokens t ON u.user_id = t.user_id
            WHERE t.token_hash = $1 AND t.is_active = TRUE AND u.is_active = TRUE
        `, tokenHash).Scan(&userID, &orgID, &userRole)

		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired token"})
		}

		c.Locals("user_id", userID)
		c.Locals("org_id", orgID)
		c.Locals("role", userRole)

		return c.Next()
	}
}

// TokenAuthRequired는 API 요청에 대한 토큰 인증을 처리하는 미들웨어입니다.
func TokenAuthRequired(requiredPermission string, getCategory func(*fiber.Ctx) string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get(HEADER_AUTHORIZATION)
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Authorization header is required"})
		}

		if !strings.HasPrefix(authHeader, HEADER_BEARER_PREFIX) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token format, must be Bearer token"})
		}

		token := strings.TrimPrefix(authHeader, HEADER_BEARER_PREFIX)
		tokenHash := HashToken(token)

		var categoryName string
		if getCategory != nil {
			categoryName = getCategory(c)
		}

		var hasPermission bool
		err := database.CoreDB.QueryRow("SELECT verify_token($1, $2, $3)", tokenHash, requiredPermission, categoryName).Scan(&hasPermission)
		if err != nil || !hasPermission {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Permission denied"})
		}

		return c.Next()
	}
}

// VerifyTokenForLogin은 로그인 시 토큰을 검증합니다.
func VerifyTokenForLogin(token string) (bool, error) {
	tokenHash := HashToken(token)
	var hasPermission bool
	err := database.CoreDB.QueryRow("SELECT verify_token($1, 'admin', NULL)", tokenHash).Scan(&hasPermission)
	return hasPermission, err
}

// AuthRequired는 인증이 필요한 경로를 보호하는 미들웨어입니다.
func AuthRequired(store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 개발 중에는 세션 체크 무시하고 무조건 통과
		// TODO: 프로덕션 배포 시 이 부분 제거 필요
		// return c.Next() // 이 줄을 제거하여 실제 인증 로직이 동작하도록 함

		sess, err := store.Get(c)
		if err != nil {
			return c.Redirect("/login")
		}

		// 세션 전체 key-value를 보기 쉽게 로그로 출력
		if sess != nil {
			var sessPairs []string
			for _, k := range sess.Keys() {
				v := sess.Get(k)
				sessPairs = append(sessPairs, fmt.Sprintf("%s=%v", k, v))
			}
			fmt.Printf("[AuthRequired] session: %s\n", strings.Join(sessPairs, ", "))
		}

		if sess.Get("authenticated") != true {
			return c.Redirect("/login")
		}

		// 세션에서 사용자 정보 가져오기
		userID := sess.Get("user_id")
		orgID := sess.Get("org_id")
		role := sess.Get("role")

		var usernameStr string
		if userID != nil {
			// user_id가 있으면 DB에서 username 조회
			userObj, err := database.GetUserByID(database.GetDB(), userID.(string))
			if err == nil && userObj != nil {
				usernameStr = userObj.Username
			} else {
				usernameStr = "Guest"
			}
		} else {
			usernameStr = "Guest"
		}
		userOrgID := "default"
		if orgID != nil {
			userOrgID = orgID.(string)
		}
		userRole := "viewer" // 기본 역할은 viewer로 설정
		if role != nil {
			userRole = role.(string)
		}

		// database.User 구조체는 Name 필드가 없으므로 Username을 사용합니다.
		user := &database.User{
			UserID:   userID.(string),
			OrgID:    userOrgID,
			Role:     userRole,
			Username: usernameStr, // 반드시 DB에서 조회한 username만 사용
		}
		c.Locals("user", user)
		c.Locals("user_id", userID.(string))
		c.Locals("org_id", userOrgID)
		c.Locals("role", userRole)

		return c.Next()
	}
}

// AdminRequired는 관리자 권한이 필요한 경로를 보호하는 미들웨어입니다.
func AdminRequired(store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 개발 중에는 권한 체크 무시하고 무조건 통과
		// TODO: 프로덕션 배포 시 이 부분 제거 필요
		// return c.Next() // 이 줄을 제거하여 실제 인증 로직이 동작하도록 함

		sess, err := store.Get(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		if sess.Get("authenticated") != true {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		role := sess.Get("role")
		if role != "admin" {
			return c.Status(fiber.StatusForbidden).SendString("Admin privileges required")
		}

		return c.Next()
	}
}

// GetUserID는 세션에서 사용자 ID를 가져옵니다.
func GetUserID(c *fiber.Ctx, store *session.Store) (string, error) {
	sess, err := store.Get(c)
	if err != nil {
		return "", err
	}

	if sess.Get("authenticated") != true {
		return "", fiber.NewError(fiber.StatusUnauthorized, "Not authenticated")
	}

	userID := sess.Get("user_id")
	if userID == nil {
		return "", fiber.NewError(fiber.StatusUnauthorized, "User ID not found in session")
	}

	return userID.(string), nil
}

// GetUserRole은 세션에서 사용자 역할을 가져옵니다.
func GetUserRole(c *fiber.Ctx, store *session.Store) (string, error) {
	sess, err := store.Get(c)
	if err != nil {
		return "", err
	}

	if sess.Get("authenticated") != true {
		return "", fiber.NewError(fiber.StatusUnauthorized, "Not authenticated")
	}

	role := sess.Get("role")
	if role == nil {
		return "", fiber.NewError(fiber.StatusUnauthorized, "User role not found in session")
	}

	return role.(string), nil
}

// IsAuthenticated는 현재 사용자가 인증되었는지 확인합니다.
func IsAuthenticated(c *fiber.Ctx, store *session.Store) bool {
	sess, err := store.Get(c)
	if err != nil {
		return false
	}

	authenticated := sess.Get("authenticated")
	return authenticated == true
}

// IsAdmin은 현재 사용자가 관리자인지 확인합니다.
func IsAdmin(c *fiber.Ctx, store *session.Store) bool {
	role, err := GetUserRole(c, store)
	if err != nil {
		return false
	}
	return role == "admin"
}

// AdminTokenRequired는 토큰 기반 요청에 대해 관리자 권한을 확인하는 미들웨어입니다.
// 반드시 UserTokenAuthRequired 뒤에 사용해야 합니다.
func AdminTokenRequired() fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, ok := c.Locals("role").(string)
		if !ok || role != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Admin privileges required"})
		}
		return c.Next()
	}
}

// AdminRoleRequired는 API 토큰을 사용하여 관리자 역할 여부를 확인하는 미들웨어입니다.
func AdminRoleRequired() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get(HEADER_AUTHORIZATION)
		if authHeader == "" || !strings.HasPrefix(authHeader, HEADER_BEARER_PREFIX) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Authorization header is required"})
		}

		token := strings.TrimPrefix(authHeader, HEADER_BEARER_PREFIX)
		tokenHash := HashToken(token)

		var role string
		err := database.CoreDB.QueryRow(`
			SELECT role FROM auth_tokens WHERE token_hash = $1
		`, tokenHash).Scan(&role)

		if err != nil || role != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Admin role required"})
		}

		return c.Next()
	}
}

// GetOrgID는 세션에서 현재 사용자의 조직 ID를 반환합니다.
func GetOrgID(c *fiber.Ctx) (string, error) {
	store := c.Locals("session_store").(*session.Store)
	sess, err := store.Get(c)
	if err != nil {
		return "", fmt.Errorf("failed to get session")
	}

	orgID := sess.Get("org_id")
	if orgID == nil {
		return "", fmt.Errorf("org_id not found in session")
	}

	return orgID.(string), nil
}

// SessionOrTokenAuthRequired: 세션이 있으면 세션 인증, 없으면 토큰 인증
func SessionOrTokenAuthRequired(store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err == nil && sess.Get("authenticated") == true {
			userID, ok1 := sess.Get("user_id").(string)
			orgID, ok2 := sess.Get("org_id").(string)
			role, ok3 := sess.Get("role").(string)
			if ok1 && ok2 && ok3 && userID != "" && orgID != "" && role != "" {
				c.Locals("user_id", userID)
				c.Locals("org_id", orgID)
				c.Locals("role", role)
				return c.Next()
			}
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session missing user info"})
		}
		// 2. 토큰 인증 시도 (UserTokenAuthRequired와 동일)
		authHeader := c.Get(HEADER_AUTHORIZATION)
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Authorization header is required"})
		}
		if !strings.HasPrefix(authHeader, HEADER_BEARER_PREFIX) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token format, must be Bearer token"})
		}
		token := strings.TrimPrefix(authHeader, HEADER_BEARER_PREFIX)
		tokenHash := HashToken(token)
		var userID, orgID, userRole string
		err = database.CoreDB.QueryRow(`SELECT u.user_id, u.org_id, u.role FROM users u JOIN user_access_tokens t ON u.user_id = t.user_id WHERE t.token_hash = $1 AND t.is_active = TRUE AND u.is_active = TRUE`, tokenHash).Scan(&userID, &orgID, &userRole)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired token"})
		}
		c.Locals("user_id", userID)
		c.Locals("org_id", orgID)
		c.Locals("role", userRole)
		return c.Next()
	}
}
