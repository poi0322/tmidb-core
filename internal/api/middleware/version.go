package middleware

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmidb/tmidb-core/internal/database"
)

// 버전 패턴을 매칭하는 정규식
var versionPattern = regexp.MustCompile(`^v(\d+)$`)

// VersionContext는 버전 관련 컨텍스트 정보입니다
type VersionContext struct {
	RequestedVersion string   `json:"requested_version"` // v1, v2, latest, all
	TargetVersions   []string `json:"target_versions"`   // 실제 조회할 버전들
	IsMultiVersion   bool     `json:"is_multi_version"`  // all 요청 여부
}

// PaginationContext는 페이징 관련 컨텍스트 정보입니다
type PaginationContext struct {
	Page           int  `json:"page"`
	PageSize       int  `json:"page_size"`
	AutoPagination bool `json:"auto_pagination"` // 자동 페이징 적용 여부
	MaxPageSize    int  `json:"max_page_size"`   // 최대 페이지 크기
}

// VersionMiddleware는 API 버전 처리를 담당합니다
func VersionMiddleware(version string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		versionCtx := &VersionContext{
			RequestedVersion: version,
		}

		// 버전별 처리 로직
		switch version {
		case "v1":
			versionCtx.TargetVersions = []string{"1"}
			versionCtx.IsMultiVersion = false
		case "v2":
			versionCtx.TargetVersions = []string{"2"}
			versionCtx.IsMultiVersion = false
		case "latest":
			// 카테고리별로 최신 버전을 조회해야 함 (핸들러에서 처리)
			versionCtx.TargetVersions = []string{"latest"}
			versionCtx.IsMultiVersion = false
		case "all":
			// 모든 버전을 조회
			versionCtx.TargetVersions = []string{"all"}
			versionCtx.IsMultiVersion = true
		default:
			return c.Status(400).JSON(fiber.Map{
				"error":              "Unsupported API version",
				"code":               "VERSION_UNSUPPORTED",
				"supported_versions": []string{"v1", "v2", "latest", "all"},
			})
		}

		// 컨텍스트에 버전 정보 저장
		c.Locals("version_context", versionCtx)

		return c.Next()
	}
}

// PaginationMiddleware는 자동 페이징 처리를 담당합니다
func PaginationMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		paginationCtx := &PaginationContext{
			Page:           1,
			PageSize:       1000, // 기본 페이지 크기
			AutoPagination: false,
			MaxPageSize:    100000, // 최대 10만건
		}

		// 쿼리 파라미터에서 페이징 정보 추출
		if pageStr := c.Query("page"); pageStr != "" {
			if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
				paginationCtx.Page = page
			}
		}

		if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
			if pageSize, err := strconv.Atoi(pageSizeStr); err == nil && pageSize > 0 {
				// 사용자 설정 페이지 크기 제한
				if pageSize > paginationCtx.MaxPageSize {
					return c.Status(400).JSON(fiber.Map{
						"error":          "Page size too large",
						"code":           "PAGINATION_SIZE_EXCEEDED",
						"max_page_size":  paginationCtx.MaxPageSize,
						"requested_size": pageSize,
					})
				}
				paginationCtx.PageSize = pageSize
			}
		}

		// auto_size 파라미터 확인
		if autoSizeStr := c.Query("auto_size"); autoSizeStr == "true" {
			paginationCtx.AutoPagination = true
		}

		// 컨텍스트에 페이징 정보 저장
		c.Locals("pagination_context", paginationCtx)

		return c.Next()
	}
}

// GetVersionContext는 컨텍스트에서 버전 정보를 가져옵니다
func GetVersionContext(c *fiber.Ctx) *VersionContext {
	if ctx := c.Locals("version_context"); ctx != nil {
		return ctx.(*VersionContext)
	}
	return &VersionContext{
		RequestedVersion: "v1",
		TargetVersions:   []string{"1"},
		IsMultiVersion:   false,
	}
}

// GetPaginationContext는 컨텍스트에서 페이징 정보를 가져옵니다
func GetPaginationContext(c *fiber.Ctx) *PaginationContext {
	if ctx := c.Locals("pagination_context"); ctx != nil {
		return ctx.(*PaginationContext)
	}
	return &PaginationContext{
		Page:           1,
		PageSize:       1000,
		AutoPagination: false,
		MaxPageSize:    100000,
	}
}

// AutoPaginationMiddleware는 데이터 크기에 따른 자동 페이징을 처리합니다
func AutoPaginationMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// PaginationMiddleware 실행
		paginationHandler := PaginationMiddleware()
		if err := paginationHandler(c); err != nil {
			return err
		}

		// 사용자가 페이지 크기를 지정했으면 자동 규칙 무시
		if c.Query("page_size") != "" {
			c.Locals("user_specified_pagination", true)
			return c.Next()
		}

		// 카테고리별 데이터 크기 확인 (필요시)
		category := c.Params("category")
		if category != "" {
			paginationCtx := GetPaginationContext(c)

			// 자동 페이징 활성화 조건 확인 (10만건 이상)
			if shouldEnableAutoPagination(c, category) {
				paginationCtx.AutoPagination = true
				// 페이지 크기 자동 조정 (큰 데이터셋은 1000건으로 제한)
				if paginationCtx.PageSize > 1000 {
					paginationCtx.PageSize = 1000
				}
				c.Locals("pagination_context", paginationCtx)
				c.Locals("auto_pagination_applied", true)
			}
		}

		return c.Next()
	}
}

