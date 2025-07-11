----------------------------------------------------------------
-- 3. 시스템 설정 테이블
----------------------------------------------------------------
-- 시스템 초기 설정 상태 관리
CREATE TABLE IF NOT EXISTS public.system_config (
    config_key TEXT PRIMARY KEY,
    config_value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 조직별 설정 테이블
CREATE TABLE IF NOT EXISTS public.organization_config (
    org_id UUID NOT NULL,
    config_key TEXT NOT NULL,
    config_value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, config_key)
);

-- 조직 데이터베이스 정보 테이블
CREATE TABLE IF NOT EXISTS public.organization_databases (
    org_id UUID PRIMARY KEY REFERENCES organizations(org_id) ON DELETE CASCADE,
    database_name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
); 