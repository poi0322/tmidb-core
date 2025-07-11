package database

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/tmidb/tmidb-core/internal/config" // config 패키지 임포트
	"golang.org/x/crypto/bcrypt"

	_ "github.com/lib/pq"
)

// 전역 DB 인스턴스
var CoreDB *sql.DB // 코어 데이터베이스 (_core_tmidb)

// GetDB는 전역 데이터베이스 인스턴스를 반환합니다 (호환성을 위해 CoreDB 반환)
func GetDB() *sql.DB {
	return CoreDB
}

// GetCoreDB는 코어 데이터베이스 인스턴스를 반환합니다
func GetCoreDB() *sql.DB {
	return CoreDB
}

// InitDatabase는 데이터베이스 연결을 초기화합니다.
func InitDatabase(cfg *config.Config) error {
	// 1단계: 관리자 권한으로 연결하여 _core_tmidb 전용 사용자 및 데이터베이스 생성
	if err := setupDatabasesAndUser(cfg); err != nil {
		return fmt.Errorf("failed to setup databases and user: %v", err)
	}

	// 2단계: 코어 데이터베이스 연결
	if err := connectToCoreDatabase(cfg); err != nil {
		return fmt.Errorf("failed to connect to core database: %v", err)
	}

	// 3단계: 스키마 초기화
	if err := initializeSchema(); err != nil {
		return fmt.Errorf("failed to initialize schema: %v", err)
	}

	// 4단계: 환경변수 기반 초기 설정 처리
	if err := processInitialSetup(cfg); err != nil {
		return fmt.Errorf("failed to process initial setup: %v", err)
	}

	log.Println("Database connections and setup completed successfully")
	return nil
}

