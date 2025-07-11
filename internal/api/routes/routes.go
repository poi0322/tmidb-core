package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/tmidb/tmidb-core/internal/api/handlers"
	"github.com/tmidb/tmidb-core/internal/api/middleware"
	"github.com/tmidb/tmidb-core/internal/database"
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
	// /api 하위 모든 라우트에 session_store 미들웨어 적용
	api.Use(func(c *fiber.Ctx) error {
		c.Locals("session_store", sessionStore)
		return c.Next()
	})

	// 관리 API (JSON, 세션/토큰 기반)
	setupManagementAPIRoutes(api, sessionStore)

	// 일반 데이터 API (JSON, 토큰 기반)
	setupDataAPIRoutes(api, sessionStore)
}

// setupBasicRoutes는 기본 페이지 라우팅을 설정합니다
func setupBasicRoutes(app *fiber.App, sessionStore *session.Store) {
	// 메인 페이지 - 초기 설정 상태에 따라 리디렉션
	app.Get("/", func(c *fiber.Ctx) error {
		// 초기 설정 완료 여부 확인
		setupCompleted, err := database.IsSetupCompleted()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Database connection error",
			})
		}

		// 초기 설정이 완료되지 않은 경우 setup 페이지로 리디렉션
		if !setupCompleted {
			return c.Redirect("/setup")
		}

		// 초기 설정이 완료된 경우 세션 확인
		sess, err := sessionStore.Get(c)
		if err != nil {
			return c.Redirect("/login")
		}

		userID := sess.Get("user_id")
		if userID == nil {
			// 로그인하지 않은 사용자는 로그인 페이지로
			return c.Redirect("/login")
		}

		// 로그인된 사용자는 대시보드로
		return c.Redirect("/dashboard")
	})

	// 인증 관련
	app.Get("/login", handlers.LoginPage)
	app.Post("/login", handlers.LoginProcess)
	app.Get("/logout", handlers.LogoutPage)
	app.Post("/logout", handlers.LogoutPage)

	// 초기 설정
	app.Get("/setup", handlers.SetupPage)
	app.Post("/setup", handlers.SetupProcess)
	app.Get("/api/setup/status", handlers.SetupStatus)
	app.Post("/api/setup/organization", handlers.SetupProcess)

	// 테스트 페이지
	app.Get("/test", handlers.TestPage)

	// 조직 상세 페이지
	app.Get("/organization/:org_id", middleware.AuthRequired(sessionStore), handlers.OrganizationPage)
}

// setupWebConsoleRoutes는 웹 콘솔 페이지 라우팅을 설정합니다
func setupWebConsoleRoutes(app *fiber.App, sessionStore *session.Store) {
	// 세션 스토어를 Locals에 설정하는 미들웨어 추가
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("session_store", sessionStore)
		return c.Next()
	})

	// 모든 웹콘솔 라우트에 세션 기반 인증 미들웨어를 일관 적용
	app.Get("/dashboard", middleware.AuthRequired(sessionStore), middleware.OrganizationContext(), handlers.DashboardPage)
	app.Get("/categories", middleware.AuthRequired(sessionStore), middleware.OrganizationContext(), handlers.CategoriesPage)
	app.Get("/categories/migrations", middleware.AuthRequired(sessionStore), middleware.OrganizationContext(), handlers.MigrationsPage)
	app.Get("/listeners", middleware.AuthRequired(sessionStore), middleware.OrganizationContext(), handlers.ListenersPage)
	app.Get("/data-explorer", middleware.AuthRequired(sessionStore), middleware.OrganizationContext(), handlers.DataExplorerPage)
	app.Get("/files", middleware.AuthRequired(sessionStore), middleware.OrganizationContext(), handlers.FilesPage)
	app.Get("/users", middleware.AuthRequired(sessionStore), middleware.AdminRequired(sessionStore), middleware.OrganizationContext(), handlers.UsersPage)
	app.Get("/tokens", middleware.AuthRequired(sessionStore), middleware.AdminRequired(sessionStore), middleware.OrganizationContext(), handlers.TokensPage)
	app.Get("/migrations", middleware.AuthRequired(sessionStore), middleware.AdminRequired(sessionStore), middleware.OrganizationContext(), handlers.MigrationsPage)
	app.Get("/logs", middleware.AuthRequired(sessionStore), middleware.AdminRequired(sessionStore), middleware.OrganizationContext(), handlers.LogsPage)
}

