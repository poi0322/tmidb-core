package migration

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Migration은 단일 마이그레이션을 나타냅니다
type Migration struct {
	ID           int        `json:"id" db:"id"`
	Name         string     `json:"name" db:"name"`
	Description  string     `json:"description" db:"description"`
	CategoryName string     `json:"category_name" db:"category_name"`
	FromVersion  float64    `json:"from_version" db:"from_version"`
	ToVersion    float64    `json:"to_version" db:"to_version"`
	SQL          string     `json:"sql,omitempty" db:"sql"`
	Script       string     `json:"script,omitempty" db:"script"`
	Type         string     `json:"type" db:"type"` // "sql" or "script"
	Status       string     `json:"status" db:"status"`
	Error        string     `json:"error,omitempty" db:"error"`
	ExecutedAt   *time.Time `json:"executed_at,omitempty" db:"executed_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// MigrationResult는 마이그레이션 실행 결과를 나타냅니다
type MigrationResult struct {
	Success  bool           `json:"success"`
	Error    string         `json:"error,omitempty"`
	Output   string         `json:"output,omitempty"`
	Changes  int            `json:"changes"`
	Duration time.Duration  `json:"duration"`
	Details  map[string]any `json:"details,omitempty"`
}

// MigrationManager는 마이그레이션을 관리합니다
type MigrationManager struct {
	db *sql.DB
}

// NewMigrationManager는 새로운 마이그레이션 매니저를 생성합니다
func NewMigrationManager(db *sql.DB) *MigrationManager {
	return &MigrationManager{db: db}
}

// InitializeMigrationTable은 마이그레이션 테이블을 초기화합니다
func (m *MigrationManager) InitializeMigrationTable() error {
	// First check if the migrations table already exists
	var tableExists bool
	err := m.db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'migrations'
		)
	`).Scan(&tableExists)
	
	if err != nil {
		return fmt.Errorf("failed to check if migrations table exists: %v", err)
	}
	
	if tableExists {
		// Table exists, check if it has the required columns
		// and add them if they don't exist
		alterTableSQL := `
		DO $$
		BEGIN
			-- Add description column if it doesn't exist
			IF NOT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'description'
			) THEN
				ALTER TABLE migrations ADD COLUMN description TEXT;
			END IF;
			
			-- Add sql column if it doesn't exist
			IF NOT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'sql'
			) THEN
				ALTER TABLE migrations ADD COLUMN sql TEXT;
			END IF;
			
			-- Add script column if it doesn't exist
			IF NOT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'script'
			) THEN
				ALTER TABLE migrations ADD COLUMN script TEXT;
			END IF;
			
			-- Add type column if it doesn't exist
			IF NOT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'type'
			) THEN
				ALTER TABLE migrations ADD COLUMN type VARCHAR(10) DEFAULT 'sql';
				ALTER TABLE migrations ADD CONSTRAINT check_type CHECK (type IN ('sql', 'script'));
			END IF;
			
			-- Add status column if it doesn't exist
			IF NOT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'status'
			) THEN
				ALTER TABLE migrations ADD COLUMN status VARCHAR(20) DEFAULT 'completed';
				ALTER TABLE migrations ADD CONSTRAINT check_status CHECK (status IN ('pending', 'running', 'completed', 'failed', 'rollback'));
			END IF;
			
			-- Add error column if it doesn't exist
			IF NOT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'error'
			) THEN
				ALTER TABLE migrations ADD COLUMN error TEXT;
			END IF;
			
			-- Add executed_at column if it doesn't exist
			IF NOT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'executed_at'
			) THEN
				ALTER TABLE migrations ADD COLUMN executed_at TIMESTAMP;
			END IF;
			
			-- Add created_at column if it doesn't exist
			IF NOT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'created_at'
			) THEN
				ALTER TABLE migrations ADD COLUMN created_at TIMESTAMP DEFAULT NOW();
			END IF;
			
			-- Create indexes if they don't exist
			IF NOT EXISTS (
				SELECT FROM pg_indexes 
				WHERE schemaname = 'public' AND tablename = 'migrations' AND indexname = 'idx_migrations_category'
			) THEN
				CREATE INDEX idx_migrations_category ON migrations(category_name);
			END IF;
			
			IF NOT EXISTS (
				SELECT FROM pg_indexes 
				WHERE schemaname = 'public' AND tablename = 'migrations' AND indexname = 'idx_migrations_status'
			) THEN
				CREATE INDEX idx_migrations_status ON migrations(status);
			END IF;
			
			IF NOT EXISTS (
				SELECT FROM pg_indexes 
				WHERE schemaname = 'public' AND tablename = 'migrations' AND indexname = 'idx_migrations_created_at'
			) THEN
				CREATE INDEX idx_migrations_created_at ON migrations(created_at);
			END IF;
		END
		$$;
		`
		
		_, err = m.db.Exec(alterTableSQL)
		if err != nil {
			return fmt.Errorf("failed to alter migrations table: %v", err)
		}
		
		return nil
	} else {
		// Table doesn't exist, create it
		createTableSQL := `
		CREATE TABLE migrations (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			description TEXT,
			category_name VARCHAR(100) NOT NULL DEFAULT 'general',
			from_version DOUBLE PRECISION NOT NULL,
			to_version DOUBLE PRECISION NOT NULL,
			sql TEXT,
			script TEXT,
			type VARCHAR(10) NOT NULL CHECK (type IN ('sql', 'script')),
			status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'rollback')),
			error TEXT,
			executed_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);
		
		CREATE INDEX idx_migrations_category ON migrations(category_name);
		CREATE INDEX idx_migrations_status ON migrations(status);
		CREATE INDEX idx_migrations_created_at ON migrations(created_at);
		`
		
		_, err = m.db.Exec(createTableSQL)
		if err != nil {
			return fmt.Errorf("failed to create migrations table: %v", err)
		}
		
		return nil
	}
}

