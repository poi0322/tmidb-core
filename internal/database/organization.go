package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// CreateInitialOrganizationAndAdmin 은 초기 조직과 관리자 계정을 생성합니다.
// 또한 관리자용 API 토큰을 생성하여 반환합니다.
func CreateInitialOrganizationAndAdmin(db *sql.DB, orgName, adminUsername, adminPassword string) (*User, *AuthToken, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // 롤백은 커밋되지 않은 경우에만 실행됩니다.

	// 1. 조직 생성
	var orgID string
	err = tx.QueryRow("INSERT INTO organizations (name) VALUES ($1) RETURNING org_id", orgName).Scan(&orgID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// 2. 관리자 사용자 생성
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to hash password: %w", err)
	}

	var adminUserID string
	err = tx.QueryRow(
		"INSERT INTO users (org_id, username, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING user_id",
		orgID, adminUsername, string(hashedPassword), "admin",
	).Scan(&adminUserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create admin user: %w", err)
	}

	// 3. 사용자를 조직에 연결
	_, err = tx.Exec("INSERT INTO user_organizations (user_id, org_id) VALUES ($1, $2)", adminUserID, orgID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to link user to organization: %w", err)
	}

	// 4. 관리자용 영구 API 토큰 생성 (조직에 귀속)
	tokenString, err := GenerateRandomString(32)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate token string: %w", err)
	}

	encryptedToken, err := EncryptToken(tokenString)
	if err != nil {
		return nil, nil, fmt.Errorf("could not encrypt token: %w", err)
	}

	var authTokenID string
	description := fmt.Sprintf("Initial admin token for organization %s", orgName)
	err = tx.QueryRow(
		`INSERT INTO auth_tokens (org_id, encrypted_token, description, permissions, is_admin, is_active) 
         VALUES ($1, $2, $3, $4, $5, TRUE) RETURNING token_id`,
		orgID, encryptedToken, description, sql.NullString{String: `{"admin": true}`, Valid: true}, true,
	).Scan(&authTokenID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create auth token: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 생성된 사용자 및 토큰 정보 반환
	adminUser, err := GetUserByID(db, adminUserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve created admin user: %w", err)
	}

	authToken, err := GetAuthTokenByID(db, authTokenID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve created auth token: %w", err)
	}

	return adminUser, authToken, nil
}

// OrganizationInfo는 조직 목록 조회 시 반환될 상세 정보를 담는 구조체입니다.
type OrganizationInfo struct {
	OrgID        string    `json:"org_id"`
	Name         string    `json:"name"`
	DatabaseName string    `json:"database_name"`
	CreatedAt    time.Time `json:"created_at"`
	UserCount    int       `json:"user_count"`
	DataCount    int       `json:"data_count"`
}

