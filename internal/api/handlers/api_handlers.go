package handlers

import (
	"encoding/json"
	"log" // Added log import
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2" // uuid 패키지 임포트
	_ "github.com/lib/pq"         // PostgreSQL 드라이버
	"github.com/tmidb/tmidb-core/internal/config"
	"github.com/tmidb/tmidb-core/internal/database"
	"github.com/tmidb/tmidb-core/internal/migration"
)

// Dashboard API Handlers

// DashboardMetrics는 대시보드 메트릭을 반환합니다.
func DashboardMetrics(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)

	// TODO: 실제 메트릭 데이터 조회
	metrics := map[string]interface{}{
		"targets_count":    42,
		"categories_count": 8,
		"listeners_count":  3,
		"data_points":      15420,
		"storage_used":     "2.3 GB",
		"cpu_usage":        25.4,
		"memory_usage":     68.2,
		"disk_usage":       45.1,
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    metrics,
		"org_id":  orgID,
	})
}

// DashboardActivities는 최근 활동을 반환합니다.
func DashboardActivities(c *fiber.Ctx) error {
	// TODO: 실제 활동 로그 조회
	activities := []map[string]interface{}{
		{
			"id":        1,
			"type":      "data_insert",
			"message":   "새로운 vital 데이터가 추가되었습니다",
			"timestamp": "2025-01-02T10:30:00Z",
			"user":      "system",
		},
		{
			"id":        2,
			"type":      "category_created",
			"message":   "새 카테고리 'patient_info'가 생성되었습니다",
			"timestamp": "2025-01-02T09:15:00Z",
			"user":      "admin",
		},
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    activities,
	})
}

// DashboardResources는 시스템 리소스 정보를 반환합니다.
func DashboardResources(c *fiber.Ctx) error {
	// TODO: 실제 시스템 리소스 조회
	resources := map[string]interface{}{
		"cpu": map[string]interface{}{
			"cores":         4,
			"usage_percent": 25.4,
			"frequency":     "2.4 GHz",
		},
		"memory": map[string]interface{}{
			"total_gb":      16,
			"used_gb":       10.9,
			"usage_percent": 68.2,
		},
		"disk": map[string]interface{}{
			"total_gb":      500,
			"used_gb":       225.5,
			"usage_percent": 45.1,
		},
		"network": map[string]interface{}{
			"rx_mbps": 12.3,
			"tx_mbps": 8.7,
		},
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    resources,
	})
}

// DashboardApiStats는 API 통계를 반환합니다.
func DashboardApiStats(c *fiber.Ctx) error {
	// TODO: 실제 API 통계 조회
	stats := map[string]interface{}{
		"total_requests":    15420,
		"requests_per_hour": 245,
		"avg_response_time": "45ms",
		"error_rate":        "0.2%",
		"top_endpoints": []map[string]interface{}{
			{"path": "/api/v1/category/vital", "count": 3240},
			{"path": "/api/v1/targets", "count": 2180},
			{"path": "/api/v1/listener/dashboard", "count": 1950},
		},
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    stats,
	})
}

// SystemCheck는 시스템 상태를 확인합니다.
func SystemCheck(c *fiber.Ctx) error {
	// TODO: 실제 시스템 체크 로직
	checkResult := map[string]interface{}{
		"database":  "healthy",
		"nats":      "healthy",
		"seaweedfs": "healthy",
		"api":       "healthy",
		"overall":   "healthy",
		"timestamp": "2025-01-02T10:30:00Z",
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    checkResult,
	})
}

// ClearCache는 캐시를 지웁니다.
func ClearCache(c *fiber.Ctx) error {
	// TODO: 실제 캐시 클리어 로직
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Cache cleared successfully",
	})
}

// Category API Handlers

// GetCategoriesAPI는 카테고리 목록을 반환합니다.
func GetCategoriesAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		log.Printf("[GetCategoriesAPI] orgID=%s, DB 연결 실패: %v", orgID, err)
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to connect to organization database: " + err.Error()})
	}
	defer orgDB.Close()

	categories, err := orgDB.GetCategories()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"categories": categories,
	})
}

