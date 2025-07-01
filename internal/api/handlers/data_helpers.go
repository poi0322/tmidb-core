package handlers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmidb/tmidb-core/internal/api/middleware"
	"github.com/tmidb/tmidb-core/internal/database"
)

// parseQueryFilters는 쿼리 파라미터를 파싱합니다
func parseQueryFilters(c *fiber.Ctx) ([]string, error) {
	var filters []string

	// 기존 query_parser.go의 로직을 확장
	queries := c.Context().QueryArgs()

	// 예약된 파라미터 제외
	reservedParams := map[string]bool{
		"page":      true,
		"page_size": true,
		"auto_size": true,
		"sort":      true,
		"order":     true,
	}

	queries.VisitAll(func(key, value []byte) {
		keyStr := string(key)
		valueStr := string(value)

		if reservedParams[keyStr] {
			return
		}

		// 복잡한 쿼리 파싱 (기존 코드 활용)
		filter := parseComplexFilter(keyStr, valueStr)
		if filter != "" {
			filters = append(filters, filter)
		}
	})

	return filters, nil
}

// parseComplexFilter는 복잡한 필터를 파싱합니다
func parseComplexFilter(key, value string) string {
	// 기존 query_parser.go에서 사용하던 패턴들

	// 배열 연산: tags[]contains=urgent
	if strings.Contains(key, "[]") {
		arrayField := strings.Replace(key, "[]", "", 1)
		return fmt.Sprintf("%s ARRAY OPERATION '%s'", arrayField, value)
	}

	// 비교 연산자: age>18, bp>=120
	comparisonOps := []string{">=", "<=", "!=", ">", "<", "="}
	for _, op := range comparisonOps {
		if strings.Contains(key, op) {
			parts := strings.Split(key, op)
			if len(parts) == 2 {
				field := parts[0]
				return fmt.Sprintf("%s %s '%s'", field, op, value)
			}
		}
	}

	// 존재 확인: field.exists=true
	if strings.HasSuffix(key, ".exists") {
		field := strings.TrimSuffix(key, ".exists")
		if value == "true" {
			return fmt.Sprintf("%s IS NOT NULL", field)
		} else {
			return fmt.Sprintf("%s IS NULL", field)
		}
	}

	// 기본 등등 비교
	return fmt.Sprintf("%s = '%s'", key, value)
}

// buildCountQuery는 COUNT 쿼리를 생성합니다
func buildCountQuery(category string, versionCtx *middleware.VersionContext, filters []string) string {
	baseQuery := "SELECT COUNT(*) FROM target_categories WHERE org_id = $1 AND category_name = '" + category + "'"

	// 버전 필터 추가
	if versionCtx.RequestedVersion != "all" && versionCtx.RequestedVersion != "latest" {
		version := strings.TrimPrefix(versionCtx.RequestedVersion, "v")
		baseQuery += " AND schema_version = " + version
	}

	// 추가 필터 적용
	for _, filter := range filters {
		// JSON 필드 검색을 위한 PostgreSQL JSON 연산자 사용
		jsonFilter := convertFilterToJSONB(filter)
		baseQuery += " AND " + jsonFilter
	}

	return baseQuery
}

// buildDataQuery는 데이터 조회 쿼리를 생성합니다
func buildDataQuery(category string, versionCtx *middleware.VersionContext,
	paginationCtx *middleware.PaginationContext, filters []string) string {

	baseQuery := `
		SELECT target_id, category_name, schema_version::text, category_data::text, created_at, updated_at 
		FROM target_categories 
		WHERE org_id = $1 AND category_name = '` + category + `'`

	// 버전 필터 추가
	if versionCtx.RequestedVersion != "all" && versionCtx.RequestedVersion != "latest" {
		version := strings.TrimPrefix(versionCtx.RequestedVersion, "v")
		baseQuery += " AND schema_version = " + version
	}

	// 추가 필터 적용
	for _, filter := range filters {
		jsonFilter := convertFilterToJSONB(filter)
		baseQuery += " AND " + jsonFilter
	}

	// 정렬 (최신 순)
	baseQuery += " ORDER BY updated_at DESC"

	// 페이징
	baseQuery += " LIMIT $2 OFFSET $3"

	return baseQuery
}

