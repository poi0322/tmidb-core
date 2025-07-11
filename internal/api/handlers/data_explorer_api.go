package handlers

import (
	"encoding/json"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/tmidb/tmidb-core/internal/config"
	"github.com/tmidb/tmidb-core/internal/database"
)

// ExploreData는 데이터 익스플로러의 기본 데이터 목록을 조회합니다.
func ExploreData(c *fiber.Ctx) error {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Error loading config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Configuration error",
		})
	}

	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "org_id is required",
		})
	}

	filter := database.ExplorerDataFilter{
		Category:  c.Query("category"),
		StartDate: c.Query("startDate"),
		EndDate:   c.Query("endDate"),
	}

	data, err := database.GetExplorerData(cfg, orgID, filter)
	if err != nil {
		log.Printf("[ExploreData] orgID=%s, 데이터 조회 실패: %v", orgID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to retrieve data",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

// GetDataCategories는 데이터 익스플로러에서 사용할 카테고리 목록을 조회합니다.
func GetDataCategories(c *fiber.Ctx) error {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Error loading config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Configuration error",
		})
	}

	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "org_id is required",
		})
	}

	categories, err := database.GetCategoriesForExplorer(cfg, orgID)
	if err != nil {
		log.Printf("Error getting categories for explorer for org %s: %v", orgID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to retrieve categories",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    categories,
	})
}

// SearchTargets는 특정 카테고리의 타겟을 검색합니다.
func SearchTargets(c *fiber.Ctx) error {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Error loading config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Configuration error",
		})
	}

	orgID := c.Locals("org_id").(string)
	if orgID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "org_id is required",
		})
	}

	query := c.Query("q")
	if query == "" {
		return c.JSON(fiber.Map{"success": true, "data": []interface{}{}})
	}

	targets, err := database.SearchTargets(cfg, orgID, query)
	if err != nil {
		log.Printf("Error searching targets for org %s: %v", orgID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to search targets",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    targets,
	})
}

// ValidateData는 주어진 데이터가 카테고리 스키마에 맞는지 검증합니다.
func ValidateData(c *fiber.Ctx) error {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Error loading config: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "Configuration error"})
	}

	orgID := c.Locals("org_id").(string)
	categoryName := c.Params("category")
	if orgID == "" || categoryName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "org_id and category are required and org_id must be a valid UUID"})
	}

	var reqBody struct {
		Data json.RawMessage `json:"data"`
	}

	if err := c.BodyParser(&reqBody); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body"})
	}

	if len(reqBody.Data) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Data field is required"})
	}

	result, err := database.ValidateDataAgainstSchema(cfg, orgID, categoryName, reqBody.Data)
	if err != nil {
		log.Printf("Error validating data for org %s, category %s: %v", orgID, categoryName, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to validate data",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    result,
	})
}

// SaveData는 새로운 데이터를 추가하거나 기존 데이터를 수정합니다.
func SaveData(c *fiber.Ctx) error {
	// 1. 파라미터 및 폼 데이터 파싱
	orgID := c.Locals("org_id").(string)
	targetID := c.Params("target_id")
	categoryName := c.Params("category_name")

	if orgID == "" || targetID == "" || categoryName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "org_id, target_id, category_name are required and org_id must be a valid UUID"})
	}

	// 'category_data' 필드는 JSON 문자열로 전송됩니다.
	jsonData := c.FormValue("category_data")
	if jsonData == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "category_data form value is required"})
	}

	// TODO: 파일 핸들링 로직 추가
	// form, err := c.MultipartForm()
	// if err == nil {
	// 	files := form.File["files"]
	// }

	// 2. 데이터베이스 함수 호출
	cfg, err := config.Load()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "Configuration error"})
	}

	// 3. 데이터베이스에 저장
	// 참고: 현재는 간단한 UPSERT만 구현합니다. 시계열, 파일 처리 등은 추가 구현이 필요합니다.
	err = database.SaveExplorerData(cfg, orgID, targetID, categoryName, []byte(jsonData))
	if err != nil {
		log.Printf("Error saving data for org %s: %v", orgID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to save data",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Data saved successfully",
	})
}