// setupDatabasesAndUser는 관리자 권한으로 데이터베이스들과 사용자를 생성합니다.
func setupDatabasesAndUser(cfg *config.Config) error {
	log.Printf("Connecting to PostgreSQL as admin user '%s' for initial setup", cfg.PostgresUser)

	// postgres 데이터베이스에 관리자로 연결
	adminDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable",
		cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort)

	log.Printf("adminDBURL: %s", adminDBURL)

	adminDB, err := sql.Open("postgres", adminDBURL)
	if err != nil {
		return fmt.Errorf("failed to connect as admin: %v", err)
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping admin database: %v", err)
	}

	// _core_tmidb 데이터베이스 존재 여부 확인
	var coreDBExists bool
	err = adminDB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = '_core_tmidb')").Scan(&coreDBExists)
	if err != nil {
		_ = adminDB.Close() // 에러 발생 시 연결 닫기
		return fmt.Errorf("failed to check _core_tmidb existence: %v", err)
	}

	// _core_tmidb가 없으면 신규 셋업 모드
	if !coreDBExists {
		log.Println("🆕 _core_tmidb database not found - entering fresh setup mode")

		// _core_tmidb 데이터베이스 생성
		_, err = adminDB.Exec(`
			CREATE DATABASE _core_tmidb
			WITH ENCODING = 'UTF8'
			LC_COLLATE = 'en_US.utf8'
			LC_CTYPE = 'en_US.utf8'
			TEMPLATE = template0
		`)
		if err != nil {
			return fmt.Errorf("failed to create _core_tmidb database: %v", err)
		}

		log.Println("✅ Fresh _core_tmidb database created successfully")
	} else {
		// _core_tmidb가 있으면 기존 모드
		log.Println("🔄 _core_tmidb database exists - using existing setup")
	}

	// tmiDB 전용 사용자 생성
	_, err = adminDB.Exec(fmt.Sprintf(`
		DO $$ 
		BEGIN
			IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '%s') THEN
				CREATE USER %s WITH PASSWORD '%s';
			END IF;
		END $$
	`, cfg.TmiDBUser, cfg.TmiDBUser, cfg.TmiDBPassword))
	if err != nil {
		return fmt.Errorf("failed to create tmiDB user: %v", err)
	}

	// 초기 설정을 위한 admin 사용자 생성 (존재하지 않을 경우에만)
	// 이 사용자는 나중에 첫 번째 조직의 관리자가 됩니다.
	if cfg.InitUser != "" && cfg.InitPassword != "" {
		_, err = adminDB.Exec(fmt.Sprintf(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '%s') THEN
				CREATE USER %s WITH PASSWORD '%s' LOGIN;
			END IF;
		END $$
	`, cfg.InitUser, cfg.InitUser, cfg.InitPassword))
		if err != nil {
			return fmt.Errorf("failed to create initial admin user '%s': %v", cfg.InitUser, err)
		}
	}

	// tmiDB 사용자에게 _core_tmidb 데이터베이스 권한 부여
	_, err = adminDB.Exec(fmt.Sprintf(`
		GRANT ALL PRIVILEGES ON DATABASE _core_tmidb TO %[1]s;
		ALTER USER %[1]s WITH SUPERUSER CREATEDB;
	`, cfg.TmiDBUser))
	if err != nil {
		return fmt.Errorf("failed to grant database privileges: %v", err)
	}

	// _core_tmidb 데이터베이스에 연결하여 스키마 권한 부여
	coreDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/_core_tmidb?sslmode=disable",
		cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort)

	coreDB, err := sql.Open("postgres", coreDBURL)
	if err != nil {
		return fmt.Errorf("failed to connect to _core_tmidb database: %v", err)
	}
	defer coreDB.Close()

	// public 스키마 권한 부여
	_, err = coreDB.Exec(fmt.Sprintf(`
		GRANT ALL ON SCHEMA public TO %s;
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO %s;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO %s;
		ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO %s;
		ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO %s;
	`, cfg.TmiDBUser, cfg.TmiDBUser, cfg.TmiDBUser, cfg.TmiDBUser, cfg.TmiDBUser))
	if err != nil {
		return fmt.Errorf("failed to grant _core_tmidb privileges: %v", err)
	}

	log.Printf("Database '_core_tmidb' and user '%s' setup completed", cfg.TmiDBUser)
	return nil
}

// CreateOrganizationDatabase는 setup 시 organization 이름으로 새로운 PostgreSQL 데이터베이스를 생성합니다.
func CreateOrganizationDatabase(orgName, adminUsername, adminPassword string, cfg *config.Config) (orgID, adminUserID string, err error) {
	// 1. 관리자 권한으로 연결
	adminDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable",
		cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort)

	adminDB, err := sql.Open("postgres", adminDBURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to connect as admin: %v", err)
	}
	defer func() {
		if err := adminDB.Close(); err != nil {
			log.Printf("⚠️ Failed to close adminDB connection: %v", err)
		}
	}()

	// 2. organization 이름으로 데이터베이스 생성
	dbName := fmt.Sprintf("tmidb_%s", strings.ToLower(strings.ReplaceAll(orgName, " ", "_")))

	// 데이터베이스 존재 여부 확인
	var dbExists bool
	err = adminDB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&dbExists)
	if err != nil {
		return "", "", fmt.Errorf("failed to check database existence: %v", err)
	}

	if dbExists {
		return "", "", fmt.Errorf("database '%s' already exists", dbName)
	}

	// 롤백을 위한 변수들
	var dbCreated bool

	// 오류 발생 시 롤백 처리
	defer func() {
		if err != nil && dbCreated {
			log.Printf("🔄 Rolling back database creation for '%s'", dbName)

			// 별도의 관리자 연결로 롤백 수행
			rollbackDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable",
				cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort)

			rollbackDB, rollbackErr := sql.Open("postgres", rollbackDBURL)
			if rollbackErr != nil {
				log.Printf("❌ Failed to connect for rollback: %v", rollbackErr)
				return
			}
			defer rollbackDB.Close()

			// 기존 연결 강제 종료
			_, _ = rollbackDB.Exec(fmt.Sprintf(`
				SELECT pg_terminate_backend(pid) 
				FROM pg_stat_activity 
				WHERE datname = '%s' AND pid <> pg_backend_pid()
			`, dbName))

			// 데이터베이스 삭제
			_, rollbackErr = rollbackDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			if rollbackErr != nil {
				log.Printf("❌ Failed to rollback database creation: %v", rollbackErr)
			} else {
				log.Printf("✅ Successfully rolled back database '%s'", dbName)
			}
		}
	}()

	// 데이터베이스 생성
	_, err = adminDB.Exec(fmt.Sprintf(`
		CREATE DATABASE %s
		WITH ENCODING = 'UTF8'
		LC_COLLATE = 'en_US.utf8'
		LC_CTYPE = 'en_US.utf8'
		TEMPLATE = template0
	`, dbName))
	if err != nil {
		return "", "", fmt.Errorf("failed to create database '%s': %v", dbName, err)
	}
	dbCreated = true

	// 3. tmiDB 사용자에게 새 데이터베이스 권한 부여
	_, err = adminDB.Exec(fmt.Sprintf(`
		GRANT ALL PRIVILEGES ON DATABASE %s TO %s;
	`, dbName, cfg.TmiDBUser))
	if err != nil {
		return "", "", fmt.Errorf("failed to grant database privileges: %v", err)
	}

	// 4. 새 데이터베이스에 연결하여 스키마 초기화
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort, dbName)

	newDB, err := sql.Open("postgres", dbURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to connect to new database: %v", err)
	}
	defer func() {
		if err := newDB.Close(); err != nil {
			log.Printf("⚠️ Failed to close newDB connection: %v", err)
		}
	}()

	// 5. 스키마 및 함수 생성 (마이그레이션 사용)
	if err = executeMigrations(newDB, orgMigrations, "sql/migrations/org"); err != nil {
		return "", "", fmt.Errorf("failed to apply organization migrations: %v", err)
	}

	// 조직 DB용 함수 생성
	if _, err = newDB.Exec(orgFunctionsSQL); err != nil {
		return "", "", fmt.Errorf("failed to create organization functions: %w", err)
	}

	// 7. 코어 DB 트랜잭션 시작 - 이 시점부터는 코어 DB 롤백만 처리
	tx, err := CoreDB.Begin()
	if err != nil {
		return "", "", fmt.Errorf("failed to begin transaction for core db operations: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered in CreateOrganizationDatabase defer: %v", r)
		}
		if err := tx.Rollback(); err != nil {
			log.Printf("⚠️ Failed to rollback core DB transaction: %v", err)
		}
	}()

	// 8. 코어 DB에 organization 정보 저장
	err = tx.QueryRow("INSERT INTO organizations (name) VALUES ($1) RETURNING org_id", orgName).Scan(&orgID)
	if err != nil {
		return "", "", fmt.Errorf("failed to insert organization into core db: %w", err)
	}

	// 6. 기본 리스너 생성 (orgID 생성 후)
	if err = createDefaultListeners(newDB, orgID); err != nil {
		return "", "", fmt.Errorf("failed to create default listeners: %v", err)
	}

	// 9. 코어 DB에 관리자 사용자 생성
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", "", fmt.Errorf("failed to hash password: %w", err)
	}
	err = tx.QueryRow(
		"INSERT INTO users (org_id, username, password_hash, role) VALUES ($1, $2, $3, 'admin') RETURNING user_id",
		orgID, adminUsername, string(hashedPassword),
	).Scan(&adminUserID)
	if err != nil {
		return "", "", fmt.Errorf("failed to insert admin user into core db: %w", err)
	}

	// 10. 사용자-조직 매핑
	_, err = tx.Exec("INSERT INTO user_organizations (user_id, org_id) VALUES ($1, $2)", adminUserID, orgID)
	if err != nil {
		return "", "", fmt.Errorf("failed to link user to organization in core db: %w", err)
	}

	// 11. 조직 데이터베이스 정보 저장
	log.Printf("Attempting to insert into organization_databases for org_id: %s, db_name: %s", orgID, dbName)
	_, err = tx.Exec("INSERT INTO organization_databases (org_id, database_name) VALUES ($1, $2)", orgID, dbName)
	if err != nil {
		log.Printf("ERROR: Failed to insert into organization_databases: %v", err)
		return "", "", fmt.Errorf("failed to insert organization database info into core db: %w", err)
	}
	log.Printf("Successfully inserted into organization_databases for org_id: %s", orgID)

	if err = tx.Commit(); err != nil {
		log.Printf("ERROR: Failed to commit final transaction: %v", err)
		return "", "", fmt.Errorf("failed to commit core db transaction: %w", err)
	}

	log.Printf("✅ Organization database '%s' created successfully with ID: %s", dbName, orgID)
	return orgID, adminUserID, nil
}

// RollbackOrganizationCreation은 조직 생성 후 오류 발생 시 전체 롤백을 수행합니다.
func RollbackOrganizationCreation(orgID, dbName string, cfg *config.Config) error {
	log.Printf("🔄 Starting rollback for organization ID '%s' and database '%s'", orgID, dbName)

	// 1. 코어 DB에서 조직 관련 데이터 삭제
	tx, err := CoreDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin rollback transaction: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered in RollbackOrganizationCreation defer: %v", r)
		}
		if err := tx.Rollback(); err != nil {
			log.Printf("⚠️ Failed to rollback core DB transaction: %v", err)
		}
	}()

	// 조직 데이터베이스 정보 삭제
	_, err = tx.Exec("DELETE FROM organization_databases WHERE org_id = $1", orgID)
	if err != nil {
		log.Printf("⚠️ Failed to delete organization_databases record: %v", err)
	}

	// 사용자-조직 매핑 삭제
	_, err = tx.Exec("DELETE FROM user_organizations WHERE org_id = $1", orgID)
	if err != nil {
		log.Printf("⚠️ Failed to delete user_organizations records: %v", err)
	}

	// 사용자 삭제
	_, err = tx.Exec("DELETE FROM users WHERE org_id = $1", orgID)
	if err != nil {
		log.Printf("⚠️ Failed to delete users: %v", err)
	}

	// 인증 토큰 삭제
	_, err = tx.Exec("DELETE FROM auth_tokens WHERE org_id = $1", orgID)
	if err != nil {
		log.Printf("⚠️ Failed to delete auth_tokens: %v", err)
	}

	// 조직 삭제
	_, err = tx.Exec("DELETE FROM organizations WHERE org_id = $1", orgID)
	if err != nil {
		log.Printf("⚠️ Failed to delete organization: %v", err)
	}

	if err = tx.Commit(); err != nil {
		log.Printf("⚠️ Failed to commit rollback transaction: %v", err)
	}

	// 2. 조직 데이터베이스 삭제
	adminDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable",
		cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort)

	adminDB, err := sql.Open("postgres", adminDBURL)
	if err != nil {
		return fmt.Errorf("failed to connect for database rollback: %v", err)
	}
	defer func() {
		if err := adminDB.Close(); err != nil {
			log.Printf("⚠️ Failed to close adminDB connection during rollback: %v", err)
		}
	}()

	// 기존 연결 강제 종료
	_, _ = adminDB.Exec(fmt.Sprintf(`
		SELECT pg_terminate_backend(pid) 
		FROM pg_stat_activity 
		WHERE datname = '%s' AND pid <> pg_backend_pid()
	`, dbName))

	// 데이터베이스 삭제
	_, err = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
	if err != nil {
		return fmt.Errorf("failed to drop database '%s': %v", dbName, err)
	}

	log.Printf("✅ Rollback completed for organization ID '%s'", orgID)
	return nil
}

// GetOrganizationDatabase는 organization ID로 해당 데이터베이스 이름을 조회합니다.
func GetOrganizationDatabase(orgID string) (string, error) {
	var dbName string
	err := CoreDB.QueryRow(`
		SELECT database_name 
		FROM organization_databases 
		WHERE org_id = $1
	`, orgID).Scan(&dbName)
	if err != nil {
		return "", fmt.Errorf("failed to get organization database: %v", err)
	}
	return dbName, nil
}

// ConnectToOrganizationDatabase는 특정 organization의 데이터베이스에 연결합니다.
func ConnectToOrganizationDatabase(orgID string, cfg *config.Config) (*sql.DB, error) {
	// 1. organization 데이터베이스 이름 조회
	dbName, err := GetOrganizationDatabase(orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization database name: %v", err)
	}

	// 2. 데이터베이스 연결
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.TmiDBUser, cfg.TmiDBPassword, cfg.PostgresHost, cfg.PostgresPort, dbName)

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %v", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	return db, nil
}

// WaitForTable은 특정 테이블이 나타날 때까지 대기합니다.
// data-manager, data-consumer가 api 서버의 마이그레이션을 기다리기 위해 사용합니다.
func WaitForTable(db *sql.DB, tableName string) error {
	maxRetries := 12
	for i := 0; i < maxRetries; i++ {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE  table_schema = 'public'
			AND    table_name   = $1
		);`
		err := db.QueryRow(query, tableName).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to query for table %s: %w", tableName, err)
		}
		if exists {
			log.Printf("✅ Table '%s' found.", tableName)
			return nil
		}
		log.Printf("⏳ Waiting for table '%s' to be created... (attempt %d/%d)", tableName, i+1, maxRetries)
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("table '%s' not found after waiting", tableName)
}

