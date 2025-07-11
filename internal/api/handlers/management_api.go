package handlers

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/tmidb/tmidb-core/internal/database"
)

// SwitchOrganization는 사용자의 현재 활성 조직을 세션에서 변경합니다.
func SwitchOrganization(c *fiber.Ctx) error {
	type OrgSwitchRequest struct {
		OrgID string `json:"org_id"`
	}

	var req OrgSwitchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	if req.OrgID == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "org_id is required",
		})
	}

	sess, err := c.Locals("session_store").(*session.Store).Get(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get session",
		})
	}
	sess.Set("org_id", req.OrgID)
	if err := sess.Save(); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to save session",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Organization switched successfully",
		"org_id":  req.OrgID,
	})
}

// GetUserTokensAPI는 현재 사용자의 모든 API 토큰을 반환합니다.
func GetUserTokensAPI(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	tokens, err := database.GetUserTokens(userID, orgID)
	if err != nil {
		log.Printf("[GetUserTokensAPI] userID=%s, orgID=%s, 토큰 조회 실패: %v", userID, orgID, err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    tokens,
	})
}

// CreateUserTokenAPI는 현재 사용자를 위한 새 API 토큰을 생성합니다.
func CreateUserTokenAPI(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	type TokenRequest struct {
		Description string `json:"description"`
	}

	var req TokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	if req.Description == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Description is required",
		})
	}

	tokenString, tokenInfo, err := database.CreateUserToken(userID, orgID, req.Description)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": map[string]interface{}{
			"token":      tokenString,
			"token_info": tokenInfo,
		},
	})
}

// DeleteUserTokenAPI는 현재 사용자의 특정 API 토큰을 삭제합니다.
func DeleteUserTokenAPI(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	tokenID := c.Params("id")

	if err := database.DeleteUserToken(tokenID, userID, orgID); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Token deleted successfully",
	})
}

// GetCategoriesForExplorerAPI는 데이터 탐색기용 카테고리 목록을 반환합니다.
func GetCategoriesForExplorerAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database",
			"details": err.Error(),
		})
	}
	defer orgDB.Close()

	categories, err := orgDB.GetCategories()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to retrieve categories",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    categories,
	})
}

// GetOrganizationsAPI는 모든 조직의 목록을 반환합니다. (관리자 전용)
func GetOrganizationsAPI(c *fiber.Ctx) error {
	organizations, err := database.GetOrganizations()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to retrieve organizations",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":       true,
		"organizations": organizations,
	})
}

// GetOrganizationAPI는 특정 조직의 상세 정보를 반환합니다. (관리자 전용)
func GetOrganizationAPI(c *fiber.Ctx) error {
	orgID := c.Params("org_id")
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	organization, err := database.GetOrganizationByID(orgID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   "Organization not found",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"organization": organization,
	})
}

// UpdateOrganizationAPI는 특정 조직의 정보를 업데이트합니다. (관리자 전용)
func UpdateOrganizationAPI(c *fiber.Ctx) error {
	orgID := c.Params("org_id")
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	type UpdateRequest struct {
		Name string `json:"name"`
	}
	var req UpdateRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Organization name cannot be empty",
		})
	}

	updatedOrg, err := database.UpdateOrganization(orgID, req.Name)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to update organization",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"organization": updatedOrg,
	})
}

// DeleteOrganizationAPI는 특정 조직을 삭제합니다. (관리자 전용)
func DeleteOrganizationAPI(c *fiber.Ctx) error {
	orgID := c.Params("org_id")
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	err := database.DeleteOrganization(orgID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to delete organization",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Organization deleted successfully",
	})
}
