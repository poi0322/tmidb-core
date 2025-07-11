-- tmidb-core/internal/database/sql/migrations/org/002_promoted_attributes.sql

----------------------------------------------------------------
-- 1. 프로모션된 실수형 속성 (Promoted Doubles)
-- 이 테이블은 double precision 타입의 데이터를 저장하여 빠른 수치 계산 및 범위 검색을 지원합니다.
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.promoted_doubles (
    target_id           UUID NOT NULL,
    category_name       TEXT NOT NULL,
    field_name          TEXT NOT NULL, -- 예: "temperature", "pressure"
    value               DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (target_id, category_name, field_name),
    CONSTRAINT fk_target_category
        FOREIGN KEY(target_id, category_name) 
        REFERENCES public.target_categories(target_id, category_name)
        ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_promoted_doubles_field_value ON public.promoted_doubles(field_name, value);
COMMENT ON TABLE public.promoted_doubles IS '빠른 실수 검색 및 집계를 위해 프로모션된 실수형 필드를 저장합니다.';

----------------------------------------------------------------
-- 2. 프로모션된 정수형 속성 (Promoted Integers)
-- 이 테이블은 bigint 타입의 데이터를 저장하여 정확한 정수 값 비교 및 집계를 지원합니다.
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.promoted_integers (
    target_id           UUID NOT NULL,
    category_name       TEXT NOT NULL,
    field_name          TEXT NOT NULL, -- 예: "heart_rate", "steps"
    value               BIGINT NOT NULL,
    PRIMARY KEY (target_id, category_name, field_name),
    CONSTRAINT fk_target_category
        FOREIGN KEY(target_id, category_name) 
        REFERENCES public.target_categories(target_id, category_name)
        ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_promoted_integers_field_value ON public.promoted_integers(field_name, value);
COMMENT ON TABLE public.promoted_integers IS '빠른 정수 검색 및 집계를 위해 프로모션된 정수형 필드를 저장합니다.';


----------------------------------------------------------------
-- 3. 프로모션된 문자열 속성 (Promoted Keywords)
-- 이 테이블은 문자열 타입의 데이터를 저장하여 특정 키워드 검색, 필터링 및 그룹화를 가속화합니다.
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.promoted_keywords (
    target_id           UUID NOT NULL,
    category_name       TEXT NOT NULL,
    field_name          TEXT NOT NULL, -- 예: "patient_status", "ward_code"
    value               TEXT NOT NULL,
    PRIMARY KEY (target_id, category_name, field_name),
    CONSTRAINT fk_target_category
        FOREIGN KEY(target_id, category_name) 
        REFERENCES public.target_categories(target_id, category_name)
        ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_promoted_keywords_field_value ON public.promoted_keywords(field_name, value);
COMMENT ON TABLE public.promoted_keywords IS '빠른 텍스트 검색 및 필터링을 위해 프로모션된 문자열 필드를 저장합니다.';


----------------------------------------------------------------
-- 4. 프로모션된 불리언 속성 (Promoted Flags)
-- 이 테이블은 참/거짓 상태를 나타내는 데이터를 저장하여 특정 조건의 플래그를 빠르게 필터링합니다.
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.promoted_flags (
    target_id           UUID NOT NULL,
    category_name       TEXT NOT NULL,
    field_name          TEXT NOT NULL, -- 예: "is_discharged", "needs_attention"
    value               BOOLEAN NOT NULL,
    PRIMARY KEY (target_id, category_name, field_name),
    CONSTRAINT fk_target_category
        FOREIGN KEY(target_id, category_name) 
        REFERENCES public.target_categories(target_id, category_name)
        ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_promoted_flags_field_value ON public.promoted_flags(field_name, value);
COMMENT ON TABLE public.promoted_flags IS '빠른 조건 필터링을 위해 프로모션된 불리언(flag) 필드를 저장합니다.';

----------------------------------------------------------------
-- 5. 프로모션된 타임스탬프 속성 (Promoted Timestamps)
-- 이 테이블은 timestamptz 타입의 데이터를 저장하여 정확한 시간 기반 쿼리를 지원합니다.
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.promoted_timestamps (
    target_id           UUID NOT NULL,
    category_name       TEXT NOT NULL,
    field_name          TEXT NOT NULL, -- 예: "event_time", "last_updated"
    value               TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (target_id, category_name, field_name),
    CONSTRAINT fk_target_category
        FOREIGN KEY(target_id, category_name) 
        REFERENCES public.target_categories(target_id, category_name)
        ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_promoted_timestamps_field_value ON public.promoted_timestamps(field_name, value);
COMMENT ON TABLE public.promoted_timestamps IS '빠른 시간 검색 및 범위 필터링을 위해 프로모션된 타임스탬프 필드를 저장합니다.';

----------------------------------------------------------------
-- 6. 프로모션된 날짜 속성 (Promoted Dates)
-- 이 테이블은 date 타입의 데이터를 저장하여 날짜 기반 검색 및 그룹화를 지원합니다.
----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.promoted_dates (
    target_id           UUID NOT NULL,
    category_name       TEXT NOT NULL,
    field_name          TEXT NOT NULL, -- 예: "birth_date", "admission_date"
    value               DATE NOT NULL,
    PRIMARY KEY (target_id, category_name, field_name),
    CONSTRAINT fk_target_category
        FOREIGN KEY(target_id, category_name) 
        REFERENCES public.target_categories(target_id, category_name)
        ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_promoted_dates_field_value ON public.promoted_dates(field_name, value);
COMMENT ON TABLE public.promoted_dates IS '빠른 날짜 검색 및 필터링을 위해 프로모션된 날짜 필드를 저장합니다.';

-- 기존 promoted_metrics 테이블 삭제 (promoted_doubles로 대체)
DROP TABLE IF EXISTS public.promoted_metrics; 