// GetOrganizations는 모든 조직의 목록과 각 조직의 요약 정보를 반환합니다.
func GetOrganizations() ([]OrganizationInfo, error) {
	db := GetDB()
	rows, err := db.Query(`
		SELECT o.org_id, o.name, od.database_name, o.created_at 
		FROM organizations o
		LEFT JOIN organization_databases od ON o.org_id = od.org_id
		ORDER BY o.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query organizations: %w", err)
	}
	defer rows.Close()

	var organizations []OrganizationInfo
	for rows.Next() {
		var org OrganizationInfo
		var name, dbName sql.NullString
		if err := rows.Scan(&org.OrgID, &name, &dbName, &org.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan organization row: %w", err)
		}
		org.Name = name.String
		org.DatabaseName = dbName.String

		// 사용자 수 조회
		err := db.QueryRow("SELECT COUNT(*) FROM users WHERE org_id = $1", org.OrgID).Scan(&org.UserCount)
		if err != nil {
			fmt.Printf("Warning: could not get user count for org %s: %v\n", org.OrgID, err)
			org.UserCount = 0
		}

		// 데이터 수 조회
		if org.DatabaseName != "" {
			orgDB, err := GetOrgDB(org.OrgID)
			if err != nil {
				fmt.Printf("Warning: could not connect to org DB for stats %s: %v\n", org.OrgID, err)
				org.DataCount = 0
			} else {
				defer orgDB.Close()
				err = orgDB.DB.QueryRow("SELECT COUNT(*) FROM target_categories").Scan(&org.DataCount)
				if err != nil {
					fmt.Printf("Warning: could not get data count for org %s: %v\n", org.OrgID, err)
					org.DataCount = 0
				}
			}
		}

		organizations = append(organizations, org)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return organizations, nil
}

// GetOrganizationByID는 ID로 특정 조직의 정보를 조회합니다.
func GetOrganizationByID(orgID string) (*OrganizationInfo, error) {
	db := GetDB()
	var org OrganizationInfo
	var name, dbName sql.NullString
	var err error

	// Handle "default" orgID as a special case
	if orgID == "default" {
		// Query for the first organization created, or a specifically marked default one
		// For now, let's assume the first organization created is the default
		err = db.QueryRow(`
			SELECT o.org_id, o.name, od.database_name, o.created_at 
			FROM organizations o 
			LEFT JOIN organization_databases od ON o.org_id = od.org_id 
			ORDER BY o.created_at ASC LIMIT 1`).Scan(&org.OrgID, &name, &dbName, &org.CreatedAt)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("no default organization found")
			}
			return nil, fmt.Errorf("failed to query default organization: %w", err)
		}
		org.Name = name.String
		org.DatabaseName = dbName.String
	} else {
		// Original query for a specific orgID
		err = db.QueryRow(`
			SELECT o.org_id, o.name, od.database_name, o.created_at 
			FROM organizations o 
			LEFT JOIN organization_databases od ON o.org_id = od.org_id 
			WHERE o.org_id = $1`, orgID).Scan(&org.OrgID, &name, &dbName, &org.CreatedAt)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("organization with ID %s not found in core tables", orgID)
			}
			return nil, fmt.Errorf("failed to query organization: %w", err)
		}
		org.Name = name.String
		org.DatabaseName = dbName.String
	}

	// 사용자 수 조회
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE org_id = $1", org.OrgID).Scan(&org.UserCount)
	if err != nil {
		fmt.Printf("Warning: could not get user count for org %s: %v\n", org.OrgID, err)
		org.UserCount = 0
	}

	// 데이터 수 조회
	if org.DatabaseName != "" {
		orgDB, err := GetOrgDB(org.OrgID)
		if err != nil {
			fmt.Printf("Warning: could not connect to org DB for stats %s: %v\n", org.OrgID, err)
			org.DataCount = 0
		} else {
			defer orgDB.Close()
			err = orgDB.DB.QueryRow("SELECT COUNT(*) FROM target_categories").Scan(&org.DataCount)
			if err != nil {
				fmt.Printf("Warning: could not get data count for org %s: %v\n", org.OrgID, err)
				org.DataCount = 0
			}
		}
	}

	return &org, nil
}

// UpdateOrganization은 조직의 이름을 변경합니다.
func UpdateOrganization(orgID, newName string) (*OrganizationInfo, error) {
	db := GetDB()
	_, err := db.Exec("UPDATE organizations SET name = $1 WHERE org_id = $2", newName, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to update organization name: %w", err)
	}

	// 변경된 정보 다시 조회
	return GetOrganizationByID(orgID)
}

// DeleteOrganization은 조직과 관련된 모든 데이터를 삭제합니다.
// 주의: 이 작업은 되돌릴 수 없습니다.
func DeleteOrganization(orgID string) error {
	db := GetDB()

	// 0. 조직이 하나뿐인 경우 삭제 방지
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM organizations").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count organizations: %w", err)
	}
	if count <= 1 {
		return fmt.Errorf("cannot delete the last organization")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. 조직 DB 이름 조회
	var dbName string
	err = tx.QueryRow("SELECT database_name FROM organization_databases WHERE org_id = $1", orgID).Scan(&dbName)
	if err != nil {
		return fmt.Errorf("failed to find database for organization %s: %w", orgID, err)
	}

	// 2. core DB에서 조직 관련 데이터 삭제
	// (user_organizations, auth_tokens, users 등 ON DELETE CASCADE로 자동 처리될 수 있음)
	if _, err := tx.Exec("DELETE FROM organizations WHERE org_id = $1", orgID); err != nil {
		return fmt.Errorf("failed to delete from organizations table: %w", err)
	}

	// 3. 트랜잭션 커밋 (core DB 변경사항 적용)
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit core db transaction: %w", err)
	}

	// 4. 조직 전용 데이터베이스 삭제
	// 주의: 이 작업은 매우 위험하며, 별도의 DB 유저 권한이 필요할 수 있습니다.
	if _, err := db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", pq.QuoteIdentifier(dbName))); err != nil {
		return fmt.Errorf("failed to drop organization database %s: %w", dbName, err)
	}

	return nil
}

// OrganizationExistsByName 은 이름으로 조직의 존재 여부를 확인합니다.
func OrganizationExistsByName(name string) (bool, error) {
	var exists bool
	err := CoreDB.QueryRow("SELECT EXISTS(SELECT 1 FROM organizations WHERE name = $1)", name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if organization exists by name: %w", err)
	}
	return exists, nil
}

// CreateOrganizationAndDatabase는 조직과 해당 데이터베이스를 생성하고 스키마를 마이그레이션합니다.
func CreateOrganizationAndDatabase(orgID, orgName string) (*Organization, error) {
	tx, err := CoreDB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. 조직 생성
	newOrg := &Organization{}
	err = tx.QueryRow(
		"INSERT INTO organizations (org_id, name) VALUES ($1, $2) RETURNING org_id, name, created_at",
		orgID, orgName,
	).Scan(&newOrg.OrgID, &newOrg.Name, &newOrg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization in core DB: %w", err)
	}

	// 2. 조직 데이터베이스 생성
	dbName := fmt.Sprintf("org_%s", newOrg.OrgID)
	dbName = SanitizeDBName(dbName) // DB 이름 유효성 검사 및 정리

	// 데이터베이스 생성 쿼리는 트랜잭션 내에서 실행할 수 없습니다.
	// 따라서 트랜잭션을 잠시 커밋하고, DB 생성 후 다시 진행합니다.
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction before creating database: %w", err)
	}

	// DB 생성 (admin 연결 사용)
	adminDB, err := ConnectToAdminDatabase()
	if err != nil {
		return nil, fmt.Errorf("failed to connect as admin user to create database: %w", err)
	}
	defer adminDB.Close()

	if _, err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", pq.QuoteIdentifier(dbName))); err != nil {
		return nil, fmt.Errorf("failed to execute CREATE DATABASE statement: %w", err)
	}

	// 3. organization_databases 테이블에 정보 저장
	_, err = CoreDB.Exec(
		"INSERT INTO organization_databases (org_id, database_name) VALUES ($1, $2)",
		newOrg.OrgID, dbName,
	)
	if err != nil {
		// 만약 여기서 실패하면 생성된 DB를 롤백해야 하지만, 복잡성을 위해 일단 로깅만 합니다.
		return nil, fmt.Errorf("failed to record organization database mapping: %w", err)
	}

	// 4. 새 조직 데이터베이스에 스키마 마이그레이션 적용
	orgDB, err := ConnectToOrganizationDatabase(newOrg.OrgID, nil) // config는 nil로 전달하여 내부에서 로드하도록 함
	if err != nil {
		return nil, fmt.Errorf("failed to connect to new organization database for migration: %w", err)
	}
	defer orgDB.Close()

	if err := executeMigrations(orgDB, orgMigrations, "sql/migrations/org"); err != nil {
		return nil, fmt.Errorf("failed to apply migrations to new organization database: %w", err)
	}

	// 첫 조직인 경우, 시스템 메트릭 타겟을 생성합니다.
	var orgCount int
	err = CoreDB.QueryRow("SELECT COUNT(*) FROM organizations").Scan(&orgCount)
	if err != nil {
		// 이 단계에서 실패하더라도 조직 생성 자체를 롤백할 필요는 없으므로 로깅만 합니다.
		log.Printf("Warning: could not get organization count to determine if system metrics target should be created: %v", err)
	}

	if orgCount == 1 {
		if err := createSystemMetricsTarget(orgDB, newOrg.OrgID); err != nil {
			log.Printf("Warning: failed to create system metrics target for the first organization '%s': %v", newOrg.OrgID, err)
		}
	}

	return newOrg, nil
}

// createSystemMetricsTarget는 주어진 조직 DB에 시스템 메트릭 수집용 타겟과 스키마를 생성합니다.
func createSystemMetricsTarget(orgDB *sql.DB, orgID string) error {
	systemMetricsUUID := "00000000-0000-4000-8000-000000000001"

	var targetExists bool
	err := orgDB.QueryRow("SELECT EXISTS(SELECT 1 FROM targets WHERE target_id = $1)", systemMetricsUUID).Scan(&targetExists)
	if err != nil {
		return fmt.Errorf("failed to check for system metrics target: %w", err)
	}

	if !targetExists {
		tx, err := orgDB.Begin()
		if err != nil {
			return fmt.Errorf("failed to start transaction for system metrics target: %w", err)
		}
		defer tx.Rollback()

		_, err = tx.Exec(`
			INSERT INTO targets (target_id, org_id, target_name) 
			VALUES ($1, $2, '_system_metrics')
		`, systemMetricsUUID, orgID)
		if err != nil {
			return fmt.Errorf("failed to create system metrics target: %w", err)
		}

		_, err = tx.Exec(`
			INSERT INTO category_schemas (org_id, category_name, version, schema_definition, is_active)
			VALUES ($1, 'metrics', 1, '{"type": "object", "properties": {"cpu_usage": {"type": "number"}, "memory_usage": {"type": "number"}, "disk_usage": {"type": "number"}, "network_io": {"type": "number"}}}', true)
			ON CONFLICT (org_id, category_name, version) DO NOTHING
		`, orgID)
		if err != nil {
			return fmt.Errorf("failed to create metrics schema: %w", err)
		}

		_, err = tx.Exec(`
			INSERT INTO target_categories (target_id, org_id, category_name, schema_version, category_data) 
			VALUES ($1, $2, 'metrics', 1, '{"description": "System metrics collection target"}')
			ON CONFLICT (target_id, category_name) DO NOTHING
		`, systemMetricsUUID, orgID)
		if err != nil {
			return fmt.Errorf("failed to create system metrics target category: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction for system metrics target: %w", err)
		}

		log.Printf("✅ System metrics target created successfully in organization %s", orgID)
	}

	return nil
}


// GetOrganizationsForUser는 사용자가 속한 모든 조직의 목록을 반환합니다.
func GetOrganizationsForUser(db *sql.DB, userID string) ([]OrganizationInfo, error) {
	rows, err := db.Query(`
		SELECT o.org_id, o.name, od.database_name, o.created_at 
		FROM organizations o
		LEFT JOIN organization_databases od ON o.org_id = od.org_id
		JOIN user_organizations uo ON o.org_id = uo.org_id
		WHERE uo.user_id = $1
		ORDER BY o.name ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query organizations for user: %w", err)
	}
	defer rows.Close()

	var organizations []OrganizationInfo
	for rows.Next() {
		var org OrganizationInfo
		var name, dbName sql.NullString
		if err := rows.Scan(&org.OrgID, &name, &dbName, &org.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan organization row: %w", err)
		}
		org.Name = name.String
		org.DatabaseName = dbName.String
		organizations = append(organizations, org)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return organizations, nil
}

// IsUserMemberOfOrg는 사용자가 특정 조직의 멤버인지 확인합니다.
func IsUserMemberOfOrg(db *sql.DB, userID string, orgID string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM user_organizations WHERE user_id = $1 AND org_id = $2)"
	err := db.QueryRow(query, userID, orgID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check user organization membership: %w", err)
	}
	return exists, nil
}
