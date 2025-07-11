package database

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq" // PostgreSQL 드라이버
	"github.com/tmidb/tmidb-core/internal/config"
)

//go:embed all:sql/migrations/core
var coreMigrations embed.FS

//go:embed all:sql/migrations/org
var orgMigrations embed.FS

// InitCoreDatabase 코어 데이터베이스를 초기화합니다.
func InitCoreDatabase() (*sql.DB, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

// CategorySchema는 카테고리 스키마 테이블의 Go 표현입니다.
type CategorySchema struct {
	SchemaID         string    `json:"schema_id"`
	OrgID            string    `json:"org_id"`
	CategoryName     string    `json:"category_name"`
	Version          int       `json:"version"`
	SchemaDefinition string    `json:"schema_definition"`
	IsActive         bool      `json:"is_active"`
	IsTimeseries     bool      `json:"is_timeseries"` // 이 줄을 추가합니다.
	CreatedAt        time.Time `json:"created_at"`
}

// GetCategories는 특정 조직의 모든 카테고리를 조회합니다.
func (conn *OrgDBConnection) GetCategories() ([]CategorySchema, error) {
	rows, err := conn.DB.Query("SELECT schema_id, org_id, category_name, version, schema_definition, is_active, is_timeseries, created_at FROM category_schemas WHERE org_id = $1 ORDER BY category_name, version DESC", conn.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []CategorySchema
	for rows.Next() {
		var c CategorySchema
		if err := rows.Scan(&c.SchemaID, &c.OrgID, &c.CategoryName, &c.Version, &c.SchemaDefinition, &c.IsActive, &c.IsTimeseries, &c.CreatedAt); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, nil
}

// CreateCategory는 새 카테고리 스키마를 생성합니다.
func (conn *OrgDBConnection) CreateCategory(category *CategorySchema) error {
	category.Version = 1        // 새 버전은 항상 1로 시작
	category.OrgID = conn.OrgID // OrgID를 conn에서 가져와 설정

	err := conn.DB.QueryRow(
		`INSERT INTO category_schemas (org_id, category_name, version, schema_definition, is_active, is_timeseries)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING schema_id, created_at`,
		category.OrgID, category.CategoryName, category.Version, category.SchemaDefinition, category.IsActive, category.IsTimeseries,
	).Scan(&category.SchemaID, &category.CreatedAt)
	return err
}

// UpdateCategory는 기존 카테고리 스키마의 새 버전을 생성합니다.
func (conn *OrgDBConnection) UpdateCategory(category *CategorySchema) error {
	tx, err := conn.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 현재 최신 버전 조회
	var currentVersion int
	err = tx.QueryRow(
		"SELECT version FROM category_schemas WHERE org_id = $1 AND category_name = $2 ORDER BY version DESC LIMIT 1",
		conn.OrgID, category.CategoryName,
	).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("could not find current version for category %s: %w", category.CategoryName, err)
	}

	// 새 버전 설정
	category.Version = currentVersion + 1
	category.OrgID = conn.OrgID // OrgID 설정

	// 새 버전 삽입
	err = tx.QueryRow(
		`INSERT INTO category_schemas (org_id, category_name, version, schema_definition, is_active, is_timeseries)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING schema_id, created_at`,
		category.OrgID, category.CategoryName, category.Version, category.SchemaDefinition, category.IsActive, category.IsTimeseries,
	).Scan(&category.SchemaID, &category.CreatedAt)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteCategory는 특정 조직에서 카테고리를 삭제합니다.
func (conn *OrgDBConnection) DeleteCategory(name string) error {
	// TODO: 해당 카테고리를 사용하는 타겟이 있는지 확인하는 로직 추가 필요
	_, err := conn.DB.Exec("DELETE FROM category_schemas WHERE category_name = $1 AND org_id = $2", name, conn.OrgID)
	return err
}

// GetCategorySchema는 특정 조직의 카테고리 스키마(최신 버전)를 조회합니다.
func (conn *OrgDBConnection) GetCategorySchema(name string) (*CategorySchema, error) {
	var c CategorySchema
	err := conn.DB.QueryRow(
		`SELECT schema_id, org_id, category_name, version, schema_definition, is_active, is_timeseries, created_at
		 FROM category_schemas 
		 WHERE org_id = $1 AND category_name = $2
		 ORDER BY version DESC LIMIT 1`,
		conn.OrgID, name,
	).Scan(&c.SchemaID, &c.OrgID, &c.CategoryName, &c.Version, &c.SchemaDefinition, &c.IsActive, &c.IsTimeseries, &c.CreatedAt)

	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCategorySchemaByVersion는 특정 버전의 카테고리 스키마를 조회합니다.
func (conn *OrgDBConnection) GetCategorySchemaByVersion(name string, version int) (*CategorySchema, error) {
	var c CategorySchema
	err := conn.DB.QueryRow(
		`SELECT schema_id, org_id, category_name, version, schema_definition, is_active, is_timeseries, created_at
		 FROM category_schemas 
		 WHERE org_id = $1 AND category_name = $2 AND version = $3`,
		conn.OrgID, name, version,
	).Scan(&c.SchemaID, &c.OrgID, &c.CategoryName, &c.Version, &c.SchemaDefinition, &c.IsActive, &c.IsTimeseries, &c.CreatedAt)

	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Listener는 리스너 테이블의 Go 표현입니다.
type Listener struct {
	ListenerID   string    `json:"listener_id"`
	OrgID        string    `json:"org_id"`
	CategoryName string    `json:"category_name"`
	Description  string    `json:"description"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// GetListeners는 특정 조직의 모든 리스너를 조회합니다.
func (conn *OrgDBConnection) GetListeners() ([]Listener, error) {
	rows, err := conn.DB.Query("SELECT listener_id, org_id, category_name, description, is_active, created_at FROM listeners WHERE org_id = $1 ORDER BY created_at DESC", conn.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var listeners []Listener
	for rows.Next() {
		var l Listener
		if err := rows.Scan(&l.ListenerID, &l.OrgID, &l.CategoryName, &l.Description, &l.IsActive, &l.CreatedAt); err != nil {
			return nil, err
		}
		listeners = append(listeners, l)
	}
	return listeners, nil
}

// CreateListener는 새 리스너를 생성합니다.
func (conn *OrgDBConnection) CreateListener(listener *Listener) error {
	_, err := conn.DB.Exec(
		`INSERT INTO listeners (listener_id, org_id, category_name, description, is_active)
		 VALUES ($1, $2, $3, $4, $5)`, // is_active를 파라미터로 받도록 수정
		listener.ListenerID, listener.OrgID, listener.CategoryName, listener.Description, listener.IsActive,
	)
	return err
}

// DeleteListener는 특정 조직에서 리스너를 삭제합니다.
func (conn *OrgDBConnection) DeleteListener(id string) error { // orgID는 conn.OrgID 사용
	_, err := conn.DB.Exec("DELETE FROM listeners WHERE listener_id = $1 AND org_id = $2", id, conn.OrgID)
	return err
}

// executeMigrations는 지정된 embed.FS에서 SQL 마이그레이션을 실행합니다.
func executeMigrations(db *sql.DB, fs embed.FS, migrationsPath string) error {
	files, err := fs.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("could not read migrations directory '%s': %w", migrationsPath, err)
	}

	// 파일명을 기준으로 정렬하여 순서 보장
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		log.Printf("Applying migration: %s", file.Name())
		content, err := fs.ReadFile(filepath.Join(migrationsPath, file.Name()))
		if err != nil {
			return fmt.Errorf("could not read migration file '%s': %w", file.Name(), err)
		}

		// 각 마이그레이션 파일을 별도의 트랜잭션에서 실행
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration '%s': %w", file.Name(), err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration file '%s': %w", file.Name(), err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction for migration '%s': %w", file.Name(), err)
		}
	}
	return nil
}

// 코어 데이터베이스 스키마 (_core_tmidb) - 이제 사용되지 않음
// const coreSchemaSQL = `...`

// 트리거 생성 SQL - 이제 사용되지 않음
// const triggersSQL = `...`

// 코어 데이터베이스 트리거 생성 SQL - 이제 사용되지 않음
// const coreTriggersSQL = `...`

// TimescaleDB 하이퍼테이블 생성 SQL (코어 데이터베이스용) - 이제 사용되지 않음
// const coreTimescaleSQL = `...`

// TimescaleDB 하이퍼테이블 생성 SQL (organization 데이터베이스용)
const timescaleSQL = `
-- ... (이 부분도 다음 단계에서 분리)

CREATE INDEX IF NOT EXISTS idx_user_access_tokens_expires_at ON public.user_access_tokens(expires_at);

-- updated_at 자동 갱신 트리거 적용
DROP TRIGGER IF EXISTS set_timestamp ON public.users;
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON public.users
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

DROP TRIGGER IF EXISTS set_timestamp ON public.auth_tokens;
CREATE TRIGGER set_timestamp
BEFORE UPDATE ON public.auth_tokens
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();
`

// 기본 사용자 생성 함수
func CreateDefaultUsers() error {
	// 시스템 초기화 상태 확인
	var setupCompleted bool
	err := CoreDB.QueryRow("SELECT EXISTS(SELECT 1 FROM system_config WHERE config_key = 'setup_completed')").Scan(&setupCompleted)
	if err != nil {
		return err
	}

	if !setupCompleted {
		// 초기 설정 시작 시간 기록
		_, err = CoreDB.Exec(`
			INSERT INTO system_config (config_key, config_value) 
			VALUES ('setup_started_at', $1)
			ON CONFLICT (config_key) DO NOTHING
		`, time.Now().Format(time.RFC3339))
		if err != nil {
			return err
		}

		log.Println("System initialization required - no admin users will be created automatically")
		log.Println("Please complete setup through web console within 30 minutes")
		return nil
	}

	// 이미 설정이 완료된 경우, 기존 관리자 확인
	var adminExists bool
	err = CoreDB.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE role = 'admin' AND is_active = true)").Scan(&adminExists)
	if err != nil {
		return err
	}

	if !adminExists {
		log.Println("Warning: No active admin users found but setup is marked as completed")
	}

	return nil
}

// CheckSetupTimeout은 설정 제한시간을 확인합니다
func CheckSetupTimeout() error {
	var startTimeStr string
	err := CoreDB.QueryRow("SELECT config_value FROM system_config WHERE config_key = 'setup_started_at'").Scan(&startTimeStr)
	if err != nil {
		// setup_started_at가 없으면 이미 설정 완료된 것으로 간주
		return nil
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		return err
	}

	// 30분 경과 확인
	if time.Since(startTime) > 30*time.Minute {
		return fmt.Errorf("setup timeout exceeded - system is locked")
	}

	return nil
}

// SetSetupCompleted은 초기 설정을 완료합니다
func SetSetupCompleted() error {
	_, err := CoreDB.Exec(`
		INSERT INTO system_config (config_key, config_value) 
		VALUES ('setup_completed', 'true')
		ON CONFLICT (config_key) DO UPDATE SET 
			config_value = EXCLUDED.config_value,
			updated_at = now()
	`)
	return err
}

// IsSetupCompleted는 초기 설정이 완료되었는지 확인합니다
func IsSetupCompleted() (bool, error) {
	db := GetDB()
	if db == nil {
		return false, fmt.Errorf("database connection is not initialized")
	}

	var orgCount, userCount int

	err := db.QueryRow("SELECT COUNT(*) FROM organizations").Scan(&orgCount)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code.Name() == "undefined_table" {
			return false, nil
		}
		return false, fmt.Errorf("failed to query organizations count: %w", err)
	}

	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code.Name() == "undefined_table" {
			return false, nil
		}
		return false, fmt.Errorf("failed to query users count: %w", err)
	}

	return orgCount > 0 && userCount > 0, nil
}

// IsSetupCompletedForOrg는 특정 조직의 초기 설정이 완료되었는지 확인합니다.
func IsSetupCompletedForOrg(orgName string) (bool, error) {
	var exists bool
	key := fmt.Sprintf("setup_completed_org_%s", orgName)
	err := CoreDB.QueryRow("SELECT EXISTS(SELECT 1 FROM system_config WHERE config_key = $1 AND config_value = 'true')", key).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return exists, nil
}

// SetSetupCompletedForOrg는 특정 조직의 초기 설정을 완료로 표시합니다.
func SetSetupCompletedForOrg(orgName string) error {
	key := fmt.Sprintf("setup_completed_org_%s", orgName)
	_, err := CoreDB.Exec(`
		INSERT INTO system_config (config_key, config_value) 
		VALUES ($1, 'true')
		ON CONFLICT (config_key) DO UPDATE SET 
			config_value = EXCLUDED.config_value,
			updated_at = now()
	`, key)
	return err
}

// CreateInitialData 초기 데이터를 생성합니다
func CreateInitialData() error {
	// 기본 조직 생성 로직을 제거합니다.
	// 이제 모든 조직은 /setup 과정을 통해서만 생성됩니다.
	return nil
}

// InitializeCompleteSchema 데이터베이스 스키마를 완전히 초기화합니다
func InitializeCompleteSchema() error {
	if CoreDB == nil {
		return fmt.Errorf("_core_tmidb database not initialized")
	}

	log.Println("Initializing _core_tmidb schema from migration files...")

	// 코어 데이터베이스 스키마 생성
	if err := executeMigrations(CoreDB, coreMigrations, "sql/migrations/core"); err != nil {
		return fmt.Errorf("failed to apply _core_tmidb migrations: %v", err)
	}

	// 조직 및 시스템 타겟 자동 생성 로직 제거 (사용자 지시사항)
	// 모든 조직 생성은 /setup 엔드포인트를 통해 사용자가 직접 수행합니다.
	// 시스템 메트릭 타겟은 첫 조직 생성 시 함께 생성되도록 로직을 이전합니다.

	log.Println("_core_tmidb schema initialization completed successfully")
	return nil
}