// CreateCategoryAPI는 새 카테고리를 생성합니다.
func CreateCategoryAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to connect to organization database: " + err.Error()})
	}
	defer orgDB.Close()

	// 프론트엔드에서 보내는 데이터 구조에 맞게 수정
	var req struct {
		CategoryName     string `json:"category_name"`
		Version          int    `json:"version"`
		IsActive         bool   `json:"is_active"`
		IsTimeseries     bool   `json:"is_timeseries"`
		SchemaDefinition string `json:"schema_definition"` // Frontend sends a JSON string
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request body: " + err.Error()})
	}

	// 카테고리 이름 유효성 검사
	if req.CategoryName == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Category name cannot be empty"})
	}

	// 스키마 정의가 유효한 JSON인지 확인 (선택 사항이지만 안전을 위해)
	var tempSchema map[string]interface{}
	if err := json.Unmarshal([]byte(req.SchemaDefinition), &tempSchema); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid schema definition JSON: " + err.Error()})
	}

	// 스키마 유효성 검증
	// ValidateSchemaData 함수는 []byte를 받으므로 req.SchemaDefinition을 []byte로 변환하여 전달
	if err := orgDB.ValidateSchemaData([]byte(req.SchemaDefinition)); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid schema data: " + err.Error()})
	}

	category := database.CategorySchema{
		OrgID:            orgID, // c.Locals에서 가져온 orgID 설정
		CategoryName:     req.CategoryName,
		Version:          req.Version,
		IsActive:         req.IsActive,
		IsTimeseries:     req.IsTimeseries,
		SchemaDefinition: req.SchemaDefinition,
	}

	if err := orgDB.CreateCategory(&category); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to create category: " + err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"success": true, "message": "Category created successfully", "data": category})
}

// UpdateCategoryAPI는 카테고리를 업데이트합니다.
func UpdateCategoryAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	categoryName := c.Params("name") // URL 파라미터에서 카테고리 이름 가져옴

	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to connect to organization database: " + err.Error()})
	}
	defer orgDB.Close()

	var req struct {
		// CategoryName은 URL 파라미터에서 가져오므로 여기서는 필요 없음
		Version          int    `json:"version"`
		IsActive         bool   `json:"is_active"`
		IsTimeseries     bool   `json:"is_timeseries"`
		SchemaDefinition string `json:"schema_definition"` // Frontend sends a JSON string
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body: " + err.Error()})
	}

	// 스키마 정의가 유효한 JSON인지 확인
	var tempSchema map[string]interface{}
	if err := json.Unmarshal([]byte(req.SchemaDefinition), &tempSchema); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid schema definition JSON: " + err.Error()})
	}

	// 스키마 유효성 검증
	if err := orgDB.ValidateSchemaData([]byte(req.SchemaDefinition)); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid schema data: " + err.Error()})
	}

	category := database.CategorySchema{
		OrgID:            orgID,        // c.Locals에서 가져온 orgID 설정
		CategoryName:     categoryName, // URL 파라미터에서 가져온 이름 사용
		Version:          req.Version,
		IsActive:         req.IsActive,
		IsTimeseries:     req.IsTimeseries,
		SchemaDefinition: req.SchemaDefinition,
	}

	if err := orgDB.UpdateCategory(&category); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to update category: " + err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Category updated successfully", "data": category})
}

// DeleteCategoryAPI는 카테고리를 삭제합니다.
func DeleteCategoryAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	categoryName := c.Params("name")

	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to connect to organization database: " + err.Error()})
	}
	defer orgDB.Close()

	if err := orgDB.DeleteCategory(categoryName); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Category deleted successfully",
	})
}

// GetCategorySchemaAPI는 카테고리 스키마를 반환합니다.
func GetCategorySchemaAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	categoryName := c.Params("name")

	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to connect to organization database: " + err.Error()})
	}
	defer orgDB.Close()

	schema, err := orgDB.GetCategorySchema(categoryName)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"error":   "Category schema not found",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    schema,
	})
}

// Listener API Handlers

// GetListenersAPI는 리스너 목록을 반환합니다.
func GetListenersAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to connect to organization database: " + err.Error()})
	}
	defer orgDB.Close()

	listeners, err := orgDB.GetListeners()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    listeners,
	})
}

// CreateListenerAPI는 새 리스너를 생성합니다.
func CreateListenerAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to connect to organization database: " + err.Error()})
	}
	defer orgDB.Close()

	var listener database.Listener
	if err := c.BodyParser(&listener); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	listener.OrgID = orgID
	// listener.ListenerID는 입력값이 있으면 사용, 없으면 아예 전달하지 않음(빈 값도 전달하지 않음)
	// DB에서 자동 생성되므로 uuid.NewString() 호출 제거
	log.Printf("Attempting to create listener. Category: %s, Org: %s", listener.CategoryName, listener.OrgID)

	if err := orgDB.CreateListener(&listener); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    listener,
	})
}