// createDefaultListeners는 새로운 조직 데이터베이스에 기본 리스너를 생성합니다.
func createDefaultListeners(db *sql.DB, orgID string) error {
	defaultListeners := map[string]string{
		"System Health":   "Monitors basic system health metrics.",
		"Network Events":  "Captures network-related events and logs.",
		"Security Alerts": "Handles security-related alerts and incidents.",
	}

	for name, desc := range defaultListeners {
		_, err := db.Exec(`
			INSERT INTO listeners (org_id, category_name, description, is_active)
			VALUES ($1, $2, $3, TRUE)
			ON CONFLICT (org_id, category_name) DO NOTHING
		`, orgID, name, desc)
		if err != nil {
			return fmt.Errorf("failed to create default listener '%s': %w", name, err)
		}
	}
	log.Printf("✅ Created default listeners for organization ID '%s'", orgID)
	return nil
}

// SaveOrganizationToCore는 코어 DB에 organization과 admin 사용자 정보를 저장합니다.
// 생성된 admin user의 ID를 반환합니다.
func SaveOrganizationToCore(orgName, dbName string) (string, error) {
	tx, err := CoreDB.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. 조직 정보 저장
	var orgID string
	err = tx.QueryRow("INSERT INTO organizations (name) VALUES ($1) RETURNING org_id", orgName).Scan(&orgID)
	if err != nil {
		return "", fmt.Errorf("failed to insert organization: %w", err)
	}

	// 2. 조직 데이터베이스 정보 저장
	_, err = tx.Exec("INSERT INTO organization_databases (org_id, database_name) VALUES ($1, $2)", orgID, dbName)
	if err != nil {
		return "", fmt.Errorf("failed to insert organization database info: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return orgID, nil
}

// processInitialSetup은 환경변수를 기반으로 초기 설정을 처리합니다.
func processInitialSetup(cfg *config.Config) error {
	// TMIDB_INIT_MODE가 "setup"이 아니면 건너뛰기
	if cfg.InitMode != "setup" {
		log.Println("🔄 No initial setup mode specified, skipping automatic setup")
		return nil
	}

	// 필수 환경변수 확인
	if cfg.InitOrg == "" || cfg.InitUser == "" || cfg.InitPassword == "" {
		log.Println("⚠️  Initial setup mode enabled but missing required environment variables")
		log.Println("    Required: TMIDB_INIT_ORG, TMIDB_INIT_USER, TMIDB_INIT_PASSWORD")
		return nil
	}

	log.Printf("🚀 Processing initial setup for organization '%s'", cfg.InitOrg)

	// 조직이 이미 존재하는지 확인
	var existingOrgID string
	err := CoreDB.QueryRow("SELECT org_id FROM organizations WHERE name = $1", cfg.InitOrg).Scan(&existingOrgID)
	if err == nil {
		log.Printf("✅ Organization '%s' already exists, skipping setup", cfg.InitOrg)
		return nil
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check organization existence: %v", err)
	}

	// 조직 데이터베이스 생성
	orgID, _, err := CreateOrganizationDatabase(cfg.InitOrg, cfg.InitUser, cfg.InitPassword, cfg)
	if err != nil {
		return fmt.Errorf("failed to create organization database: %v", err)
	}

	log.Printf("✅ Organization '%s' (ID: %s) created successfully", cfg.InitOrg, orgID)

	// 초기 관리자 토큰 처리
	if cfg.InitAdminToken != "" {
		if err := saveInitialAdminToken(orgID, cfg.InitAdminToken); err != nil {
			log.Printf("⚠️  Failed to save initial admin token: %v", err)
		} else {
			log.Println("✅ Initial admin token saved successfully")
		}
	}

	return nil
}

// saveInitialAdminToken은 환경변수로 제공된 초기 관리자 토큰을 저장합니다.
func saveInitialAdminToken(orgID, tokenString string) error {
	// 토큰 암호화
	encryptedToken, err := EncryptToken(tokenString)
	if err != nil {
		return fmt.Errorf("failed to encrypt token: %v", err)
	}

	// 관리자 권한으로 토큰 저장
	_, err = CoreDB.Exec(`
		INSERT INTO auth_tokens (org_id, encrypted_token, description, permissions, is_admin, is_active)
		VALUES ($1, $2, 'Initial admin token (from environment)', '{"admin": true}', TRUE, TRUE)
		ON CONFLICT (encrypted_token) DO NOTHING
	`, orgID, encryptedToken)

	return err
}

// connectToCoreDatabase는 코어 데이터베이스에 연결합니다.
func connectToCoreDatabase(cfg *config.Config) error {
	log.Printf("Connected to _core_tmidb database as user '%s'", cfg.TmiDBUser)
	coreDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/_core_tmidb?sslmode=disable",
		cfg.TmiDBUser, cfg.TmiDBPassword, cfg.PostgresHost, cfg.PostgresPort)

	var err error
	CoreDB, err = sql.Open("postgres", coreDBURL)
	if err != nil {
		return fmt.Errorf("failed to connect to _core_tmidb database: %v", err)
	}

	// 연결 테스트
	if err := CoreDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping _core_tmidb database: %v", err)
	}

	// 연결 풀 설정
	CoreDB.SetMaxOpenConns(10)
	CoreDB.SetMaxIdleConns(3)

	return nil
}

