package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/tmidb/tmidb-core/internal/config"
	"github.com/tmidb/tmidb-core/internal/database"
)

// User 구조체 정의
type User struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	OrgID    string `json:"org_id"`
	Name     string `json:"name"`
}

// CheckSetupStatus는 초기 설정 완료 여부를 확인합니다.
func CheckSetupStatus() (bool, error) {
	return database.IsSetupCompleted()
}

// LoginPage는 로그인 페이지를 렌더링합니다.
func LoginPage(c *fiber.Ctx) error {
	// 세션을 안전하게 가져옵니다.
	sessStore, ok := c.Locals("session_store").(*session.Store)
	if !ok || sessStore == nil {
		// 세션 스토어가 없으면 바로 로그인 페이지를 렌더링합니다.
		// 이 경우 500 오류 대신 정상적으로 페이지가 표시됩니다.
		log.Println("Warning: session_store not found in context for /login. Rendering login page directly.")
		return c.Render("login", fiber.Map{
			"Title": "로그인 - tmiDB",
		})
	}

	sess, err := sessStore.Get(c)
	if err == nil && sess.Get("authenticated") == true {
		return c.Redirect("/dashboard")
	}

	return c.Render("login", fiber.Map{
		"Title": "로그인 - tmiDB",
	})
}

// LoginProcess는 로그인 처리를 담당합니다.
func LoginProcess(c *fiber.Ctx) error {
	var loginData struct {
		Username string `json:"username" form:"username"`
		Password string `json:"password" form:"password"`
	}

	isJSONRequest := strings.Contains(c.Get("Content-Type"), "application/json")

	// 요청 본문을 파싱 (JSON 또는 form-urlencoded)
	if err := c.BodyParser(&loginData); err != nil {
		if isJSONRequest {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request data"})
		}
		return c.Redirect("/login")
	}

	// 입력값 검증
	if loginData.Username == "" || loginData.Password == "" {
		if isJSONRequest {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "사용자명과 비밀번호를 입력하세요"})
		}
		return c.Redirect("/login")
	}

	// 사용자 인증
	userID, orgID, role, err := database.AuthenticateUser(loginData.Username, loginData.Password)
	if err != nil {
		log.Printf("[LoginProcess] 로그인 실패: %v (username=%s)", err, loginData.Username)
		if isJSONRequest {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "로그인 실패: " + err.Error()})
		}
		return c.Redirect("/login")
	}

	// 세션에 사용자 정보 저장 (api_token 저장 제거)
	sess, err := c.Locals("session_store").(*session.Store).Get(c)
	if err != nil {
		if isJSONRequest {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "세션 생성 실패"})
		}
		return c.Redirect("/login")
	}

	sess.Set("authenticated", true)
	sess.Set("user_id", userID)
	sess.Set("org_id", orgID)
	sess.Set("role", role)
	log.Printf("[LoginProcess] 세션 저장: authenticated=%%v, user_id=%%v, org_id=%%v, role=%%v", true, userID, orgID, role)
	// api_token 저장 제거
	if err := sess.Save(); err != nil {
		if isJSONRequest {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "세션 저장 실패"})
		}
		return c.Redirect("/login")
	}

	// 응답
	if isJSONRequest {
		// 로그인 성공 시 토큰 발급
		token := ""
		accessToken := ""
		// 사용자 토큰 발급 함수가 있으면 호출
		if userID != "" && orgID != "" {
			t, _, err := database.CreateUserToken(userID, orgID, "Web login session token")
			if err == nil {
				token = t
				accessToken = t
			}
		}
		return c.JSON(fiber.Map{
			"success":      true,
			"message":      "로그인 성공",
			"redirect":     "/dashboard",
			"token":        token,
			"access_token": accessToken,
		})
	}

	return c.Redirect("/dashboard")
}

// LogoutPage는 로그아웃 처리를 담당합니다.
func LogoutPage(c *fiber.Ctx) error {
	// 세션 삭제
	sess, err := c.Locals("session_store").(*session.Store).Get(c)
	if err == nil {
		sess.Destroy()
	}
	return c.Redirect("/login")
}

// SetupPage는 초기 설정 페이지를 렌더링합니다.
func SetupPage(c *fiber.Ctx) error {
	// 이미 설정이 완료되었는지 확인
	completed, err := database.IsSetupCompleted()
	if err != nil {
		// 데이터베이스 오류가 발생하면 에러 페이지 또는 로그를 남길 수 있습니다.
		// 여기서는 간단히 500 에러를 반환합니다.
		return c.Status(fiber.StatusInternalServerError).SendString("Database error checking setup status")
	}

	if completed {
		// 설정이 완료되었다면 로그인 페이지로 리디렉션
		return c.Redirect("/login")
	}

	return c.Render("setup", fiber.Map{
		"Title": "초기 설정 - tmiDB",
	})
}

