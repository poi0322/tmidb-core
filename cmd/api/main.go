package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	
	"github.com/tmidb/tmidb-core/internal/api/handlers"
	"github.com/tmidb/tmidb-core/internal/api/routes"
	"github.com/tmidb/tmidb-core/internal/database"
	"github.com/tmidb/tmidb-core/internal/migration"
)

func main() {
	log.Println("🌐 Starting tmiDB API Server...")

	// 데이터베이스 연결 초기화
	if err := database.Initialize(); err != nil {
		log.Fatalf("❌ Failed to initialize database: %v", err)
	}
	defer database.Close()

	// 캐시 시스템 초기화
	handlers.InitDataCache()
	log.Println("💾 데이터 캐시 시스템 초기화 완료")

	// 마이그레이션 시스템 초기화
	migrationManager := migration.NewMigrationManager(database.GetDB())
	if err := migrationManager.InitializeMigrationTable(); err != nil {
		log.Fatalf("❌ Failed to initialize migration system: %v", err)
	}
	log.Println("🔧 마이그레이션 시스템 초기화 완료")

	// 세션 스토어 초기화
	sessionStore := session.New(session.Config{
		KeyLookup:      "cookie:session_id",
		CookieDomain:   "",
		CookiePath:     "/",
		CookieSecure:   false,
		CookieHTTPOnly: true,
		CookieSameSite: "Lax",
		Expiration:     24 * time.Hour,
	})

	// 웹 콘솔 템플릿 엔진 초기화
	engine := html.New("./cmd/api/views", ".html")

	// Fiber 앱 생성
	app := fiber.New(fiber.Config{
		Views: engine,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			// 기본 500 에러
			code := fiber.StatusInternalServerError
			
			// Fiber 에러인 경우 상태 코드 추출
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			
			// JSON API 요청인 경우 JSON 에러 응답
			if c.Path() != "/" && (c.Get("Accept") == "application/json" || 
				c.Get("Content-Type") == "application/json" || 
				c.Path() == "/api") {
				return c.Status(code).JSON(fiber.Map{
					"success": false,
					"error": fiber.Map{
						"code":    "INTERNAL_ERROR",
						"message": err.Error(),
					},
					"timestamp": time.Now(),
				})
			}
			
			// HTML 에러 페이지
			return c.Status(code).Render("error", fiber.Map{
				"Title": "Error",
				"Code":  code,
				"Error": err.Error(),
			})
		},
	})

	// 미들웨어 설정
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-Request-ID",
	}))
	
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} - ${latency}\n",
	}))

	// 새로운 라우팅 시스템 사용
	routes.SetupRoutes(app, sessionStore)

	// 서버 시작
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8020"
	}

	go func() {
		log.Printf("🌐 API Server listening on :%s", port)
		if err := app.Listen(":" + port); err != nil {
			log.Fatalf("❌ Failed to start server: %v", err)
		}
	}()

	// 종료 시그널 대기
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down API Server...")

	// 서버 종료
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("❌ Server forced to shutdown: %v", err)
	}

	log.Println("✅ API Server stopped")
}
