package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmidb/tmidb-core/internal/api/middleware"
	"github.com/tmidb/tmidb-core/internal/cache"
	"github.com/tmidb/tmidb-core/internal/database"
)

// 전역 캐시 인스턴스
var dataCache *cache.MemoryCache

// InitDataCache는 데이터 캐시를 초기화합니다
func InitDataCache() {
	// 최대 10000개 항목, 기본 TTL 5분
	dataCache = cache.NewMemoryCache(10000, 5*time.Minute)
}

// StandardResponse는 표준화된 API 응답 형식입니다
type StandardResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Meta      *Meta       `json:"meta,omitempty"`
	Error     *ApiError   `json:"error,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

// Meta는 메타데이터 정보입니다
type Meta struct {
	Pagination *PaginationMeta `json:"pagination,omitempty"`
	Version    *VersionMeta    `json:"version,omitempty"`
	Query      *QueryMeta      `json:"query,omitempty"`
}

// PaginationMeta는 페이징 메타데이터입니다
type PaginationMeta struct {
	CurrentPage  int  `json:"current_page"`
	PageSize     int  `json:"page_size"`
	TotalPages   int  `json:"total_pages"`
	TotalRecords int  `json:"total_records"`
	HasNext      bool `json:"has_next"`
	HasPrev      bool `json:"has_prev"`
}

// VersionMeta는 버전 메타데이터입니다
type VersionMeta struct {
	RequestedVersion string   `json:"requested_version"`
	ActualVersions   []string `json:"actual_versions"`
	IsMultiVersion   bool     `json:"is_multi_version"`
}

// QueryMeta는 쿼리 메타데이터입니다
type QueryMeta struct {
	Filters     []string `json:"filters,omitempty"`
	ProcessTime string   `json:"process_time,omitempty"`
	CacheHit    bool     `json:"cache_hit,omitempty"`
}

// ApiError는 표준화된 에러 형식입니다
type ApiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// CategoryData는 카테고리 데이터 구조입니다
type CategoryData struct {
	TargetID  string                 `json:"target_id"`
	Category  string                 `json:"category"`
	Version   string                 `json:"version"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// GetCategoryData는 카테고리별 데이터를 조회합니다
func GetCategoryData(c *fiber.Ctx) error {
	startTime := time.Now()

	// 컨텍스트 정보 가져오기
	versionCtx := middleware.GetVersionContext(c)
	paginationCtx := middleware.GetPaginationContext(c)

	category := c.Params("category")
	orgID, err := middleware.GetOrgIDFromToken(c)
	if err != nil {
		return sendErrorResponse(c, "AUTH_ERROR", err.Error(), "")
	}

	// 쿼리 파라미터 파싱
	queryFilters, err := parseQueryFilters(c)
	if err != nil {
		return sendErrorResponse(c, "QUERY_PARSE_ERROR", err.Error(), "")
	}

	// 캐시 키 생성
	cacheKey := fmt.Sprintf("category:%s:org:%d:v:%s:page:%d:size:%d:filters:%v",
		category, orgID, versionCtx.RequestedVersion,
		paginationCtx.Page, paginationCtx.PageSize, queryFilters)

	var data []CategoryData
	var totalCount int
	var cacheHit bool

	// 캐시에서 조회 시도
	if dataCache != nil {
		type CachedResult struct {
			Data       []CategoryData `json:"data"`
			TotalCount int            `json:"total_count"`
		}

		var cached CachedResult
		if dataCache.GetJSON(cacheKey, &cached) {
			data = cached.Data
			totalCount = cached.TotalCount
			cacheHit = true
		}
	}

	// 캐시 미스 시 DB에서 조회
	if !cacheHit {
		data, totalCount, err = getCategoryDataFromDB(orgID, category, versionCtx, paginationCtx, queryFilters)
		if err != nil {
			return sendErrorResponse(c, "DATABASE_ERROR", err.Error(), "")
		}

		// 결과를 캐시에 저장 (TTL: 3분)
		if dataCache != nil {
			cachedResult := struct {
				Data       []CategoryData `json:"data"`
				TotalCount int            `json:"total_count"`
			}{
				Data:       data,
				TotalCount: totalCount,
			}
			dataCache.SetJSON(cacheKey, cachedResult, 3*time.Minute)
		}
	}

	// 메타데이터 구성
	meta := &Meta{
		Pagination: &PaginationMeta{
			CurrentPage:  paginationCtx.Page,
			PageSize:     paginationCtx.PageSize,
			TotalRecords: totalCount,
			TotalPages:   (totalCount + paginationCtx.PageSize - 1) / paginationCtx.PageSize,
			HasNext:      paginationCtx.Page*paginationCtx.PageSize < totalCount,
			HasPrev:      paginationCtx.Page > 1,
		},
		Version: &VersionMeta{
			RequestedVersion: versionCtx.RequestedVersion,
			ActualVersions:   versionCtx.TargetVersions,
			IsMultiVersion:   versionCtx.IsMultiVersion,
		},
		Query: &QueryMeta{
			Filters:     queryFilters,
			ProcessTime: time.Since(startTime).String(),
			CacheHit:    cacheHit,
		},
	}

	return sendSuccessResponse(c, data, meta)
}