// initializeSchema는 데이터베이스 스키마를 초기화합니다.
func initializeSchema() error {
	// 완전한 스키마 초기화 수행
	return InitializeCompleteSchema()
}

// CloseDatabase는 데이터베이스 연결을 종료합니다.
func CloseDatabase() error {
	if CoreDB != nil {
		if err := CoreDB.Close(); err != nil {
			return fmt.Errorf("failed to close _core_tmidb connection: %w", err)
		}
	}
	return nil
}

// ExecuteFunction은 데이터베이스 함수를 실행하는 헬퍼 함수입니다.
func ExecuteFunction(functionName string, args ...any) (*sql.Rows, error) {
	placeholders := make([]string, len(args))
	for i := range args {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	query := fmt.Sprintf("SELECT * FROM %s(%s)", functionName, strings.Join(placeholders, ", "))
	return CoreDB.Query(query, args...)
}

// CheckDatabaseHealth는 데이터베이스 상태를 확인합니다.
func CheckDatabaseHealth() error {
	if CoreDB == nil {
		return fmt.Errorf("_core_tmidb connection is nil")
	}
	if err := CoreDB.Ping(); err != nil {
		return fmt.Errorf("_core_tmidb: %v", err)
	}
	return nil
}

// Close는 데이터베이스 연결을 닫습니다
func Close() {
	CloseDatabase()
}

// InitializeSchema는 데이터베이스 스키마를 초기화합니다
func InitializeSchema() error {
	return initializeSchema()
}

// ConnectDatabase는 기존 데이터베이스에 연결만 합니다 (초기화 없이)
func ConnectDatabase(cfg *config.Config) error {
	// 최대 120초 동안 재시도 (2초 간격으로 60번) - API 서버가 초기화를 완료할 때까지 기다림
	maxRetries := 60
	retryInterval := 2 * time.Second

	log.Printf("🔄 Waiting for _core_tmidb database to be ready...")

	for i := 0; i < maxRetries; i++ {
		var err error

		// 먼저 사용자가 존재하는지 확인
		adminDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable",
			cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort)

		adminDB, err := sql.Open("postgres", adminDBURL)
		if err != nil {
			log.Printf("⏳ Failed to connect to postgres (attempt %d/%d): %v", i+1, maxRetries, err)
			time.Sleep(retryInterval)
			continue
		}

		// 사용자 존재 여부 확인
		var userExists bool
		err = adminDB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", cfg.TmiDBUser).Scan(&userExists)
		if err != nil {
			adminDB.Close()
			log.Printf("⏳ Failed to check user existence (attempt %d/%d): %v", i+1, maxRetries, err)
			time.Sleep(retryInterval)
			continue
		}

		if !userExists {
			adminDB.Close()
			log.Printf("⏳ User '%s' does not exist yet, waiting for API server setup (attempt %d/%d)", cfg.TmiDBUser, i+1, maxRetries)
			time.Sleep(retryInterval)
			continue
		}

		// 데이터베이스 존재 여부 확인
		var dbExists bool
		err = adminDB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = '_core_tmidb')").Scan(&dbExists)
		adminDB.Close()

		if err != nil {
			log.Printf("⏳ Failed to check database existence (attempt %d/%d): %v", i+1, maxRetries, err)
			time.Sleep(retryInterval)
			continue
		}

		if !dbExists {
			log.Printf("⏳ Database '_core_tmidb' does not exist yet, waiting for API server setup (attempt %d/%d)", i+1, maxRetries)
			time.Sleep(retryInterval)
			continue
		}

		// _core_tmidb 연결
		coreDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/_core_tmidb?sslmode=disable",
			cfg.TmiDBUser, cfg.TmiDBPassword, cfg.PostgresHost, cfg.PostgresPort)

		CoreDB, err = sql.Open("postgres", coreDBURL)
		if err != nil {
			log.Printf("⏳ Failed to open _core_tmidb connection (attempt %d/%d): %v", i+1, maxRetries, err)
			CoreDB = nil
			time.Sleep(retryInterval)
			continue
		}

		// _core_tmidb 연결 테스트
		if err := CoreDB.Ping(); err != nil {
			log.Printf("⏳ Failed to ping _core_tmidb (attempt %d/%d): %v", i+1, maxRetries, err)
			CoreDB.Close()
			CoreDB = nil
			time.Sleep(retryInterval)
			continue
		}

		// 연결 풀 설정
		CoreDB.SetMaxOpenConns(25)
		CoreDB.SetMaxIdleConns(5)

		log.Printf("✅ Connected to _core_tmidb database as user '%s' (attempt %d)", cfg.TmiDBUser, i+1)
		return nil
	}

	return fmt.Errorf("failed to connect to _core_tmidb database after %d attempts (waited %v)", maxRetries, time.Duration(maxRetries)*retryInterval)
}

// getAdminDSN은 주어진 DSN의 데이터베이스 이름을 'postgres'로 변경합니다.
func getAdminDSN(dsn string) (string, error) {
	parsedURL, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("could not parse database URL: %w", err)
	}

	// 경로에서 데이터베이스 이름을 변경합니다.
	parsedURL.Path = "postgres"

	return parsedURL.String(), nil
}

// ConnectToAdminDatabase는 admin 데이터베이스에 연결하여 DB 생성/삭제 등의 작업을 수행합니다.
func ConnectToAdminDatabase() (*sql.DB, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config for admin connection: %w", err)
	}

	// postgres 데이터베이스에 연결하기 위해 DSN을 수정합니다.
	// 이 DSN은 슈퍼유저 권한을 가정합니다.
	adminDSN, err := getAdminDSN(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to construct admin DSN: %w", err)
	}

	db, err := sql.Open("postgres", adminDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open admin db connection: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping admin db: %w", err)
	}

	return db, nil
}