// convertFilterToJSONB는 필터를 PostgreSQL JSONB 쿼리로 변환합니다
func convertFilterToJSONB(filter string) string {
	// 간단한 패턴 매칭으로 JSONB 쿼리 생성
	// 예: "age > '18'" -> "category_data->>'age'::numeric > 18"

	// 숫자 비교 패턴
	numericPattern := regexp.MustCompile(`(\w+)\s*(>=|<=|!=|>|<|=)\s*'(\d+(?:\.\d+)?)'`)
	if match := numericPattern.FindStringSubmatch(filter); match != nil {
		field, op, value := match[1], match[2], match[3]
		return fmt.Sprintf("(category_data->>'%s')::numeric %s %s", field, op, value)
	}

	// 문자열 비교 패턴
	stringPattern := regexp.MustCompile(`(\w+)\s*(=|!=)\s*'([^']+)'`)
	if match := stringPattern.FindStringSubmatch(filter); match != nil {
		field, op, value := match[1], match[2], match[3]
		if op == "!=" {
			return fmt.Sprintf("category_data->>'%s' <> '%s'", field, value)
		}
		return fmt.Sprintf("category_data->>'%s' = '%s'", field, value)
	}

	// NULL 체크 패턴
	if strings.Contains(filter, "IS NOT NULL") {
		field := strings.Split(filter, " ")[0]
		return fmt.Sprintf("category_data ? '%s'", field)
	}
	if strings.Contains(filter, "IS NULL") {
		field := strings.Split(filter, " ")[0]
		return fmt.Sprintf("NOT category_data ? '%s'", field)
	}

	// 배열 연산 패턴 (간단 구현)
	if strings.Contains(filter, "ARRAY OPERATION") {
		// TODO: 복잡한 배열 연산 구현
		return "true" // 임시로 항상 참 반환
	}

	// 기본값 (안전한 처리)
	return "true"
}

// validateCategorySchema는 카테고리 스키마에 대한 데이터 검증을 수행합니다
func validateCategorySchema(orgID int, category, version string, data map[string]interface{}) (bool, error) {
	db := database.GetDB()

	// 카테고리 스키마 조회
	var schemaJSON string
	query := `
		SELECT schema_definition 
		FROM category_schemas 
		WHERE org_id = $1 AND category_name = $2 AND version = $3
	`

	err := db.QueryRow(query, orgID, category, version).Scan(&schemaJSON)
	if err != nil {
		// 스키마가 없으면 기본적으로 허용 (유연한 스키마)
		return true, nil
	}

	// JSON 스키마 파싱
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return false, fmt.Errorf("invalid schema format: %v", err)
	}

	// 기본적인 스키마 검증 (실제로는 더 복잡한 JSON Schema 라이브러리 사용 권장)
	return validateDataAgainstSchema(data, schema), nil
}

// validateDataAgainstSchema는 데이터와 스키마를 비교합니다
func validateDataAgainstSchema(data, schema map[string]interface{}) bool {
	// 간단한 스키마 검증 로직
	properties, hasProperties := schema["properties"].(map[string]interface{})
	if !hasProperties {
		return true // 스키마에 properties가 없으면 모든 데이터 허용
	}

	// 필수 필드 검증
	if required, hasRequired := schema["required"].([]interface{}); hasRequired {
		for _, reqField := range required {
			fieldName := reqField.(string)
			if _, exists := data[fieldName]; !exists {
				return false // 필수 필드 누락
			}
		}
	}

	// 타입 검증 (간단 구현)
	for fieldName, fieldSchema := range properties {
		if fieldValue, hasField := data[fieldName]; hasField {
			if fieldSchemaMap, ok := fieldSchema.(map[string]interface{}); ok {
				if fieldType, hasType := fieldSchemaMap["type"].(string); hasType {
					if !validateFieldType(fieldValue, fieldType) {
						return false
					}
				}
			}
		}
	}

	return true
}

// validateFieldType은 필드 타입을 검증합니다
func validateFieldType(value interface{}, expectedType string) bool {
	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok1 := value.(float64)
		_, ok2 := value.(int)
		return ok1 || ok2
	case "integer":
		_, ok := value.(int)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := value.([]interface{})
		return ok
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	default:
		return true // 알 수 없는 타입은 허용
	}
}

// saveTargetData는 타겟 데이터를 저장합니다
func saveTargetData(orgID int, targetID, category, version string, data map[string]interface{}) error {
	db := database.GetDB()

	// JSON 데이터 직렬화
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}

	versionInt, _ := strconv.Atoi(version)

	// UPSERT 쿼리 (PostgreSQL)
	query := `
		INSERT INTO target_categories (org_id, target_id, category_name, schema_version, category_data, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (org_id, target_id, category_name, schema_version)
		DO UPDATE SET 
			category_data = EXCLUDED.category_data,
			updated_at = NOW()
	`

	_, err = db.Exec(query, orgID, targetID, category, versionInt, string(dataJSON))
	return err
}

// deleteTargetData는 타겟 데이터를 삭제합니다
func deleteTargetData(orgID int, targetID, category string) (int64, error) {
	db := database.GetDB()

	query := `
		DELETE FROM target_categories 
		WHERE org_id = $1 AND target_id = $2 AND category_name = $3
	`

	result, err := db.Exec(query, orgID, targetID, category)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	return rowsAffected, err
}

