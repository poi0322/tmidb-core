package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmidb/tmidb-core/internal/api/middleware"
	"github.com/tmidb/tmidb-core/internal/database"
)

// ListenerData는 리스너 데이터 구조입니다
type ListenerData struct {
	ListenerID   string                            `json:"listener_id"`
	Name         string                            `json:"name"`
	Description  string                            `json:"description,omitempty"`
	Categories   map[string][]CategoryData         `json:"categories"`
	Metadata     map[string]interface{}            `json:"metadata,omitempty"`
	LastUpdated  time.Time                         `json:"last_updated"`
	SubscribeName string                           `json:"subscribe_name,omitempty"`
}

// ListenerConfig는 리스너 설정 구조입니다
type ListenerConfig struct {
	ListenerID  string                 `json:"listener_id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Queries     map[string]string      `json:"queries"`     // 카테고리별 쿼리
	Filters     map[string]interface{} `json:"filters,omitempty"`
	CreatedBy   int                    `json:"created_by"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// GetSingleListenerData는 단일 리스너 데이터를 조회합니다
func GetSingleListenerData(c *fiber.Ctx) error {
	startTime := time.Now()
	
	listenerID := c.Params("listener_id")
	orgID, err := middleware.GetOrgIDFromToken(c)
	if err != nil {
		return sendErrorResponse(c, "AUTH_ERROR", err.Error(), "")
	}

	// subscribe_name 파라미터 확인
	subscribeName := c.Query("subscribe_name")

	// 리스너 설정 조회
	listenerConfig, err := getListenerConfig(orgID, listenerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return sendErrorResponse(c, "LISTENER_NOT_FOUND", 
				fmt.Sprintf("Listener %s not found", listenerID), "")
		}
		return sendErrorResponse(c, "DATABASE_ERROR", err.Error(), "")
	}

	// 버전 정보 가져오기
	versionCtx := middleware.GetVersionContext(c)
	paginationCtx := middleware.GetPaginationContext(c)

	// 리스너 데이터 조회
	data, err := getListenerData(orgID, listenerConfig, versionCtx, paginationCtx)
	if err != nil {
		return sendErrorResponse(c, "DATABASE_ERROR", err.Error(), "")
	}

	// subscribe_name이 있으면 추가
	if subscribeName != "" {
		data.SubscribeName = subscribeName
	}

	// 메타데이터 구성
	meta := &Meta{
		Version: &VersionMeta{
			RequestedVersion: versionCtx.RequestedVersion,
			ActualVersions:   versionCtx.TargetVersions,
			IsMultiVersion:   versionCtx.IsMultiVersion,
		},
		Query: &QueryMeta{
			ProcessTime: time.Since(startTime).String(),
		},
	}

	return sendSuccessResponse(c, data, meta)
}

// GetMultiListenerData는 다중 리스너 경로를 처리합니다
func GetMultiListenerData(c *fiber.Ctx) error {
	startTime := time.Now()
	
	// 경로에서 리스너 ID들 추출: /listener/vital+ward+io
	path := c.Params("*")
	listenerIDs := strings.Split(path, "+")
	
	if len(listenerIDs) == 0 {
		return sendErrorResponse(c, "INVALID_LISTENER_PATH", 
			"Invalid listener path format. Use: /listener/id1+id2+id3", "")
	}

	orgID, err := middleware.GetOrgIDFromToken(c)
	if err != nil {
		return sendErrorResponse(c, "AUTH_ERROR", err.Error(), "")
	}

	// subscribe_name 파라미터 확인
	subscribeName := c.Query("subscribe_name")

	// 버전 정보 가져오기
	versionCtx := middleware.GetVersionContext(c)
	paginationCtx := middleware.GetPaginationContext(c)

	// 각 리스너 데이터 조회
	var combinedData = make(map[string][]CategoryData)
	var lastUpdated time.Time

	for _, listenerID := range listenerIDs {
		listenerID = strings.TrimSpace(listenerID)
		if listenerID == "" {
			continue
		}

		// 리스너 설정 조회
		listenerConfig, err := getListenerConfig(orgID, listenerID)
		if err != nil {
			continue // 에러 리스너는 스킵
		}

		// 리스너 데이터 조회
		data, err := getListenerData(orgID, listenerConfig, versionCtx, paginationCtx)
		if err != nil {
			continue // 에러 리스너는 스킵
		}

		// 데이터 병합
		for category, categoryData := range data.Categories {
			if existingData, exists := combinedData[category]; exists {
				combinedData[category] = append(existingData, categoryData...)
			} else {
				combinedData[category] = categoryData
			}
		}

		// 최신 업데이트 시간 추적
		if data.LastUpdated.After(lastUpdated) {
			lastUpdated = data.LastUpdated
		}
	}

	// 응답 데이터 구성
	responseData := &ListenerData{
		ListenerID:    strings.Join(listenerIDs, "+"),
		Name:          "Combined Listeners",
		Categories:    combinedData,
		LastUpdated:   lastUpdated,
		SubscribeName: subscribeName,
	}

	// 메타데이터 구성
	meta := &Meta{
		Version: &VersionMeta{
			RequestedVersion: versionCtx.RequestedVersion,
			ActualVersions:   versionCtx.TargetVersions,
			IsMultiVersion:   versionCtx.IsMultiVersion,
		},
		Query: &QueryMeta{
			ProcessTime: time.Since(startTime).String(),
		},
	}

	return sendSuccessResponse(c, responseData, meta)
}