// CreateMigration은 새로운 마이그레이션을 생성합니다
func (m *MigrationManager) CreateMigration(migration *Migration) error {
	// 이름 중복 확인
	var exists bool
	err := m.db.QueryRow("SELECT EXISTS(SELECT 1 FROM migrations WHERE name = $1)", migration.Name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("이름 중복 확인 실패: %v", err)
	}
	if exists {
		return fmt.Errorf("마이그레이션 이름이 이미 존재합니다: %s", migration.Name)
	}

	// 기본값 설정
	if migration.CategoryName == "" {
		migration.CategoryName = "general"
	}
	if migration.Status == "" {
		migration.Status = "pending"
	}

	// 타입 결정
	if migration.SQL != "" {
		migration.Type = "sql"
	} else if migration.Script != "" {
		migration.Type = "script"
	} else {
		return fmt.Errorf("SQL 또는 Script 중 하나는 반드시 제공해야 합니다")
	}

	// 삽입
	query := `
	INSERT INTO migrations (name, description, category_name, from_version, to_version, sql, script, type, status)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING id, created_at`

	err = m.db.QueryRow(query,
		migration.Name, migration.Description, migration.CategoryName,
		migration.FromVersion, migration.ToVersion,
		migration.SQL, migration.Script, migration.Type, migration.Status,
	).Scan(&migration.ID, &migration.CreatedAt)

	if err != nil {
		return fmt.Errorf("마이그레이션 생성 실패: %v", err)
	}

	log.Printf("마이그레이션 생성됨: %s (ID: %d)", migration.Name, migration.ID)
	return nil
}

// GetMigrations는 마이그레이션 목록을 조회합니다
func (m *MigrationManager) GetMigrations(category string, status string, limit int) ([]Migration, error) {
	var migrations []Migration
	var args []any
	var conditions []string
	argIdx := 1

	query := "SELECT id, name, description, category_name, from_version, to_version, type, status, error, executed_at, created_at FROM migrations"

	if category != "" {
		conditions = append(conditions, fmt.Sprintf("category_name = $%d", argIdx))
		args = append(args, category)
		argIdx++
	}

	if status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, limit)
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("마이그레이션 조회 실패: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var migration Migration
		err := rows.Scan(
			&migration.ID, &migration.Name, &migration.Description,
			&migration.CategoryName, &migration.FromVersion, &migration.ToVersion, &migration.Type,
			&migration.Status, &migration.Error, &migration.ExecutedAt,
			&migration.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("마이그레이션 스캔 실패: %v", err)
		}
		migrations = append(migrations, migration)
	}

	return migrations, nil
}