// SetupProcess는 초기 설정 처리를 담당합니다.
func SetupProcess(c *fiber.Ctx) error {
	// 1. 요청 본문 파싱
	type setupRequest struct {
		AdminUsername    string `json:"admin_username"`
		AdminPassword    string `json:"admin_password"`
		OrganizationName string `json:"organization_name"`
	}

	req := new(setupRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   fiber.Map{"message": "Invalid request body: " + err.Error()},
		})
	}

	// 2. 입력값 검증
	if req.AdminUsername == "" || req.AdminPassword == "" || req.OrganizationName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   fiber.Map{"message": "All fields are required"},
		})
	}

	// 3. 데이터베이스 및 초기 설정 로직 호출
	cfg, err := config.Load()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   fiber.Map{"message": "Failed to load configuration: " + err.Error()},
		})
	}

	// 4. 조직 데이터베이스 생성 및 관리자 생성 로직 실행
	orgID, adminUserID, err := database.CreateOrganizationDatabase(req.OrganizationName, req.AdminUsername, req.AdminPassword, cfg)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   fiber.Map{"message": "Failed to complete setup: " + err.Error()},
		})
	}

	// 5. 새로 생성된 관리자 사용자를 위한 액세스 토큰 생성
	accessToken, _, err := database.CreateUserToken(adminUserID, orgID, "Initial admin setup token")
	if err != nil {
		// 토큰 생성 실패 시 생성된 조직과 데이터베이스 롤백
		log.Printf("❌ Token creation failed, rolling back organization '%s'", req.OrganizationName)

		// 조직 데이터베이스 삭제
		dbName := fmt.Sprintf("tmidb_%s", strings.ToLower(strings.ReplaceAll(req.OrganizationName, " ", "_")))
		if rollbackErr := database.RollbackOrganizationCreation(orgID, dbName, cfg); rollbackErr != nil {
			log.Printf("❌ Failed to rollback organization creation: %v", rollbackErr)
		} else {
			log.Printf("✅ Successfully rolled back organization '%s'", req.OrganizationName)
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   fiber.Map{"message": "Failed to create access token: " + err.Error()},
		})
	}

	// 6. 성공 응답 반환
	log.Printf("✅ Initial setup completed successfully. Admin user for organization '%s' created.", req.OrganizationName)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success":         true,
		"message":         "Setup completed successfully",
		"organization_id": orgID,
		"user_id":         adminUserID,
		"admin_token":     accessToken,
	})
}

// SetupStatus는 설정 상태를 반환합니다.
func SetupStatus(c *fiber.Ctx) error {
	completed, err := database.IsSetupCompleted()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"setup_completed": completed})
}

// DashboardPage는 대시보드 페이지를 렌더링합니다.
func DashboardPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		log.Printf("Error populating base data for dashboard: %v", err)
		return c.Redirect("/login") // Redirect to login on error
	}
	data["Title"] = "대시보드"
	data["PageTitle"] = "시스템 대시보드"
	data["CurrentPage"] = "dashboard"
	return renderPage(c, "dashboard", data)
}

// CategoriesPage는 카테고리 관리 페이지를 렌더링합니다.
func CategoriesPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["Title"] = "카테고리 관리"
	data["PageTitle"] = "카테고리 관리"
	data["CurrentPage"] = "categories"
	return renderPage(c, "categories", data)
}

// ListenersPage는 리스너 관리 페이지를 렌더링합니다.
func ListenersPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["Title"] = "리스너 관리"
	data["PageTitle"] = "리스너 관리"
	data["CurrentPage"] = "listeners"
	return renderPage(c, "listeners", data)
}

// DataExplorerPage는 데이터 탐색기 페이지를 렌더링합니다.
func DataExplorerPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["Title"] = "데이터 탐색기"
	data["PageTitle"] = "데이터 탐색기"
	data["CurrentPage"] = "data-explorer"
	return renderPage(c, "data-explorer", data)
}

// FilesPage는 파일 관리 페이지를 렌더링합니다.
func FilesPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["Title"] = "파일 관리"
	data["PageTitle"] = "파일 관리"
	data["CurrentPage"] = "files"
	return renderPage(c, "files", data)
}

// UsersPage는 사용자 관리 페이지를 렌더링합니다.
func UsersPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["Title"] = "사용자 관리"
	data["PageTitle"] = "사용자 관리"
	data["CurrentPage"] = "users"
	return renderPage(c, "users", data)
}

