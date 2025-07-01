package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// 임시 스텁 핸들러들 - 삭제된 파일들의 함수 대체용

// 페이지 핸들러 스텁들
func FilesPage(c *fiber.Ctx) error {
	return c.SendString("Files page - Coming Soon")
}

func MigrationsPage(c *fiber.Ctx) error {
	return c.SendString("Migrations page - Coming Soon")
}

func LogsPage(c *fiber.Ctx) error {
	return c.SendString("Logs page - Coming Soon")
}

// 대시보드 API 스텁들
func DashboardMetrics(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"total_targets":    0,
		"total_categories": 0,
		"today_api_calls":  0,
		"cache_hit_rate":   0.0,
	})
}

func DashboardActivities(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"activities": []interface{}{},
	})
}

func DashboardResources(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"cpu_usage":    0.0,
		"memory_usage": 0.0,
		"disk_usage":   0.0,
	})
}

func DashboardApiStats(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"endpoints": []interface{}{},
	})
}

func SystemCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"database": "ok",
		"services": "ok",
	})
}

func ClearCache(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Cache cleared",
	})
}

// 리스너 API 스텁들
func GetListenersAPI(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"listeners": []interface{}{},
	})
}

func CreateListenerAPI(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Listener created",
	})
}

func DeleteListenerAPI(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Listener deleted",
	})
}

// 사용자 API와 토큰 API는 다른 파일에 이미 구현됨

// 마이그레이션 API 스텁들
func GetMigrationsAPI(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"migrations": []interface{}{},
	})
}

func CreateMigrationAPI(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Migration created",
	})
}

func ExecuteMigrationAPI(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Migration executed",
	})
}

func GetMigrationStatusAPI(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "completed",
	})
}

// 헬퍼 함수들은 다른 파일에 이미 구현됨

func UploadFiles(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Files uploaded",
	})
}

func DeleteFile(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"success": true,
		"message": "File deleted",
	})
}