// CategoryFromParams는 URL 파라미터에서 카테고리를 추출합니다 (권한 확인용)
func CategoryFromParams(c *fiber.Ctx) string {
	return c.Params("category")
}

// 시계열 데이터 관련 함수들

// GetTimeSeriesData는 시계열 데이터를 조회합니다
func GetTimeSeriesDataHelper(c *fiber.Ctx) error {
	targetID := c.Params("target_id")
	category := c.Params("category")
	orgID, err := middleware.GetOrgIDFromToken(c)
	if err != nil {
		return sendErrorResponse(c, "AUTH_ERROR", err.Error(), "")
	}

	// 시간 범위 파라미터
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")
	interval := c.Query("interval", "1h") // 기본 1시간 간격

	// TimescaleDB 쿼리
	data, err := getTimeSeriesFromDB(orgID, targetID, category, startTime, endTime, interval)
	if err != nil {
		return sendErrorResponse(c, "DATABASE_ERROR", err.Error(), "")
	}

	return sendSuccessResponse(c, data, nil)
}

// InsertTimeSeriesData는 시계열 데이터를 삽입합니다
func InsertTimeSeriesDataHelper(c *fiber.Ctx) error {
	targetID := c.Params("target_id")
	category := c.Params("category")
	orgID, err := middleware.GetOrgIDFromToken(c)
	if err != nil {
		return sendErrorResponse(c, "AUTH_ERROR", err.Error(), "")
	}

	// 요청 데이터 파싱
	var timeSeriesData []map[string]interface{}
	if err := c.BodyParser(&timeSeriesData); err != nil {
		return sendErrorResponse(c, "INVALID_JSON", "Invalid JSON format", err.Error())
	}

	// 시계열 데이터 저장
	err = saveTimeSeriesData(orgID, targetID, category, timeSeriesData)
	if err != nil {
		return sendErrorResponse(c, "DATABASE_ERROR", err.Error(), "")
	}

	return sendSuccessResponse(c, fiber.Map{
		"inserted_count": len(timeSeriesData),
		"target_id":      targetID,
		"category":       category,
	}, nil)
}

// getTimeSeriesFromDB는 시계열 데이터를 조회합니다
func getTimeSeriesFromDB(orgID int, targetID, category, startTime, endTime, interval string) (interface{}, error) {
	db := database.GetDB()

	// TimescaleDB time_bucket 함수 사용
	query := `
		SELECT 
			time_bucket($5::interval, timestamp) as time_bucket,
			AVG((data->>'value')::numeric) as avg_value,
			COUNT(*) as count
		FROM target_timeseries 
		WHERE org_id = $1 AND target_id = $2 AND category = $3
	`
	args := []interface{}{orgID, targetID, category}

	if startTime != "" {
		query += " AND timestamp >= $4"
		args = append(args, startTime)
	}
	if endTime != "" {
		if startTime != "" {
			query += " AND timestamp <= $5"
			args = append(args, endTime, interval)
		} else {
			query += " AND timestamp <= $4"
			args = append(args, endTime, interval)
		}
	} else {
		args = append(args, interval)
	}

	query += " GROUP BY time_bucket ORDER BY time_bucket"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var timeBucket string
		var avgValue float64
		var count int

		err := rows.Scan(&timeBucket, &avgValue, &count)
		if err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"timestamp": timeBucket,
			"avg_value": avgValue,
			"count":     count,
		})
	}

	return results, nil
}

// saveTimeSeriesData는 시계열 데이터를 저장합니다
func saveTimeSeriesData(orgID int, targetID, category string, data []map[string]interface{}) error {
	db := database.GetDB()

	// 트랜잭션 시작
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 준비된 문 생성
	stmt, err := tx.Prepare(`
		INSERT INTO target_timeseries (org_id, target_id, category, timestamp, data)
		VALUES ($1, $2, $3, $4, $5)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// 각 데이터 포인트 삽입
	for _, point := range data {
		// 타임스탬프 추출 (없으면 현재 시간 사용)
		timestamp := time.Now()
		if ts, exists := point["timestamp"]; exists {
			if tsStr, ok := ts.(string); ok {
				if parsedTime, err := time.Parse(time.RFC3339, tsStr); err == nil {
					timestamp = parsedTime
				}
			}
		}

		// JSON 데이터 직렬화
		pointJSON, err := json.Marshal(point)
		if err != nil {
			continue
		}

		_, err = stmt.Exec(orgID, targetID, category, timestamp, string(pointJSON))
		if err != nil {
			continue // 개별 실패는 로그만 기록하고 계속
		}
	}

	return tx.Commit()
}