// TokensPage는 API 토큰 관리 페이지를 렌더링합니다.
func TokensPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["Title"] = "API 토큰 관리"
	data["PageTitle"] = "API 토큰 관리"
	data["CurrentPage"] = "tokens"
	return renderPage(c, "tokens", data)
}

// MigrationsPage는 마이그레이션 관리 페이지를 렌더링합니다.
func MigrationsPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["Title"] = "마이그레이션 관리"
	data["PageTitle"] = "마이그레이션 관리"
	data["CurrentPage"] = "migrations"
	return renderPage(c, "migrations", data)
}

// LogsPage는 로그 관리 페이지를 렌더링합니다.
func LogsPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["Title"] = "로그 관리"
	data["PageTitle"] = "로그 관리"
	data["CurrentPage"] = "logs"
	return renderPage(c, "logs", data)
}

// TestPage는 테스트 페이지를 렌더링합니다.
func TestPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["Title"] = "테스트 페이지"
	data["PageTitle"] = "테스트 페이지"
	data["CurrentPage"] = "test"
	return renderPage(c, "test", data)
}

// OrganizationPage는 조직 상세 페이지를 렌더링합니다.
func OrganizationPage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error populating page data: " + err.Error()})
	}
	data["CurrentPage"] = "organization"
	data["Title"] = "조직 관리"
	return renderPage(c, "organization", data)
}

// OrganizationCreatePage는 새 조직 생성 페이지를 렌더링합니다.
func OrganizationCreatePage(c *fiber.Ctx) error {
	data, err := populateBaseData(c)
	if err != nil {
		return c.Status(500).SendString("Error populating page data: " + err.Error())
	}
	data["CurrentPage"] = "organization-create"
	data["Title"] = "새 조직 생성"
	return renderPage(c, "organization-create", data)
}

// 헬퍼 함수들

// getUserFromSession은 세션에서 사용자 정보를 가져옵니다.
func getUserFromSession(c *fiber.Ctx) User {
	sess, err := c.Locals("session_store").(*session.Store).Get(c)
	if err != nil {
		// 세션 오류 시 기본값 반환
		return User{
			Username: "admin",
			Role:     "admin",
			OrgID:    "default",
		}
	}

	userID := sess.Get("user_id")
	role := sess.Get("role")
	orgID := sess.Get("org_id")

	username := "admin"
	if userID != nil {
		username = userID.(string)
	}

	userRole := "admin"
	if role != nil {
		userRole = role.(string)
	}

	userOrgID := "default"
	if orgID != nil {
		userOrgID = orgID.(string)
	}

	return User{
		Username: username,
		Role:     userRole,
		OrgID:    userOrgID,
	}
}

// populateBaseData는 모든 페이지에 필요한 기본 데이터를 채웁니다.
func populateBaseData(c *fiber.Ctx) (fiber.Map, error) {
	data := fiber.Map{}
	data["Title"] = "tmiDB"

	// Get user from context
	user, ok := c.Locals("user").(*database.User)
	if !ok {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "User not found in context.")
	}
	// 로그 추가: user_id, username, role, org_id
	log.Printf("[populateBaseData] user_id: %v, username: %v, role: %v, org_id: %v", user.UserID, user.Username, user.Role, user.OrgID)
	data["User"] = user

	// Get current organization from context or user's default
	orgID, _ := c.Locals("org_id").(string)
	if orgID == "" {
		orgID = user.OrgID
	}
	log.Printf("[populateBaseData] orgID: %v", orgID)

	currentOrg, err := database.GetOrganizationByID(orgID)
	if err != nil {
		log.Printf("Warning: could not get organization data for orgID '%s': %v", orgID, err)
		// Fallback or error out
		return nil, fiber.NewError(fiber.StatusForbidden, "Could not load organization data.")
	}
	data["Organization"] = currentOrg

	// Get all organizations for the user for the switcher dropdown
	orgs, err := database.GetOrganizationsForUser(database.GetDB(), user.UserID)
	if err != nil {
		log.Printf("Warning: could not get organizations for user %s: %v", user.UserID, err)
		orgs = []database.OrganizationInfo{} // Return empty list on error
	}
	data["Organizations"] = orgs

	// Get API Token from context if available
	if apiToken, ok := c.Locals("api_token").(string); ok {
		data["APIToken"] = apiToken
	}

	return data, nil
}

// renderPage는 공통 페이지 렌더링 로직을 처리합니다.
func renderPage(c *fiber.Ctx, templateName string, data fiber.Map) error {
	return c.Render(templateName, data)
}
