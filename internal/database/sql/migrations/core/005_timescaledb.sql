-- TimescaleDB 확장 활성화 (코어 DB용)
CREATE EXTENSION IF NOT EXISTS "timescaledb" WITH SCHEMA public;

-- 시스템 메트릭 등 코어 레벨의 시계열 데이터 저장을 위한 테이블
CREATE TABLE IF NOT EXISTS public.ts_obs (
    target_id UUID NOT NULL,
    category_name TEXT NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL
);

-- 인덱스 추가 (조회 성능 향상)
-- UNIQUE 인덱스나 PK를 (target_id, category_name, ts)로 고려해볼 수 있으나,
-- 동일 시간에 중복 데이터가 들어올 가능성을 열어두기 위해 일반 인덱스로 생성합니다.
CREATE INDEX IF NOT EXISTS idx_ts_obs_target_id_ts ON public.ts_obs (target_id, ts DESC);
CREATE INDEX IF NOT EXISTS idx_ts_obs_category_name_ts ON public.ts_obs (category_name, ts DESC);

-- UPSERT를 위한 UNIQUE 제약조건 추가
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ts_obs_unique_triple'
    ) THEN
        ALTER TABLE public.ts_obs ADD CONSTRAINT ts_obs_unique_triple UNIQUE (target_id, category_name, ts);
    END IF;
END $$;

-- 하이퍼테이블로 변환
-- TimescaleDB가 설치된 환경에서만 성공합니다.
SELECT create_hypertable('ts_obs', 'ts', if_not_exists => TRUE); 