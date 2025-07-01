package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/tmidb/tmidb-core/internal/api/handlers"
	"github.com/tmidb/tmidb-core/internal/api/middleware"
)

// SetupRoutes는 모든 라우팅을 설정합니다
func SetupRoutes(app *fiber.App, sessionStore *session.Store) {
	// 정적 파일 서빙
	app.Static("/static", "./cmd/api/static")

	// 기본 페이지들
	setupBasicRoutes(app, sessionStore)
	
	// 웹 콘솔 (HTML 페이지, 세션 기반)
	setupWebConsoleRoutes(app, sessionStore)
	
	// API 라우팅
	api := app.Group("/api")
	
	// 관리 API (JSON, 세션/토큰 기반)
	setupManagementAPIRoutes(api, sessionStore)
	
	// 일반 데이터 API (JSON, 토큰 기반)
	setupDataAPIRoutes(api)
}

// setupBasicRoutes는 기본 페이지 라우팅을 설정합니다
func setupBasicRoutes(app *fiber.App, sessionStore *session.Store) {
	// 메인 페이지
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Render("main", fiber.Map{
			"Title": "tmiDB Console",
		})
	})

	// 인증 관련
	app.Get("/login", handlers.LoginPage)
	app.Post("/login", handlers.LoginProcess)
	app.Post("/logout", handlers.Logout)
	
	// 초기 설정
	app.Get("/setup", handlers.SetupPage)
	app.Post("/setup", handlers.SetupProcess)
	app.Get("/api/setup/status", handlers.SetupStatus)
}

// setupWebConsoleRoutes는 웹 콘솔 페이지 라우팅을 설정합니다
func setupWebConsoleRoutes(app *fiber.App, sessionStore *session.Store) {
	// 대시보드 (메인)
	app.Get("/dashboard", handlers.DashboardPage)
	
	// 카테고리 관리
	app.Get("/categories", middleware.AuthRequired(sessionStore), handlers.CategoriesPage)
	
	// 리스너 관리  
	app.Get("/listeners", middleware.AuthRequired(sessionStore), handlers.ListenersPage)
	
	// 데이터 탐색기
	app.Get("/data-explorer", middleware.AuthRequired(sessionStore), handlers.DataExplorerPage)
	
	// 파일 관리
	app.Get("/files", middleware.AuthRequired(sessionStore), handlers.FilesPage)
	
	// 사용자 관리 (관리자만)
	app.Get("/users", middleware.AuthRequired(sessionStore), middleware.AdminRequired(sessionStore), handlers.UsersPage)
	app.Get("/tokens", middleware.AuthRequired(sessionStore), middleware.AdminRequired(sessionStore), handlers.TokensPage)
	app.Get("/migrations", middleware.AuthRequired(sessionStore), middleware.AdminRequired(sessionStore), handlers.MigrationsPage)
	app.Get("/logs", middleware.AuthRequired(sessionStore), middleware.AdminRequired(sessionStore), handlers.LogsPage)
}