// GetMigrationByID는 ID로 마이그레이션을 조회합니다
func (m *MigrationManager) GetMigrationByID(id int) (*Migration, error) {
	var migration Migration

	query := `
	SELECT id, name, description, category_name, from_version, to_version, sql, script, type, status, error, executed_at, created_at 
	FROM migrations WHERE id = $1`

	err := m.db.QueryRow(query, id).Scan(
		&migration.ID, &migration.Name, &migration.Description,
		&migration.CategoryName, &migration.FromVersion, &migration.ToVersion, &migration.SQL,
		&migration.Script, &migration.Type, &migration.Status,
		&migration.Error, &migration.ExecutedAt, &migration.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("마이그레이션을 찾을 수 없습니다: ID %d", id)
		}
		return nil, fmt.Errorf("마이그레이션 조회 실패: %v", err)
	}

	return &migration, nil
}

// ExecuteMigration은 마이그레이션을 실행합니다
func (m *MigrationManager) ExecuteMigration(id int) (*MigrationResult, error) {
	startTime := time.Now()
	result := &MigrationResult{
		Details: make(map[string]any),
	}

	// 마이그레이션 조회
	migration, err := m.GetMigrationByID(id)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	// 상태 확인
	if migration.Status == "completed" {
		result.Error = "이미 완료된 마이그레이션입니다"
		return result, fmt.Errorf(result.Error)
	}

	// 실행 중 상태로 변경
	if err := m.updateMigrationStatus(id, "running", ""); err != nil {
		result.Error = fmt.Sprintf("상태 업데이트 실패: %v", err)
		return result, err
	}

	// 트랜잭션 시작
	tx, err := m.db.Begin()
	if err != nil {
		result.Error = fmt.Sprintf("트랜잭션 시작 실패: %v", err)
		_ = m.updateMigrationStatus(id, "failed", result.Error)
		return result, err
	}

	defer func() {
		if result.Success {
			if err := tx.Commit(); err != nil {
				log.Printf("⚠️ Failed to commit transaction for migration %d: %v", id, err)
				result.Error = fmt.Sprintf("트랜잭션 커밋 실패: %v", err)
				_ = m.updateMigrationStatus(id, "failed", result.Error)
			}
			result.Duration = time.Since(startTime)
			_ = m.updateMigrationStatus(id, "completed", "")
			_ = m.updateExecutedAt(id)
		} else {
			if err := tx.Rollback(); err != nil {
				log.Printf("⚠️ Failed to rollback transaction for migration %d: %v", id, err)
			}
			_ = m.updateMigrationStatus(id, "failed", result.Error)
		}
	}()

	// 타입별 실행
	switch migration.Type {
	case "sql":
		result = m.executeSQLMigration(tx, migration)
	case "script":
		result = m.executeScriptMigration(tx, migration)
	default:
		result.Error = fmt.Sprintf("지원하지 않는 마이그레이션 타입: %s", migration.Type)
		return result, fmt.Errorf(result.Error)
	}

	return result, nil
}

// executeSQLMigration은 SQL 마이그레이션을 실행합니다
func (m *MigrationManager) executeSQLMigration(tx *sql.Tx, migration *Migration) *MigrationResult {
	result := &MigrationResult{Details: make(map[string]any)}

	// SQL 문을 세미콜론으로 분리하여 실행
	statements := strings.Split(migration.SQL, ";")
	var outputs []string
	totalChanges := 0

	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		startTime := time.Now()
		res, err := tx.Exec(stmt)
		duration := time.Since(startTime)

		if err != nil {
			result.Error = fmt.Sprintf("SQL 실행 실패 (문장 %d): %v", i+1, err)
			return result
		}

		rowsAffected, _ := res.RowsAffected()
		totalChanges += int(rowsAffected)

		outputs = append(outputs, fmt.Sprintf("[%d] %dms, %d행 영향", i+1, duration.Milliseconds(), rowsAffected))
	}

	result.Success = true
	result.Changes = totalChanges
	result.Output = strings.Join(outputs, "\n")
	result.Details["statements_executed"] = len(statements) - 1 // 빈 문장 제외
	result.Details["migration_type"] = "SQL"

	return result
}