// GetCategorySchema는 카테고리 스키마를 조회합니다
func GetCategorySchema(c *fiber.Ctx) error {
	category := c.Params("category")
	orgID, err := middleware.GetOrgIDFromToken(c)
	if err != nil {
		return sendErrorResponse(c, "AUTH_ERROR", err.Error(), "")
	}

	versionCtx := middleware.GetVersionContext(c)
	
	// 스키마 조회
	schema, err := getCategorySchemaFromDB(orgID, category, versionCtx.RequestedVersion)
	if err != nil {
		if err == sql.ErrNoRows {
			return sendErrorResponse(c, "SCHEMA_NOT_FOUND", 
				fmt.Sprintf("Schema not found for category %s", category), "")
		}
		return sendErrorResponse(c, "DATABASE_ERROR", err.Error(), "")
	}

	return sendSuccessResponse(c, schema, nil)
}

// HealthCheck는 시스템 상태를 확인합니다
func HealthCheck(c *fiber.Ctx) error {
	// 데이터베이스 연결 확인
	db := database.GetDB()
	err := db.Ping()
	
	status := "healthy"
	if err != nil {
		status = "unhealthy"
	}

	healthData := fiber.Map{
		"status":    status,
		"timestamp": time.Now(),
		"version":   "1.0.0", // TODO: 실제 버전 정보로 교체
		"database":  status,
	}

	if status == "unhealthy" {
		return c.Status(503).JSON(StandardResponse{
			Success:   false,
			Data:      healthData,
			Timestamp: time.Now(),
		})
	}

	return sendSuccessResponse(c, healthData, nil)
}

// SystemInfo는 시스템 정보를 반환합니다
func SystemInfo(c *fiber.Ctx) error {
	systemInfo := fiber.Map{
		"name":        "tmiDB",
		"version":     "1.0.0", // TODO: 실제 버전 정보로 교체
		"description": "Target-based Real-time Data Management Platform",
		"api_version": "v1",
		"endpoints": fiber.Map{
			"health":     "/api/health",
			"system":     "/api/system/info",
			"categories": "/api/{version}/category/{category}",
			"targets":    "/api/{version}/targets/{target_id}/categories/{category}",
			"listeners":  "/api/{version}/listener/{listener_id}",
		},
		"supported_versions": []string{"v1", "v2", "latest", "all"},
		"timestamp": time.Now(),
	}

	return sendSuccessResponse(c, systemInfo, nil)
}

// 헬퍼 함수들

// getListenerConfig는 리스너 설정을 조회합니다
func getListenerConfig(orgID int, listenerID string) (*ListenerConfig, error) {
	db := database.GetDB()
	
	var config ListenerConfig
	var queriesJSON string
	var filtersJSON sql.NullString
	
	query := `
		SELECT listener_id, name, description, queries, filters, created_by, created_at, updated_at
		FROM listeners 
		WHERE org_id = $1 AND listener_id = $2
	`
	
	err := db.QueryRow(query, orgID, listenerID).Scan(
		&config.ListenerID, &config.Name, &config.Description, 
		&queriesJSON, &filtersJSON, &config.CreatedBy, 
		&config.CreatedAt, &config.UpdatedAt)
	
	if err != nil {
		return nil, err
	}
	
	// JSON 파싱
	if err := json.Unmarshal([]byte(queriesJSON), &config.Queries); err != nil {
		return nil, fmt.Errorf("failed to parse queries: %v", err)
	}
	
	if filtersJSON.Valid {
		if err := json.Unmarshal([]byte(filtersJSON.String), &config.Filters); err != nil {
			return nil, fmt.Errorf("failed to parse filters: %v", err)
		}
	}
	
	return &config, nil
}