// setupManagementAPIRoutes는 관리 API 라우팅을 설정합니다
func setupManagementAPIRoutes(api fiber.Router, sessionStore *session.Store) {
	mgmt := api.Group("/manage")
	// 기존: mgmt.Use(middleware.AuthRequired(sessionStore))
	// 변경: 토큰 기반 인증만 허용
	mgmt.Use(middleware.UserTokenAuthRequired())
	// 조직 컨텍스트 미들웨어 추가
	mgmt.Use(middleware.OrganizationContext())

	// 조직 컨텍스트 전환 API (세션 수정이 필요하므로 예외적으로 세션 인증 사용)
	mgmt.Post("/session/organization", middleware.AuthRequired(sessionStore), handlers.SwitchOrganization)

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

	// 사용자별 API 토큰 관리 (자신의 토큰만 관리)
	mgmt.Get("/user_tokens", handlers.GetUserTokensAPI)
	mgmt.Post("/user_tokens", handlers.CreateUserTokenAPI)
	mgmt.Delete("/user_tokens/:id", handlers.DeleteUserTokenAPI)

	// 사용자 관리 (관리자만)
	mgmtAdmin := mgmt.Group("/", middleware.AdminTokenRequired())
	mgmtAdmin.Get("/users", handlers.GetUsersAPI)
	mgmtAdmin.Post("/users", handlers.CreateUserAPI)
	mgmtAdmin.Put("/users/:id", handlers.UpdateUserAPI)
	mgmtAdmin.Delete("/users/:id", handlers.DeleteUserAPI)

	// 조직 전체 API 토큰 관리 (관리자만)
	mgmtAdmin.Get("/admin_tokens", handlers.GetAuthTokensAPI)
	mgmtAdmin.Post("/admin_tokens", handlers.CreateAuthTokenAPI)
	mgmtAdmin.Delete("/admin_tokens/:id", handlers.DeleteAuthTokenAPI)

	// 조직 관리 (관리자만)
	mgmtAdmin.Get("/organizations", handlers.GetOrganizationsAPI)
	mgmtAdmin.Get("/organizations/:org_id", handlers.GetOrganizationAPI)
	mgmtAdmin.Put("/organizations/:org_id", handlers.UpdateOrganizationAPI)
	mgmtAdmin.Delete("/organizations/:org_id", handlers.DeleteOrganizationAPI)

	// 마이그레이션 관리
	mgmtAdmin.Get("/migrations", handlers.GetMigrationsAPI)
	mgmtAdmin.Post("/migrations", handlers.CreateMigrationAPI)
	mgmtAdmin.Post("/migrations/:id/execute", handlers.ExecuteMigrationAPI)
	mgmtAdmin.Get("/migrations/:id/status", handlers.GetMigrationStatusAPI)

	// 데이터 탐색 API
	mgmt.Get("/data/explore", handlers.ExploreData)
	mgmt.Get("/data/categories", handlers.GetDataCategories)
	mgmt.Get("/data/targets/search", handlers.SearchTargets)
	mgmt.Post("/data/validate/:category", handlers.ValidateData)
}

