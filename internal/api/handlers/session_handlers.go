package handlers

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/tmidb/tmidb-core/internal/database"
)

// SetOrganizationInSession godoc
// @Summary Switch organization context in session
// @Description Stores the selected organization ID in the user's session.
// @Tags Session
// @Accept json
// @Produce json
// @Param org_id body object{org_id=string} true "Organization ID"
// @Success 200 {object} object{message=string}
// @Failure 400 {object} object{error=string}
// @Failure 403 {object} object{error=string}
// @Failure 500 {object} object{error=string}
// @Router /api/session/organization [post]
func SetOrganizationInSession(c *fiber.Ctx) error {
	var payload struct {
		OrgID string `json:"org_id"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	userID, ok := c.Locals("user_id").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: user_id not found in context"})
	}

	// Verify user is a member of the organization
	isMember, err := database.IsUserMemberOfOrg(database.GetDB(), userID, payload.OrgID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error while verifying membership"})
	}
	if !isMember {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not a member of this organization"})
	}

	org, err := database.GetOrganizationByID(payload.OrgID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get organization details"})
	}

	sess, err := c.Locals("session_store").(*session.Store).Get(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get session"})
	}

	sess.Set("org_id", payload.OrgID)
	sess.Set("org_name", org.Name)
	if err := sess.Save(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save session"})
	}

	return c.JSON(fiber.Map{"message": "Organization context switched successfully"})
}

// GetOrganizationFromSession godoc
// @Summary Get organization context from session
// @Description Retrieves the current organization ID and name from the user's session.
// @Tags Session
// @Produce json
// @Success 200 {object} object{org_id=string, org_name=string}
// @Failure 500 {object} object{error=string}
// @Router /api/session/organization [get]
func GetOrganizationFromSession(c *fiber.Ctx) error {
	sess, err := c.Locals("session_store").(*session.Store).Get(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get session"})
	}

	orgID := sess.Get("org_id")
	orgName := sess.Get("org_name")

	if orgID == nil {
		// If no org_id is in session, try to get the user's default org
		userID, ok := c.Locals("user_id").(string)
		if !ok || userID == "" {
			return c.JSON(fiber.Map{}) // Not logged in, return empty
		}
		user, err := database.GetUserByID(database.GetDB(), userID)
		if err != nil {
			log.Printf("could not get user by id %s: %v", userID, err)
			return c.JSON(fiber.Map{})
		}
		orgID = user.OrgID

		org, err := database.GetOrganizationByID(user.OrgID)
		if err != nil {
			log.Printf("could not get org by id %s: %v", user.OrgID, err)
			return c.JSON(fiber.Map{})
		}
		orgName = org.Name

		// Save the default org to the session for future requests
		sess.Set("org_id", orgID)
		sess.Set("org_name", orgName)
		sess.Save()
	}

	return c.JSON(fiber.Map{
		"org_id":   orgID,
		"org_name": orgName,
	})
}

// 세션 기반 토큰 반환
func GetSessionToken(c *fiber.Ctx) error {
	sess, err := c.Locals("session_store").(*session.Store).Get(c)
	log.Printf("[GetSessionToken] 세션: authenticated=%%v, user_id=%%v, org_id=%%v", sess.Get("authenticated"), sess.Get("user_id"), sess.Get("org_id"))
	if err != nil || sess.Get("authenticated") != true {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Not authenticated"})
	}
	userID, _ := sess.Get("user_id").(string)
	orgID, _ := sess.Get("org_id").(string)
	if userID == "" || orgID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session missing user info"})
	}
	token, _, err := database.CreateUserToken(userID, orgID, "Web session auto token")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to issue token"})
	}
	return c.JSON(fiber.Map{"token": token})
}
