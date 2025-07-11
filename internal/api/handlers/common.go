package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// GetOrgIDFromContext safely retrieves org_id from fiber context
func GetOrgIDFromContext(c *fiber.Ctx) (string, error) {
	orgID, ok := c.Locals("org_id").(string)
	if !ok || orgID == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "organization ID is required")
	}
	return orgID, nil
}

// GetOrgIDFromContextOptional safely retrieves org_id from fiber context, returns empty string if not found
func GetOrgIDFromContextOptional(c *fiber.Ctx) string {
	orgID, ok := c.Locals("org_id").(string)
	if !ok {
		return ""
	}
	return orgID
}
