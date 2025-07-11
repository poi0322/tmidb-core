----------------------------------------------------------------
-- 4. 통계 캐싱 및 로그 테이블
----------------------------------------------------------------
-- 시스템 전체 통계 캐시
CREATE TABLE IF NOT EXISTS public.system_stats_cache (
    stat_key TEXT PRIMARY KEY,
    stat_value JSONB NOT NULL,
    last_updated TIMESTAMPTZ NOT NULL DEFAULT now(),
    cache_ttl INTEGER NOT NULL DEFAULT 3600 -- 캐시 TTL (초)
);

-- 조직별 통계 캐시
CREATE TABLE IF NOT EXISTS public.organization_stats_cache (
    org_id UUID NOT NULL,
    stat_key TEXT NOT NULL,
    stat_value JSONB NOT NULL,
    last_updated TIMESTAMPTZ NOT NULL DEFAULT now(),
    cache_ttl INTEGER NOT NULL DEFAULT 3600, -- 캐시 TTL (초)
    PRIMARY KEY (org_id, stat_key)
);

-- 사용자 활동 로그
CREATE TABLE IF NOT EXISTS public.user_activity_log (
    log_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    org_id UUID NOT NULL,
    activity_type TEXT NOT NULL, -- 'login', 'logout', 'data_access', 'config_change' 등
    activity_details JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_user_activity
        FOREIGN KEY(user_id)
        REFERENCES public.users(user_id)
        ON DELETE CASCADE
); 