// GetTargetByID는 특정 타겟의 카테고리 데이터를 조회합니다
func GetTargetByID(c *fiber.Ctx) error {
	startTime := time.Now()

	targetID := c.Params("target_id")
	category := c.Params("category")
	versionCtx := middleware.GetVersionContext(c)
	orgID, err := middleware.GetOrgIDFromToken(c)
	if err != nil {
		return sendErrorResponse(c, "AUTH_ERROR", err.Error(), "")
	}

	// 단일 타겟 데이터 조회
	data, err := getTargetDataFromDB(orgID, targetID, category, versionCtx)
	if err != nil {
		if err == sql.ErrNoRows {
			return sendErrorResponse(c, "TARGET_NOT_FOUND",
				fmt.Sprintf("Target %s not found in category %s", targetID, category), "")
		}
		return sendErrorResponse(c, "DATABASE_ERROR", err.Error(), "")
	}

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

// CreateOrUpdateTargetData는 타겟 데이터를 생성/업데이트합니다
func CreateOrUpdateTargetData(c *fiber.Ctx) error {
	targetID := c.Params("target_id")
	category := c.Params("category")
	orgID, err := middleware.GetOrgIDFromToken(c)
	if err != nil {
		return sendErrorResponse(c, "AUTH_ERROR", err.Error(), "")
	}

	// 요청 본문 파싱
	var requestData map[string]interface{}
	if err := c.BodyParser(&requestData); err != nil {
		return sendErrorResponse(c, "INVALID_JSON", "Invalid JSON format", err.Error())
	}

	// 버전 정보 확인/설정
	version := "1"
	if v, exists := requestData["version"]; exists {
		if vStr, ok := v.(string); ok {
			version = strings.TrimPrefix(vStr, "v")
		}
	}

	// 카테고리 스키마 검증
	schemaValid, err := validateCategorySchema(orgID, category, version, requestData)
	if err != nil {
		return sendErrorResponse(c, "SCHEMA_VALIDATION_ERROR", err.Error(), "")
	}
	if !schemaValid {
		return sendErrorResponse(c, "SCHEMA_VALIDATION_FAILED",
			"Data does not match category schema", "")
	}

	// 데이터 저장
	err = saveTargetData(orgID, targetID, category, version, requestData)
	if err != nil {
		return sendErrorResponse(c, "DATABASE_ERROR", err.Error(), "")
	}

	// 캐시 무효화 (데이터 변경 시)
	if dataCache != nil {
		dataCache.InvalidateCategory(category)
		dataCache.InvalidateTarget(targetID)
	}

	// 응답 데이터 구성
	responseData := &CategoryData{
		TargetID:  targetID,
		Category:  category,
		Version:   version,
		Data:      requestData,
		UpdatedAt: time.Now(),
	}

	return sendSuccessResponse(c, responseData, nil)
}

// DeleteTargetData는 타겟 데이터를 삭제합니다
func DeleteTargetData(c *fiber.Ctx) error {
	targetID := c.Params("target_id")
	category := c.Params("category")
	orgID, err := middleware.GetOrgIDFromToken(c)
	if err != nil {
		return sendErrorResponse(c, "AUTH_ERROR", err.Error(), "")
	}

	// 삭제 실행
	rowsAffected, err := deleteTargetData(orgID, targetID, category)
	if err != nil {
		return sendErrorResponse(c, "DATABASE_ERROR", err.Error(), "")
	}

	if rowsAffected == 0 {
		return sendErrorResponse(c, "TARGET_NOT_FOUND",
			fmt.Sprintf("Target %s not found in category %s", targetID, category), "")
	}

	// 캐시 무효화 (데이터 삭제 시)
	if dataCache != nil {
		dataCache.InvalidateCategory(category)
		dataCache.InvalidateTarget(targetID)
	}

	return sendSuccessResponse(c, fiber.Map{
		"target_id":  targetID,
		"category":   category,
		"deleted":    true,
		"deleted_at": time.Now(),
	}, nil)
}

// 헬퍼 함수들

// getCategoryDataFromDB는 데이터베이스에서 카테고리 데이터를 조회합니다
func getCategoryDataFromDB(orgID int, category string, versionCtx *middleware.VersionContext,
	paginationCtx *middleware.PaginationContext, filters []string) ([]CategoryData, int, error) {

	db := database.GetDB()

	// COUNT 쿼리 (총 개수)
	countQuery := buildCountQuery(category, versionCtx, filters)
	var totalCount int
	err := db.QueryRow(countQuery, orgID).Scan(&totalCount)
	if err != nil {
		return nil, 0, err
	}

	// 데이터 조회 쿼리
	dataQuery := buildDataQuery(category, versionCtx, paginationCtx, filters)

	offset := (paginationCtx.Page - 1) * paginationCtx.PageSize
	rows, err := db.Query(dataQuery, orgID, paginationCtx.PageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []CategoryData
	for rows.Next() {
		var item CategoryData
		var dataJSON string
		var createdAt, updatedAt time.Time

		err := rows.Scan(&item.TargetID, &item.Category, &item.Version,
			&dataJSON, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		// JSON 데이터 파싱
		if err := json.Unmarshal([]byte(dataJSON), &item.Data); err != nil {
			continue
		}

		item.CreatedAt = createdAt
		item.UpdatedAt = updatedAt
		results = append(results, item)
	}

	return results, totalCount, nil
}

// getTargetDataFromDB는 특정 타겟의 데이터를 조회합니다
func getTargetDataFromDB(orgID int, targetID, category string,
	versionCtx *middleware.VersionContext) (*CategoryData, error) {

	db := database.GetDB()

	// 버전별 쿼리 구성
	var query string
	var args []interface{}

	if versionCtx.RequestedVersion == "all" {
		// 모든 버전 조회
		query = `
			SELECT target_id, category_name, schema_version, category_data, created_at, updated_at
			FROM target_categories 
			WHERE org_id = $1 AND target_id = $2 AND category_name = $3
			ORDER BY schema_version DESC
		`
		args = []interface{}{orgID, targetID, category}
	} else if versionCtx.RequestedVersion == "latest" {
		// 최신 버전만 조회
		query = `
			SELECT target_id, category_name, schema_version, category_data, created_at, updated_at
			FROM target_categories 
			WHERE org_id = $1 AND target_id = $2 AND category_name = $3
			ORDER BY schema_version DESC 
			LIMIT 1
		`
		args = []interface{}{orgID, targetID, category}
	} else {
		// 특정 버전 조회
		version := strings.TrimPrefix(versionCtx.RequestedVersion, "v")
		query = `
			SELECT target_id, category_name, schema_version, category_data, created_at, updated_at
			FROM target_categories 
			WHERE org_id = $1 AND target_id = $2 AND category_name = $3 AND schema_version = $4
		`
		args = []interface{}{orgID, targetID, category, version}
	}

	var result CategoryData
	var dataJSON string
	var schemaVersion int

	err := db.QueryRow(query, args...).Scan(
		&result.TargetID, &result.Category, &schemaVersion,
		&dataJSON, &result.CreatedAt, &result.UpdatedAt)

	if err != nil {
		return nil, err
	}

	result.Version = strconv.Itoa(schemaVersion)

	// JSON 데이터 파싱
	if err := json.Unmarshal([]byte(dataJSON), &result.Data); err != nil {
		return nil, err
	}

	return &result, nil
}

// 응답 헬퍼 함수들

// sendSuccessResponse는 성공 응답을 전송합니다
func sendSuccessResponse(c *fiber.Ctx, data interface{}, meta *Meta) error {
	response := StandardResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now(),
		RequestID: c.Get("X-Request-ID", generateRequestID()),
	}

	return c.JSON(response)
}

// sendErrorResponse는 에러 응답을 전송합니다
func sendErrorResponse(c *fiber.Ctx, code, message, details string) error {
	response := StandardResponse{
		Success: false,
		Error: &ApiError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Timestamp: time.Now(),
		RequestID: c.Get("X-Request-ID", generateRequestID()),
	}

	statusCode := getStatusCodeFromErrorCode(code)
	return c.Status(statusCode).JSON(response)
}

// generateRequestID는 요청 ID를 생성합니다
func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// getStatusCodeFromErrorCode는 에러 코드에 따른 HTTP 상태 코드를 반환합니다
func getStatusCodeFromErrorCode(code string) int {
	switch code {
	case "AUTH_ERROR", "AUTH_TOKEN_MISSING", "AUTH_TOKEN_INVALID", "AUTH_TOKEN_EXPIRED":
		return 401
	case "AUTH_PERMISSION_DENIED", "AUTH_CATEGORY_DENIED":
		return 403
	case "TARGET_NOT_FOUND", "CATEGORY_NOT_FOUND":
		return 404
	case "INVALID_JSON", "SCHEMA_VALIDATION_ERROR", "SCHEMA_VALIDATION_FAILED", "QUERY_PARSE_ERROR":
		return 400
	case "DATABASE_ERROR":
		return 500
	default:
		return 500
	}
}