// DeleteListenerAPI는 리스너를 삭제합니다.
func DeleteListenerAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	listenerID := c.Params("id")

	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to connect to organization database: " + err.Error()})
	}
	defer orgDB.Close()

	if err := orgDB.DeleteListener(listenerID); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Listener deleted successfully",
	})
}

// User Management API Handlers

// GetUsersAPI는 사용자 목록을 반환합니다.
func GetUsersAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	users, err := database.GetUsers(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    users,
	})
}

// CreateUserAPI는 새 사용자를 생성합니다.
func CreateUserAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	var user database.User
	if err := c.BodyParser(&user); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	user.OrgID = orgID

	createdUser, err := database.CreateUser(user)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    createdUser,
	})
}

// UpdateUserAPI는 사용자를 업데이트합니다.
func UpdateUserAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	userID := c.Params("id")

	var user database.User
	if err := c.BodyParser(&user); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	user.UserID = userID
	user.OrgID = orgID

	updatedUser, err := database.UpdateUser(user)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    updatedUser,
	})
}

// DeleteUserAPI는 사용자를 삭제합니다.
func DeleteUserAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	userID := c.Params("id")

	if err := database.DeleteUser(userID, orgID); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "User deleted successfully",
	})
}

// Auth Token API Handlers

// GetAuthTokensAPI는 인증 토큰 목록을 반환합니다.
func GetAuthTokensAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	tokens, err := database.GetAuthTokens(orgID)
	if err != nil {
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

// CreateAuthTokenAPI는 새 인증 토큰을 생성합니다.
func CreateAuthTokenAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	user, _ := c.Locals("user").(*database.User)

	type TokenRequest struct {
		Description string     `json:"description"`
		IsAdmin     bool       `json:"is_admin"`
		ExpiresAt   *time.Time `json:"expires_at"`
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

	token, err := database.GenerateAndSaveAuthToken(database.CoreDB, orgID, req.Description, req.IsAdmin, user.UserID, "api", req.ExpiresAt)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"token":   token,
	})
}

// DeleteAuthTokenAPI는 인증 토큰을 삭제합니다.
func DeleteAuthTokenAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	tokenID := c.Params("id")

	if err := database.DeleteAuthToken(tokenID, orgID); err != nil {
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

// Migration API Handlers

// GetMigrationsAPI는 마이그레이션 목록을 반환합니다.
func GetMigrationsAPI(c *fiber.Ctx) error {
	// Get orgID from locals (assuming middleware sets it)
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	// Get a database connection for the organization
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		log.Printf("[GetMigrationsAPI] orgID=%s, DB 연결 실패: %v", orgID, err)
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to connect to organization database: " + err.Error()})
	}
	defer orgDB.Close()

	// Create a MigrationManager with the organization's DB connection
	mgr := migration.NewMigrationManager(orgDB.DB)

	// Get query parameters for filtering and limiting
	category := c.Query("category")
	status := c.Query("status")
	limit := c.QueryInt("limit", 0) // 0 means no limit

	// Call the actual GetMigrations function
	migrations, err := mgr.GetMigrations(category, status, limit)
	if err != nil {
		log.Printf("[GetMigrationsAPI] 마이그레이션 조회 실패: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to retrieve migrations: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    migrations,
	})
}

