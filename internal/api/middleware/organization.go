package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/tmidb/tmidb-core/internal/database"
)

// OrganizationContext 미들웨어: 헤더(API) 또는 세션(웹 콘솔)에서 org_id 추출
func OrganizationContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 이 미들웨어를 적용하지 않을 경로 목록
		excludedPaths := []string{
			"/login", "/logout", "/setup",
			"/api/login", "/api/logout", "/api/setup",
			"/api/session/organization", "/api/manage/organizations",
		}
		path := c.Path()
		for _, p := range excludedPaths {
			if strings.HasPrefix(path, p) {
				return c.Next()
			}
		}

		var orgID string

		// 1. 세션에서 org_id 가져오기 (웹 콘솔 우선)
		sess, err := c.Locals("session_store").(*session.Store).Get(c)
		if err == nil {
			if id, ok := sess.Get("org_id").(string); ok && id != "" {
				orgID = id
			}
		}

		// 2. 헤더에서 X-Organization-ID 가져오기 (API 클라이언트 fallback)
		if orgID == "" {
			orgID = c.Get("X-Organization-ID")
		}

		// org_id가 여전히 없으면 에러 반환
		if orgID == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Organization context is not set. Please select an organization or provide X-Organization-ID header."})
		}

		// 사용자 ID 가져오기
		userID, ok := c.Locals("user_id").(string)
		if !ok || userID == "" {
			// c.Locals("user")에서 꺼내기 시도
			if user, ok := c.Locals("user").(*database.User); ok && user.UserID != "" {
				userID = user.UserID
				c.Locals("user_id", userID)
			}
		}
		if userID == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: user_id not found in context"})
		}

		// 사용자가 해당 조직의 멤버인지 확인
		isMember, err := database.IsUserMemberOfOrg(database.GetDB(), userID, orgID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error while verifying membership"})
		}
		if !isMember {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not a member of this organization"})
		}

		// 컨텍스트에 org_id 설정
		c.Locals("org_id", orgID)

		return c.Next()
	}
}
