-- 기존 테이블 삭제 (초기화 환경이므로 안전)
DROP TABLE IF EXISTS public.listeners;

CREATE TABLE public.listeners (
    listener_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL,
    category_name TEXT NOT NULL,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, category_name)
);

CREATE INDEX IF NOT EXISTS idx_listeners_org_id ON public.listeners(org_id);
CREATE INDEX IF NOT EXISTS idx_listeners_category_name ON public.listeners(category_name); 