// CreateMigrationAPI는 새 마이그레이션을 생성합니다.
func CreateMigrationAPI(c *fiber.Ctx) error {
	var req struct {
		CategoryName    string  `json:"category_name"`
		FromVersion     float64 `json:"from_version"`
		ToVersion       float64 `json:"to_version"`
		MigrationScript string  `json:"migration_script"`
		Description     string  `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request body: " + err.Error()})
	}
	if req.CategoryName == "" || req.MigrationScript == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "category_name, migration_script는 필수입니다."})
	}
	mgr := migration.NewMigrationManager(database.GetDB())
	mig := &migration.Migration{
		Name:         req.CategoryName + "_v" + strconv.Itoa(int(req.FromVersion)) + "_to_v" + strconv.Itoa(int(req.ToVersion)),
		CategoryName: req.CategoryName,
		FromVersion:  req.FromVersion,
		ToVersion:    req.ToVersion,
		Script:       req.MigrationScript,
		Type:         "script",
		Description:  req.Description,
	}
	if err := mgr.CreateMigration(mig); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": mig})
}

// ExecuteMigrationAPI는 마이그레이션을 실행합니다.
func ExecuteMigrationAPI(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid migration id"})
	}
	mgr := migration.NewMigrationManager(database.GetDB())
	result, err := mgr.ExecuteMigration(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error(), "result": result})
	}
	return c.JSON(fiber.Map{"success": true, "data": result})
}

// GetMigrationStatusAPI는 마이그레이션 상태를 반환합니다.
func GetMigrationStatusAPI(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid migration id"})
	}
	mgr := migration.NewMigrationManager(database.GetDB())
	mig, err := mgr.GetMigrationByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": mig})
}

// Health Check and System Info

// HealthCheck는 API 서버 상태를 확인합니다.
func HealthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": true,
		"status":  "healthy",
		"version": "1.0.0",
		"time":    "2025-01-02T10:30:00Z",
	})
}

// SystemInfo는 시스템 정보를 반환합니다.
func SystemInfo(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": true,
		"data": map[string]interface{}{
			"version":     "1.0.0",
			"build_time":  "2025-01-02T00:00:00Z",
			"go_version":  "1.21",
			"environment": "development",
		},
	})
}

// Helper functions for data API

// CategoryFromParams extracts category from URL parameters
func CategoryFromParams(c *fiber.Ctx) string {
	return c.Params("category")
}

// Data API Stubs (referenced in routes but not implemented)

// GetCategorySchema returns category schema
func GetCategorySchema(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "GetCategorySchema not implemented yet"})
}

// GetCategoryData returns category data
func GetCategoryData(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "GetCategoryData not implemented yet"})
}

// GetTargetByID returns target by ID
func GetTargetByID(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "GetTargetByID not implemented yet"})
}

// GetTargetCategoryData returns target category data
func GetTargetCategoryData(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "GetTargetCategoryData not implemented yet"})
}

// CreateOrUpdateTargetData creates or updates target data
func CreateOrUpdateTargetData(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	targetID := c.Params("target_id")
	category := c.Params("category")

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	// 1. 요청 본문 파싱
	var requestBody map[string]interface{}
	if err := c.BodyParser(&requestBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body: " + err.Error()})
	}

	categoryData, ok := requestBody["category_data"].(map[string]interface{})
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "'category_data' field is required and must be an object"})
	}

	// 2. 스키마 검증 및 프로모션 정보 획득
	var schema *database.CategorySchema
	var schemaVersion float64 // JSON 숫자는 float64로 파싱됨
	versionValue, versionExists := requestBody["schema_version"]

	if versionExists {
		schemaVersion, ok = versionValue.(float64)
		if !ok {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": "'schema_version' must be a number"})
		}
		schema, err = orgDB.GetCategorySchemaByVersion(category, int(schemaVersion))
	} else {
		schema, err = orgDB.GetCategorySchema(category)
	}

	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"error":   "Category or specified schema version not found",
		})
	}

	// 스키마를 사용하여 데이터 검증
	cfg, _ := config.Load()
	dataBytes, err := json.Marshal(categoryData)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to marshal category data: " + err.Error(),
		})
	}
	validationResult, err := database.ValidateDataAgainstSchema(cfg, orgDB.OrgID, category, dataBytes)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Data validation failed: " + err.Error(),
		})
	}
	if !validationResult.Valid {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Data validation failed",
			"details": validationResult.Errors,
		})
	}

	// 3. 데이터를 프로모션된 필드와 나머지 JSONB 필드로 분리
	promotedData := database.PromotedData{
		Doubles:    make(map[string]float64),
		Integers:   make(map[string]int64),
		Keywords:   make(map[string]string),
		Flags:      make(map[string]bool),
		Timestamps: make(map[string]time.Time),
		Dates:      make(map[string]string),
	}
	jsonData := make(map[string]interface{})

	var schemaDefinition map[string]interface{}
	if err := json.Unmarshal([]byte(schema.SchemaDefinition), &schemaDefinition); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Failed to parse schema definition: " + err.Error()})
	}

	schemaFields, ok := schemaDefinition["fields"].(map[string]interface{})
	if !ok {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "Invalid schema format: 'fields' key is missing or not an object"})
	}

	for key, value := range categoryData {
		fieldInfo, ok := schemaFields[key].(map[string]interface{})
		if !ok {
			// 스키마에 정의되지 않은 필드는 일단 json에 포함
			jsonData[key] = value
			continue
		}

		if promotionType, ok := fieldInfo["promoted"].(string); ok {
			switch promotionType {
			case "double":
				if v, ok := value.(float64); ok {
					promotedData.Doubles[key] = v
				}
			case "integer":
				// JSON 숫자는 기본적으로 float64로 파싱되므로, 형변환이 필요합니다.
				if v, ok := value.(float64); ok {
					promotedData.Integers[key] = int64(v)
				}
			case "keyword":
				if v, ok := value.(string); ok {
					promotedData.Keywords[key] = v
				}
			case "flag":
				if v, ok := value.(bool); ok {
					promotedData.Flags[key] = v
				}
			case "timestamp":
				if v, ok := value.(string); ok {
					// RFC3339 포맷 검증
					if t, err := time.Parse(time.RFC3339, v); err == nil {
						promotedData.Timestamps[key] = t
					}
				}
			case "date":
				if v, ok := value.(string); ok {
					// YYYY-MM-DD 포맷 검증
					if _, err := time.Parse("2006-01-02", v); err == nil {
						promotedData.Dates[key] = v
					}
				}
			default: // unknown promotion type
				jsonData[key] = value
			}
		} else { // not promoted
			jsonData[key] = value
		}
	}

	// 4. 데이터베이스에 하이브리드 데이터 저장 (UPSERT)
	// 이 함수는 트랜잭션 내에서 target_categories와 promoted_* 테이블들을 모두 처리해야 합니다.
	targetName, _ := requestBody["target_name"].(string)
	if targetName == "" {
		targetName = targetID // target_name이 없으면 target_id를 이름으로 사용
	}

	err = orgDB.UpsertHybridData(targetID, targetName, category, schema.Version, jsonData, promotedData)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to save data: " + err.Error(),
		})
	}

	// 5. 응답 생성
	response := fiber.Map{
		"success":    true,
		"target_id":  targetID,
		"category":   category,
		"updated_at": time.Now(),
		"data_stored": fiber.Map{
			"json_data":     jsonData,
			"promoted_data": promotedData,
		},
	}

	return c.JSON(response)
}

// DeleteTargetData deletes target data
func DeleteTargetData(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	targetID := c.Params("target_id")
	category := c.Params("category")

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	// 타겟 데이터 삭제
	err = orgDB.DeleteTargetData(targetID, category)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to delete target data: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Target data deleted successfully",
	})
}

// UpdateTarget updates target metadata
func UpdateTarget(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	targetID := c.Params("target_id")
	category := c.Params("category")

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	var updateData map[string]interface{}
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
	}

	// 타겟 데이터 업데이트
	err = orgDB.UpdateTargetData(targetID, category, updateData)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to update target data: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Target data updated successfully",
	})
}

// GetTimeSeriesData returns time series data
func GetTimeSeriesData(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	targetID := c.Params("target_id")
	category := c.Params("category")

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	// 쿼리 파라미터 파싱
	var startTime, endTime *time.Time
	if startStr := c.Query("start_time"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			startTime = &t
		}
	}
	if endStr := c.Query("end_time"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			endTime = &t
		}
	}

	limit := c.QueryInt("limit", 100)

	// 시계열 데이터 조회
	data, err := orgDB.GetTimeSeriesData(targetID, category, startTime, endTime, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get time series data: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

// InsertTimeSeriesData inserts time series data
func InsertTimeSeriesData(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	targetID := c.Params("target_id")
	category := c.Params("category")

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	// FormData 파싱
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to parse form data: " + err.Error(),
		})
	}

	// 카테고리 데이터 파싱
	categoryDataStr := form.Value["category_data"]
	if len(categoryDataStr) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "category_data is required",
		})
	}

	var categoryData map[string]interface{}
	if err := json.Unmarshal([]byte(categoryDataStr[0]), &categoryData); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid JSON in category_data: " + err.Error(),
		})
	}

	// 타임스탬프 파싱 (선택사항)
	var timestamp *time.Time
	if timestampValues := form.Value["timestamp"]; len(timestampValues) > 0 {
		if t, err := time.Parse(time.RFC3339, timestampValues[0]); err == nil {
			timestamp = &t
		}
	}

	// 시계열 데이터 삽입
	err = orgDB.InsertTimeSeriesData(targetID, category, categoryData, timestamp)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to insert time series data: " + err.Error(),
		})
	}

	// 파일 처리
	files := form.File["files"]
	var uploadedFiles []map[string]interface{}

	for _, file := range files {
		fileInfo := map[string]interface{}{
			"filename":    file.Filename,
			"size":        file.Size,
			"file_id":     "ts_file_" + targetID + "_" + category + "_" + file.Filename,
			"uploaded_at": time.Now(),
		}
		uploadedFiles = append(uploadedFiles, fileInfo)
	}

	// 응답 생성
	response := fiber.Map{
		"success":   true,
		"target_id": targetID,
		"category":  category,
		"type":      "timeseries",
		"data":      categoryData,
		"files":     uploadedFiles,
		"timestamp": timestamp,
		"message":   "Time series data inserted successfully",
	}

	return c.JSON(response)
}

// UpdateTimeSeriesData updates time series data
func UpdateTimeSeriesData(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": false,
		"message": "Time series data cannot be updated - create new entry instead",
	})
}

// DeleteTimeSeriesData deletes time series data
func DeleteTimeSeriesData(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": false,
		"message": "Time series data deletion not implemented - data is immutable",
	})
}

// GetSingleListenerData returns single listener data
func GetSingleListenerData(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	listenerID := c.Params("listener_id")

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	// 리스너 관련 데이터 조회 (예: 특정 카테고리의 최근 데이터)
	filters := map[string]interface{}{
		"listener_id": listenerID,
	}

	data, err := orgDB.GetDataExplorerData(filters, 100)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get listener data: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

// GetMultiListenerData returns multi listener data
func GetMultiListenerData(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	// 쿼리 파라미터에서 리스너 ID 목록 가져오기
	listenerIDs := c.Query("listener_ids", "")
	categories := c.Query("categories", "")

	filters := map[string]interface{}{}
	if categories != "" {
		filters["category"] = categories
	}

	data, err := orgDB.GetDataExplorerData(filters, 500)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get multi listener data: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"data":         data,
		"listener_ids": listenerIDs,
		"categories":   categories,
	})
}

// UploadFiles handles file uploads
func UploadFiles(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": false,
		"message": "File upload not fully implemented - SeaweedFS integration needed",
	})
}

// DeleteFile deletes a file
func DeleteFile(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": false,
		"message": "File deletion not fully implemented - SeaweedFS integration needed",
	})
}

// Data Explorer API Handlers

// GetDataExplorerAPI는 데이터 탐색을 위한 API를 제공합니다.
func GetDataExplorerAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	// 쿼리 파라미터 파싱
	filters := map[string]interface{}{}
	if category := c.Query("category"); category != "" {
		filters["category"] = category
	}
	if targetID := c.Query("target_id"); targetID != "" {
		filters["target_id"] = targetID
	}

	limit := c.QueryInt("limit", 100)

	// 데이터 조회
	data, err := orgDB.GetDataExplorerData(filters, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get data explorer data: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
		"org_id":  orgID,
		"filters": filters,
	})
}

// SearchTargetsAPI는 타겟 ID를 검색하는 API를 제공합니다.
func SearchTargetsAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	query := c.Query("q", "")
	category := c.Query("category", "")
	limit := c.QueryInt("limit", 100)

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	// 타겟 검색
	targets, err := orgDB.SearchTargets(query, category, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to search targets: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"data":     targets,
		"query":    query,
		"category": category,
	})
}

// ValidateDataAPI는 데이터가 카테고리 스키마에 맞는지 검증하는 API를 제공합니다.
func ValidateDataAPI(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "organization ID is required",
		})
	}
	categoryName := c.Params("category")

	// 조직 데이터베이스 연결
	orgDB, err := database.GetOrgDB(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to connect to organization database: " + err.Error(),
		})
	}
	defer orgDB.Close()

	var requestData struct {
		Data map[string]interface{} `json:"data"`
	}

	if err := c.BodyParser(&requestData); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid request body",
		})
	}

	// 스키마 검증
	cfg, _ := config.Load()
	dataBytes, err := json.Marshal(requestData.Data)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to marshal request data: " + err.Error(),
		})
	}
	validationResult, err := database.ValidateDataAgainstSchema(cfg, orgDB.OrgID, categoryName, dataBytes)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "Data validation failed: " + err.Error(),
		})
	}

	// 카테고리 스키마 조회
	schema, err := orgDB.GetCategorySchema(categoryName)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"error":   "Category schema not found",
		})
	}

	validationResultMap := map[string]interface{}{
		"valid":    validationResult.Valid,
		"errors":   validationResult.Errors,
		"warnings": []string{},
		"schema":   schema.SchemaDefinition,
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    validationResultMap,
	})
}
