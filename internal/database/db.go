package database

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/tmidb/tmidb-core/internal/config"
)

// DBTX is a common interface for *sql.DB and *sql.Tx.
// This allows for functions to be used both within and outside of transactions.
type DBTX interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// OrgDBConnection은 조직별 데이터베이스 연결을 관리합니다.
type OrgDBConnection struct {
	DB    *sql.DB
	OrgID string
}

// GetOrgDB는 조직 ID에 해당하는 데이터베이스 연결을 반환합니다.
func GetOrgDB(orgID string) (*OrgDBConnection, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	db, err := ConnectToOrganizationDatabase(orgID, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to organization database: %v", err)
	}

	return &OrgDBConnection{
		DB:    db,
		OrgID: orgID,
	}, nil
}

// Close는 조직 데이터베이스 연결을 닫습니다.
func (conn *OrgDBConnection) Close() error {
	if conn.DB != nil {
		return conn.DB.Close()
	}
	return nil
}

// ValidateSchemaData는 전달된 스키마 정의(JSON)의 유효성을 검사합니다.
func (db *OrgDBConnection) ValidateSchemaData(schemaBytes []byte) error {
	if !json.Valid(schemaBytes) {
		return fmt.Errorf("schema is not valid JSON")
	}

	var schemaDef struct {
		Fields       json.RawMessage `json:"fields"`
		IsTimeseries bool            `json:"is_timeseries"`
	}
	if err := json.Unmarshal(schemaBytes, &schemaDef); err != nil {
		return fmt.Errorf("failed to unmarshal schema structure: %w", err)
	}

	if len(schemaDef.Fields) == 0 || string(schemaDef.Fields) == "null" || string(schemaDef.Fields) == "{}" {
		return nil // 필드가 없는 스키마는 유효
	}

	var fields map[string]map[string]interface{}
	if err := json.Unmarshal(schemaDef.Fields, &fields); err != nil {
		return fmt.Errorf("schema 'fields' is not a valid object of objects: %w", err)
	}

	allowedTypes := map[string]bool{
		"string": true, "number": true, "boolean": true,
		"date": true, "object": true, "array": true,
	}

	for name, def := range fields {
		if name == "" {
			return fmt.Errorf("field name cannot be empty")
		}

		typeVal, ok := def["type"].(string)
		if !ok {
			return fmt.Errorf("field '%s' is missing 'type' or it's not a string", name)
		}

		if !allowedTypes[typeVal] {
			return fmt.Errorf("field '%s' has an unsupported type: '%s'", name, typeVal)
		}
	}

	return nil
}
