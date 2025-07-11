-- tmiDB 코어 데이터베이스 스키마 (계정, 설정, 통계 캐싱용)

----------------------------------------------------------------
-- 0. 조직 (Organization)
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.organizations (
    org_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

----------------------------------------------------------------
-- 1. 트리거 함수
----------------------------------------------------------------
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

----------------------------------------------------------------
-- 2. 사용자 및 권한
----------------------------------------------------------------

-- 사용자 테이블
CREATE TABLE IF NOT EXISTS public.users (
    user_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer', -- 'admin', 'editor', 'viewer'
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 사용자-조직 매핑 (다대다 관계)
CREATE TABLE IF NOT EXISTS public.user_organizations (
    user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    org_id UUID NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, org_id)
);

-- API 인증 토큰 테이블
CREATE TABLE IF NOT EXISTS public.auth_tokens (
    token_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(user_id) ON DELETE SET NULL, -- 사용자 삭제 시 토큰은 유지될 수 있음
    token_hash TEXT NOT NULL UNIQUE,
    description TEXT,
    permissions JSONB,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 사용자 액세스 토큰 (세션 대용)
CREATE TABLE IF NOT EXISTS public.user_access_tokens (
    token_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    org_id UUID NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 마이그레이션 관리 테이블 (최상단에 추가)
CREATE TABLE IF NOT EXISTS public.migrations (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 인덱스 추가
CREATE INDEX IF NOT EXISTS idx_users_org_id ON public.users(org_id);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_org_id ON public.auth_tokens(org_id);
CREATE INDEX IF NOT EXISTS idx_user_access_tokens_user_id ON public.user_access_tokens(user_id);
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