// shouldEnableAutoPagination은 자동 페이징 활성화 여부를 결정합니다
func shouldEnableAutoPagination(c *fiber.Ctx, category string) bool {
	// 조직 ID 가져오기
	orgID, err := GetOrgIDFromToken(c)
	if err != nil {
		return false // 에러 시 안전하게 false 반환
	}

	// 데이터베이스에서 해당 카테고리의 대략적인 데이터 크기 확인
	db := database.GetDB()

	var approxCount int
	query := `
		SELECT COUNT(*) 
		FROM target_categories 
		WHERE org_id = $1 AND category_name = $2
	`

	err = db.QueryRow(query, orgID, category).Scan(&approxCount)
	if err != nil {
		return false // 에러 시 안전하게 false 반환
	}

	// 10만건 이상이면 자동 페이징 활성화
	return approxCount >= 100000
}

// ValidateVersionAccess는 특정 버전에 대한 접근 권한을 확인합니다
func ValidateVersionAccess(c *fiber.Ctx, category string, requestedVersion string) error {
	orgID, err := GetOrgIDFromToken(c)
	if err != nil {
		return err
	}

	db := database.GetDB()

	// 해당 카테고리에서 사용 가능한 버전들 조회
	var availableVersions []string
	query := `
		SELECT DISTINCT schema_version::text 
		FROM target_categories 
		WHERE org_id = $1 AND category_name = $2
		ORDER BY schema_version::int DESC
	`

	rows, err := db.Query(query, orgID, category)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			continue
		}
		availableVersions = append(availableVersions, version)
	}

	if len(availableVersions) == 0 {
		return fiber.NewError(404, "Category not found or no data available")
	}

	// 버전 유효성 검사
	switch requestedVersion {
	case "latest", "all":
		return nil // 항상 허용
	default:
		// v1 -> 1, v2 -> 2 변환
		numericVersion := strings.TrimPrefix(requestedVersion, "v")
		for _, available := range availableVersions {
			if available == numericVersion {
				return nil
			}
		}

		return fiber.NewError(404, "Requested version not available for this category")
	}
}

// VersionMiddleware는 API 버전을 추출하고 검증하는 미들웨어입니다.
func VersionMiddlewareOld() fiber.Handler {
	return func(c *fiber.Ctx) error {
		version := c.Params("version")

		// 버전 검증 및 정규화
		normalizedVersion, err := normalizeVersion(version)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error": fiber.Map{
					"code":      "INVALID_VERSION",
					"message":   "Invalid API version",
					"details":   "Version must be v1, v2, latest, or all",
					"timestamp": time.Now(),
				},
			})
		}

		// Context에 정규화된 버전 저장
		c.Locals("version", normalizedVersion)
		c.Locals("raw_version", version)

		return c.Next()
	}
}

// normalizeVersion은 버전 문자열을 정규화합니다.
func normalizeVersion(version string) (string, error) {
	switch strings.ToLower(version) {
	case "latest":
		return "latest", nil
	case "all":
		return "all", nil
	default:
		// v1, v2 등의 패턴 검증
		if matches := versionPattern.FindStringSubmatch(version); len(matches) == 2 {
			versionNum, err := strconv.Atoi(matches[1])
			if err != nil || versionNum < 1 {
				return "", fiber.NewError(fiber.StatusBadRequest, "Invalid version number")
			}
			return version, nil
		}
		return "", fiber.NewError(fiber.StatusBadRequest, "Invalid version format")
	}
}

// GetVersionFromContext는 context에서 버전 정보를 가져옵니다.
func GetVersionFromContext(c *fiber.Ctx) string {
	version := c.Locals("version")
	if version == nil {
		return "v1" // 기본값
	}
	return version.(string)
}

// GetVersionNumber는 버전 번호를 정수로 반환합니다.
func GetVersionNumber(version string) (int, error) {
	if matches := versionPattern.FindStringSubmatch(version); len(matches) == 2 {
		return strconv.Atoi(matches[1])
	}
	return 0, fiber.NewError(fiber.StatusBadRequest, "Cannot extract version number")
}

// IsLatestVersion은 최신 버전인지 확인합니다.
func IsLatestVersion(version string) bool {
	return strings.ToLower(version) == "latest"
}

// IsAllVersions는 모든 버전을 요청한 것인지 확인합니다.
func IsAllVersions(version string) bool {
	return strings.ToLower(version) == "all"
}
