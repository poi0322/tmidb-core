package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover" // recover 미들웨어 import
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	"github.com/tmidb/tmidb-core/internal/config"

	"github.com/tmidb/tmidb-core/internal/api/handlers"
	"github.com/tmidb/tmidb-core/internal/api/routes"
	"github.com/tmidb/tmidb-core/internal/database"
	"github.com/tmidb/tmidb-core/internal/migration"
)

func main() {
	log.Println("🌐 Starting tmiDB API Server [/app/bin]...")

	// 설정 로드
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	// 데이터베이스 연결 초기화
	if err := database.InitDatabase(cfg); err != nil {
		log.Fatalf("❌ Failed to initialize database: %v", err)
	}
	defer database.Close()

	// 환경변수를 확인하여 초기 설정 실행
	initMode := os.Getenv("TMIDB_INIT_MODE")
	initUser := os.Getenv("TMIDB_INIT_USER")
	initPass := os.Getenv("TMIDB_INIT_PASSWORD")
	initOrg := os.Getenv("TMIDB_INIT_ORG")

	if initMode == "setup" && initUser != "" && initPass != "" && initOrg != "" {
		completed, err := database.IsSetupCompletedForOrg(initOrg)
		if err != nil && !strings.Contains(err.Error(), "no rows in result set") {
			log.Fatalf("❌ Failed to check setup status for org '%s': %v", initOrg, err)
		}

		if !completed {
			log.Printf("🚀 Starting initial setup for organization: %s", initOrg)
			_, _, err := database.CreateOrganizationDatabase(initOrg, initUser, initPass, cfg)
			if err != nil {
				// 이미 존재하는 경우, 오류로 처리하지 않고 경고만 출력
				if strings.Contains(err.Error(), "already exists") {
					log.Printf("⚠️ Organization DB '%s' already exists. Ensuring core records are synced.", initOrg)
					dbName := fmt.Sprintf("tmidb_%s", strings.ToLower(strings.ReplaceAll(initOrg, " ", "_")))
					if _, err := database.SaveOrganizationToCore(initOrg, dbName); err != nil {
						log.Fatalf("❌ Failed to sync organization records for '%s': %v", initOrg, err)
					}
				} else {
					log.Fatalf("❌ Failed to create initial organization and admin user: %v", err)
				}
			} else {
				log.Printf("✅ Successfully created organization '%s' and admin user '%s'", initOrg, initUser)
				if err := database.SetSetupCompletedForOrg(initOrg); err != nil {
					log.Fatalf("❌ Failed to mark setup as complete for org '%s': %v", initOrg, err)
				}
			}
		} else {
			log.Printf("✅ Initial setup for organization '%s' is already complete.", initOrg)
		}
	}

	log.Println("🗃️ 데이터베이스 스키마 초기화 완료")

	// 암호화 모듈 초기화
	if err := database.InitCrypto(cfg.EncryptionKey); err != nil {
		log.Fatalf("❌ Failed to initialize crypto: %v", err)
	}
	log.Println("🔐 암호화 모듈 초기화 완료")

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
	log.Println("🔧 Initializing template engine...")
	engine := html.New("./cmd/api/views", ".html")
	engine.Reload(true) // 개발 모드에서 자동 리로드 활성화

	// 템플릿 함수 추가
	engine.AddFunc("upper", func(s string) string {
		return strings.ToUpper(s)
	})
	engine.AddFunc("substr", func(s string, start, length int) string {
		if start < 0 {
			start = 0
		}
		if start > len(s) {
			return ""
		}
		runes := []rune(s)
		if start+length > len(runes) {
			return string(runes[start:])
		}
		return string(runes[start : start+length])
	})

	log.Println("✅ Template engine initialized")

	// Fiber 앱 생성
	app := fiber.New(fiber.Config{
		Views:                 engine,
		DisableStartupMessage: true, // Fiber 시작 메시지 비활성화
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
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	})) // 패닉 복구 미들웨어 추가

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-Request-ID",
	}))

	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} - ${latency}\n",
	}))

	// 세션 스토어를 전역으로 설정
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("session_store", sessionStore)
		return c.Next()
	})

	// 새로운 라우팅 시스템 사용
	routes.SetupRoutes(app, sessionStore)

	// 서버 시작
	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "8020" // 기본값
	}

	log.Printf("✅ tmiDB API Server is listening on port %s", apiPort)
	if err := app.Listen(":" + apiPort); err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}

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
