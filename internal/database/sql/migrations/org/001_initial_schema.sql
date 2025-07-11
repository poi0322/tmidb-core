-- tmiDB 메인 데이터베이스 스키마 (데이터 저장용)

----------------------------------------------------------------
-- 0. 조직 (Organization/Database) - 이 DB에 해당하는 조직 정보만 저장
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.organizations (
    org_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

----------------------------------------------------------------
-- 1. 카테고리 스키마 정의
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.category_schemas (
    schema_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL, --REFERENCES organizations(org_id) ON DELETE CASCADE,
    category_name TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    schema_definition JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, category_name, version)
);

----------------------------------------------------------------
-- 2. 대상 (Target)
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.targets (
    -- target_id는 데이터 수집 시 UUIDv7로 생성됨
    target_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL, --REFERENCES organizations(org_id) ON DELETE CASCADE,
    target_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, target_name)
);

----------------------------------------------------------------
-- 3. 대상-카테고리 매핑
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.target_categories (
    target_id UUID NOT NULL REFERENCES targets(target_id) ON DELETE CASCADE,
    org_id UUID NOT NULL,
    category_name TEXT NOT NULL,
    category_data JSONB NOT NULL,
    schema_version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (target_id, category_name)
);

CREATE INDEX idx_target_categories_category_name ON public.target_categories(category_name);

----------------------------------------------------------------
-- 4. 시계열 데이터 (TimescaleDB Hypertable) - REMOVED
-- All timeseries data is now stored in the central ts_obs table in the _core_tmidb database.
----------------------------------------------------------------

----------------------------------------------------------------
-- 5. 지리적 데이터 (PostGIS, 선택 사항)
----------------------------------------------------------------
-- CREATE EXTENSION IF NOT EXISTS postgis;
-- CREATE TABLE public.geo_trace (
--     target_id UUID NOT NULL,
--     ts TIMESTAMPTZ NOT NULL,
--     location GEOMETRY(Point, 4326) NOT NULL,
--     PRIMARY KEY (target_id, ts)
-- );
-- SELECT create_hypertable('geo_trace', 'ts', if_not_exists => TRUE);

----------------------------------------------------------------
-- 6. 파일 저장소 (SeaweedFS 등 외부 저장소와 연동)
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.files (
    file_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL, --REFERENCES organizations(org_id) ON DELETE CASCADE,
    target_id UUID NOT NULL, --REFERENCES targets(target_id) ON DELETE CASCADE,
    category_name TEXT,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_size BIGINT,
    mime_type TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

----------------------------------------------------------------
-- 7. 리스너 (Listener) - 데이터 수집 지점
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.listeners (
    listener_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL, --REFERENCES organizations(org_id) ON DELETE CASCADE,
    category_name TEXT NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, category_name)
);

----------------------------------------------------------------
-- 8. 데이터 처리 버킷 (Raw Data Bucket)
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.raw_bucket (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL, --REFERENCES organizations(org_id) ON DELETE CASCADE,
    source_id TEXT, -- e.g. 'listener-1', 'user-upload-abc'
    ts TIMESTAMPTZ NOT NULL DEFAULT now(),
    payload JSONB NOT NULL
);

----------------------------------------------------------------
-- 9. 트리거
----------------------------------------------------------------
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER set_timestamp_targets
BEFORE UPDATE ON public.targets
FOR EACH ROW
EXECUTE FUNCTION trigger_set_timestamp();

-- UnclassifiedData는 스키마 검증에 실패한 데이터를 저장합니다.
CREATE TABLE IF NOT EXISTS public.unclassified_data (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL, --REFERENCES organizations(org_id) ON DELETE CASCADE,
    raw_bucket_id UUID NOT NULL, --REFERENCES raw_bucket(id) ON DELETE CASCADE,
    target_id UUID NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    payload JSONB NOT NULL,
    error_message TEXT,
    CONSTRAINT fk_target
        FOREIGN KEY(target_id) 
        REFERENCES targets(target_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_organization
        FOREIGN KEY(org_id)
        REFERENCES organizations(org_id)
        ON DELETE CASCADE
);

-- 인덱스 생성
CREATE INDEX IF NOT EXISTS idx_unclassified_target_id ON public.unclassified_data(target_id);
CREATE INDEX IF NOT EXISTS idx_unclassified_received_at ON public.unclassified_data(received_at);
CREATE INDEX IF NOT EXISTS idx_target_categories_org_id ON public.target_categories(org_id);
CREATE INDEX IF NOT EXISTS idx_category_schemas_org_id ON public.category_schemas(org_id);
CREATE INDEX IF NOT EXISTS idx_listeners_org_name ON public.listeners(org_id);

-- TimescaleDB 하이퍼테이블로 변환
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        -- ts_obs table creation is moved to core schema
        NULL;
    END IF;
END $$; 