// setupManagementAPIRoutes는 관리 API 라우팅을 설정합니다
func setupManagementAPIRoutes(api fiber.Router, sessionStore *session.Store) {
	mgmt := api.Group("/manage")
	mgmt.Use(middleware.AuthRequired(sessionStore))
	
	// 대시보드 API
	mgmt.Get("/dashboard/metrics", handlers.DashboardMetrics)
	mgmt.Get("/dashboard/activities", handlers.DashboardActivities)
	mgmt.Get("/dashboard/resources", handlers.DashboardResources)
	mgmt.Get("/dashboard/api-stats", handlers.DashboardApiStats)
	mgmt.Post("/system/check", handlers.SystemCheck)
	mgmt.Post("/cache/clear", handlers.ClearCache)
	
	// 카테고리 관리
	mgmt.Get("/categories", handlers.GetCategoriesAPI)
	mgmt.Post("/categories", handlers.CreateCategoryAPI)
	mgmt.Put("/categories/:name", handlers.UpdateCategoryAPI)
	mgmt.Delete("/categories/:name", handlers.DeleteCategoryAPI)
	mgmt.Get("/categories/:name/schema", handlers.GetCategorySchemaAPI)
	
	// 리스너 관리
	mgmt.Get("/listeners", handlers.GetListenersAPI)
	mgmt.Post("/listeners", handlers.CreateListenerAPI)
	mgmt.Delete("/listeners/:id", handlers.DeleteListenerAPI)
	
	// 사용자 관리 (관리자만)
	mgmtAdmin := mgmt.Group("/", middleware.AdminRequired(sessionStore))
	mgmtAdmin.Get("/users", handlers.GetUsersAPI)
	mgmtAdmin.Post("/users", handlers.CreateUserAPI)
	mgmtAdmin.Put("/users/:id", handlers.UpdateUserAPI)
	mgmtAdmin.Delete("/users/:id", handlers.DeleteUserAPI)
	
	// 토큰 관리
	mgmtAdmin.Get("/tokens", handlers.GetAuthTokensAPI)
	mgmtAdmin.Post("/tokens", handlers.CreateAuthTokenAPI)
	mgmtAdmin.Delete("/tokens/:id", handlers.DeleteAuthTokenAPI)
	
	// 마이그레이션 관리
	mgmtAdmin.Get("/migrations", handlers.GetMigrationsAPI)
	mgmtAdmin.Post("/migrations", handlers.CreateMigrationAPI)
	mgmtAdmin.Post("/migrations/:id/execute", handlers.ExecuteMigrationAPI)
	mgmtAdmin.Get("/migrations/:id/status", handlers.GetMigrationStatusAPI)
}

// setupDataAPIRoutes는 일반 데이터 API 라우팅을 설정합니다
func setupDataAPIRoutes(api fiber.Router) {
	// 헬스체크 (인증 불필요)
	api.Get("/health", handlers.HealthCheck)
	api.Get("/system/info", handlers.SystemInfo)
	
	// 버전별 API 그룹
	setupVersionedRoutes(api, "v1")
	setupVersionedRoutes(api, "v2") 
	setupVersionedRoutes(api, "latest")
	setupVersionedRoutes(api, "all")
}

// setupVersionedRoutes는 특정 버전의 API 라우팅을 설정합니다
func setupVersionedRoutes(api fiber.Router, version string) {
	v := api.Group("/" + version)
	v.Use(middleware.VersionMiddleware(version))
	v.Use(middleware.AutoPaginationMiddleware())
	v.Use(middleware.TokenAuthRequired("read", handlers.CategoryFromParams))
	
	// 카테고리 데이터 API
	v.Get("/category/:category", handlers.GetCategoryData)
	v.Get("/category/:category/schema", handlers.GetCategorySchema)
	
	// 타겟 데이터 API  
	v.Get("/targets/:target_id/categories/:category", handlers.GetTargetByID)
	v.Post("/targets/:target_id/categories/:category", 
		middleware.TokenAuthRequired("write", handlers.CategoryFromParams),
		handlers.CreateOrUpdateTargetData)
	v.Delete("/targets/:target_id/categories/:category",
		middleware.TokenAuthRequired("write", handlers.CategoryFromParams), 
		handlers.DeleteTargetData)
	
	// 시계열 데이터 API
	v.Get("/targets/:target_id/categories/:category/timeseries", handlers.GetTimeSeriesData)
	v.Post("/targets/:target_id/categories/:category/timeseries",
		middleware.TokenAuthRequired("write", handlers.CategoryFromParams),
		handlers.InsertTimeSeriesData)
	
	// 리스너 API
	v.Get("/listener/:listener_id", handlers.GetSingleListenerData)
	v.Get("/listener/*", handlers.GetMultiListenerData) // 다중 리스너 경로
	
	// 파일 관리 API (추후 구현)
	v.Post("/targets/:target_id/categories/:category/files",
		middleware.TokenAuthRequired("write", handlers.CategoryFromParams),
		handlers.UploadFiles)
	v.Delete("/targets/:target_id/categories/:category/files/:file_id",
		middleware.TokenAuthRequired("write", handlers.CategoryFromParams),
		handlers.DeleteFile)
} 