// executeScriptMigration은 JavaScript 스크립트 마이그레이션을 실행합니다
func (m *MigrationManager) executeScriptMigration(tx *sql.Tx, migration *Migration) *MigrationResult {
	result := &MigrationResult{Details: make(map[string]any)}

	// TODO: JavaScript 마이그레이션 기능은 현재 비활성화됨
	// goja 패키지 의존성 추가 후 활성화 예정
	result.Error = "JavaScript 마이그레이션 기능은 현재 지원되지 않습니다"
	return result
}

// updateMigrationStatus는 마이그레이션 상태를 업데이트합니다
func (m *MigrationManager) updateMigrationStatus(id int, status, errorMsg string) error {
	query := "UPDATE migrations SET status = $1, error = $2 WHERE id = $3"
	_, err := m.db.Exec(query, status, errorMsg, id)
	return err
}

// updateExecutedAt는 실행 시간을 업데이트합니다
func (m *MigrationManager) updateExecutedAt(id int) error {
	query := "UPDATE migrations SET executed_at = NOW() WHERE id = $1"
	_, err := m.db.Exec(query, id)
	return err
}

// DeleteMigration은 마이그레이션을 삭제합니다 (pending 상태만)
func (m *MigrationManager) DeleteMigration(id int) error {
	// 상태 확인
	migration, err := m.GetMigrationByID(id)
	if err != nil {
		return err
	}

	if migration.Status != "pending" {
		return fmt.Errorf("pending 상태의 마이그레이션만 삭제할 수 있습니다 (현재: %s)", migration.Status)
	}

	query := "DELETE FROM migrations WHERE id = $1"
	result, err := m.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("마이그레이션 삭제 실패: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("마이그레이션을 찾을 수 없습니다: ID %d", id)
	}

	log.Printf("마이그레이션 삭제됨: %s (ID: %d)", migration.Name, id)
	return nil
}

// GetMigrationStats는 마이그레이션 통계를 반환합니다
func (m *MigrationManager) GetMigrationStats() (map[string]any, error) {
	stats := make(map[string]any)

	// 상태별 카운트
	query := `
	SELECT 
		COUNT(*) as total,
		COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending,
		COUNT(CASE WHEN status = 'running' THEN 1 END) as running,
		COUNT(CASE WHEN status = 'completed' THEN 1 END) as completed,
		COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed
	FROM migrations`

	var total, pending, running, completed, failed int
	err := m.db.QueryRow(query).Scan(&total, &pending, &running, &completed, &failed)
	if err != nil {
		return nil, fmt.Errorf("통계 조회 실패: %v", err)
	}

	stats["total"] = total
	stats["pending"] = pending
	stats["running"] = running
	stats["completed"] = completed
	stats["failed"] = failed

	// 카테고리별 카운트
	categoryQuery := "SELECT category_name, COUNT(*) FROM migrations GROUP BY category_name ORDER BY category_name"
	rows, err := m.db.Query(categoryQuery)
	if err != nil {
		return nil, fmt.Errorf("카테고리 통계 조회 실패: %v", err)
	}
	defer rows.Close()

	categories := make(map[string]int)
	for rows.Next() {
		var category string
		var count int
		if err := rows.Scan(&category, &count); err != nil {
			continue
		}
		categories[category] = count
	}
	stats["categories"] = categories

	return stats, nil
}

// ApplyMigrations는 디렉토리에서 마이그레이션을 적용합니다
func (m *MigrationManager) ApplyMigrations(dir string) (err error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("could not read migration directory %s: %w", dir, err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// .sql 파일만 마이그레이션 대상으로 포함합니다.
		if !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		log.Printf("Applying migration: %s", file.Name())

		query, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", filePath, err)
		}

		tx, err := m.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}

		statements := strings.Split(string(query), ";")

		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}

			if _, err = tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("failed to execute migration file %s (statement: %q): %w", file.Name(), stmt, err)
			}
		}

		if err = tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction for migration file %s: %w", file.Name(), err)
		}
	}

	return nil
}
