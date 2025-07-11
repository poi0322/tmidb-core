----------------------------------------------------------------
-- 2. 인증 관련 테이블
----------------------------------------------------------------
-- 사용자 계정 테이블
CREATE TABLE IF NOT EXISTS public.users (
    user_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL, -- 사용자의 기본 조직 ID
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL, -- bcrypt 해시된 비밀번호
    role TEXT NOT NULL DEFAULT 'viewer', -- 'admin', 'editor', 'viewer'
    permissions JSONB NOT NULL DEFAULT '{"read": [], "write": []}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, username)
);

-- 사용자-조직 매핑 테이블 (한 사용자가 여러 조직에 속할 수 있도록)
CREATE TABLE IF NOT EXISTS public.user_organizations (
    user_id UUID NOT NULL,
    org_id UUID NOT NULL,
    PRIMARY KEY (user_id, org_id),
    CONSTRAINT fk_user
        FOREIGN KEY(user_id)
        REFERENCES public.users(user_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_organization
        FOREIGN KEY(org_id)
        REFERENCES public.organizations(org_id)
        ON DELETE CASCADE
);

-- API 키와 권한을 관리하는 테이블
CREATE TABLE IF NOT EXISTS public.auth_tokens (
    token_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL,
    encrypted_token TEXT NOT NULL UNIQUE, -- 암호화된 토큰 문자열
    description TEXT,
    permissions JSONB NOT NULL DEFAULT '{"read": [], "write": []}',
    is_admin BOOLEAN NOT NULL DEFAULT false,
    is_active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 사용자별 액세스 토큰 테이블
CREATE TABLE IF NOT EXISTS public.user_access_tokens (
    token_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    org_id UUID NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_user_token
        FOREIGN KEY(user_id)
        REFERENCES public.users(user_id)
        ON DELETE CASCADE
); 