// setupDataAPIRoutes는 일반 데이터 API 라우팅을 설정합니다
func setupDataAPIRoutes(api fiber.Router, sessionStore *session.Store) {
	// 헬스체크 (인증 불필요)
	api.Get("/health", handlers.HealthCheck)
	api.Get("/system/info", handlers.SystemInfo)

	// Auth routes
	auth := api.Group("/auth")
	auth.Post("/login", handlers.LoginProcess) // Changed to LoginProcess
	auth.Get("/logout", handlers.LogoutPage)

	// Organization session management
	api.Post("/session/organization", middleware.AuthRequired(sessionStore), handlers.SetOrganizationInSession)
	api.Get("/session/organization", handlers.GetOrganizationFromSession)
	api.Get("/session/token", handlers.GetSessionToken)
	auth.Post("/validate", handlers.ValidateToken)
	auth.Post("/refresh", handlers.RefreshToken)

	// --- 데이터 입수 API (웹 콘솔 데이터 탐색기용) ---
	// 이 API들은 쓰기 권한이 필요하므로 별도의 미들웨어를 설정하거나,
	// SessionOrTokenAuthRequired 미들웨어를 사용하는 /api/manage 그룹으로 옮기는 것을 고려해야 합니다.
	// 우선은 프론트엔드 경로에 맞춰 여기에 추가합니다.
	targets := api.Group("/targets", middleware.TokenAuthRequired("write", handlers.CategoryFromParams))
	targets.Use(middleware.OrganizationContext())
	targets.Post("/:target_id/categories/:category_name", handlers.SaveData)
	targets.Put("/:target_id/categories/:category_name", handlers.SaveData)
	targets.Post("/:target_id/categories/:category_name/timeseries", handlers.SaveData)

	// 미들웨어 설정
	api.Use(middleware.TokenAuthRequired("read", handlers.CategoryFromParams))
	api.Use(middleware.OrganizationContext())
	api.Use(middleware.AutoPaginationMiddleware())

	// --- 데이터 조회 API ---

	// 카테고리 스키마 조회
	api.Get("/schemas/:category", handlers.GetCategorySchema)

	// 카테고리 데이터 조회
	api.Get("/categories/:category", handlers.GetCategoryData)

	// 타겟별 데이터 조회
	api.Get("/targets/:target_id", handlers.GetTargetByID)
	api.Get("/targets/:target_id/categories/:category", handlers.GetTargetCategoryData)

	// --- 데이터 입출력 API ---
	writeAuth := api.Group("/")
	writeAuth.Use(middleware.TokenAuthRequired("write", handlers.CategoryFromParams))

	// 타겟 데이터 생성/수정/삭제
	writeAuth.Post("/targets/:target_id/categories/:category", handlers.CreateOrUpdateTargetData)
	writeAuth.Put("/targets/:target_id/categories/:category", handlers.CreateOrUpdateTargetData) // PUT 추가
	writeAuth.Delete("/targets/:target_id/categories/:category", handlers.DeleteTargetData)

	// 타겟 메타데이터 수정 (관리자 전용)
	adminAuth := writeAuth.Group("/")
	adminAuth.Use(middleware.AdminRoleRequired()) // 관리자 권한 필요
	adminAuth.Put("/targets/:target_id", handlers.UpdateTarget)

	// 시계열 데이터 API
	api.Get("/targets/:target_id/categories/:category/timeseries", handlers.GetTimeSeriesData)
	writeAuth.Post("/targets/:target_id/categories/:category/timeseries", handlers.InsertTimeSeriesData)

	// 시계열 데이터 수정/삭제 (관리자 전용)
	adminAuth.Put("/timeseries/:obs_id", handlers.UpdateTimeSeriesData)
	adminAuth.Delete("/timeseries/:obs_id", handlers.DeleteTimeSeriesData)

	// 리스너 API (기존 유지)
	api.Get("/listener/:listener_id", handlers.GetSingleListenerData)
	api.Get("/listener/*", handlers.GetMultiListenerData)

	// 파일 관리 API
	writeAuth.Post("/targets/:target_id/categories/:category/files", handlers.UploadFiles)
	writeAuth.Delete("/targets/:target_id/categories/:category/files/:file_id", handlers.DeleteFile)
}