// getListenerData는 리스너 데이터를 조회합니다
func getListenerData(orgID int, config *ListenerConfig, versionCtx *middleware.VersionContext, 
	paginationCtx *middleware.PaginationContext) (*ListenerData, error) {
	
	data := &ListenerData{
		ListenerID:  config.ListenerID,
		Name:        config.Name,
		Description: config.Description,
		Categories:  make(map[string][]CategoryData),
		LastUpdated: config.UpdatedAt,
	}
	
	// 각 카테고리별 데이터 조회
	for category, query := range config.Queries {
		// 쿼리 파싱 (간단 구현)
		filters := parseQueryString(query)
		
		// 카테고리 데이터 조회
		categoryData, _, err := getCategoryDataFromDB(orgID, category, versionCtx, paginationCtx, filters)
		if err != nil {
			continue // 에러 카테고리는 스킵
		}
		
		data.Categories[category] = categoryData
		
		// 최신 업데이트 시간 추적
		for _, item := range categoryData {
			if item.UpdatedAt.After(data.LastUpdated) {
				data.LastUpdated = item.UpdatedAt
			}
		}
	}
	
	return data, nil
}

// getCategorySchemaFromDB는 카테고리 스키마를 조회합니다
func getCategorySchemaFromDB(orgID int, category, version string) (interface{}, error) {
	db := database.GetDB()
	
	var schemaJSON string
	var actualVersion string
	
	// 버전별 쿼리
	var query string
	var args []interface{}
	
	if version == "latest" {
		query = `
			SELECT version::text, schema_definition 
			FROM category_schemas 
			WHERE org_id = $1 AND category_name = $2 
			ORDER BY version::int DESC 
			LIMIT 1
		`
		args = []interface{}{orgID, category}
	} else if version == "all" {
		// 모든 버전 반환 (다른 구조 필요)
		return getAllVersionSchemas(orgID, category)
	} else {
		numericVersion := strings.TrimPrefix(version, "v")
		query = `
			SELECT version::text, schema_definition 
			FROM category_schemas 
			WHERE org_id = $1 AND category_name = $2 AND version = $3
		`
		args = []interface{}{orgID, category, numericVersion}
	}
	
	err := db.QueryRow(query, args...).Scan(&actualVersion, &schemaJSON)
	if err != nil {
		return nil, err
	}
	
	// JSON 파싱
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return nil, err
	}
	
	// 버전 정보 추가
	result := map[string]interface{}{
		"category": category,
		"version":  actualVersion,
		"schema":   schema,
	}
	
	return result, nil
}

// getAllVersionSchemas는 모든 버전의 스키마를 조회합니다
func getAllVersionSchemas(orgID int, category string) (interface{}, error) {
	db := database.GetDB()
	
	query := `
		SELECT version::text, schema_definition 
		FROM category_schemas 
		WHERE org_id = $1 AND category_name = $2 
		ORDER BY version::int DESC
	`
	
	rows, err := db.Query(query, orgID, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var versions []map[string]interface{}
	
	for rows.Next() {
		var version, schemaJSON string
		err := rows.Scan(&version, &schemaJSON)
		if err != nil {
			continue
		}
		
		var schema map[string]interface{}
		if json.Unmarshal([]byte(schemaJSON), &schema) == nil {
			versions = append(versions, map[string]interface{}{
				"version": version,
				"schema":  schema,
			})
		}
	}
	
	return map[string]interface{}{
		"category": category,
		"versions": versions,
	}, nil
}

// parseQueryString은 쿼리 문자열을 파싱합니다 (간단 구현)
func parseQueryString(queryStr string) []string {
	// 실제로는 더 복잡한 쿼리 파싱이 필요
	// 여기서는 간단하게 구현
	if queryStr == "" {
		return []string{}
	}
	
	// 예: "bp>=120&ward=ICU" -> ["bp >= '120'", "ward = 'ICU'"]
	filters := []string{}
	parts := strings.Split(queryStr, "&")
	
	for _, part := range parts {
		if strings.Contains(part, "=") {
			filters = append(filters, parseComplexFilter(part, ""))
		}
	}
	
	return filters
} 