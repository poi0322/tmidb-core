----------------------------------------------------------------
-- 6. 트리거, 인덱스 설정
----------------------------------------------------------------

-- 코어 데이터베이스 트리거 적용
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_timestamp_users') THEN
        CREATE TRIGGER set_timestamp_users
        BEFORE UPDATE ON public.users
        FOR EACH ROW
        EXECUTE PROCEDURE trigger_set_timestamp();
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_timestamp_system_config') THEN
        CREATE TRIGGER set_timestamp_system_config
        BEFORE UPDATE ON public.system_config
        FOR EACH ROW
        EXECUTE PROCEDURE trigger_set_timestamp();
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'set_timestamp_organization_config') THEN
        CREATE TRIGGER set_timestamp_organization_config
        BEFORE UPDATE ON public.organization_config
        FOR EACH ROW
        EXECUTE PROCEDURE trigger_set_timestamp();
    END IF;
END $$;

-- 인덱스 생성
CREATE INDEX IF NOT EXISTS idx_user_activity_user_id ON user_activity_log(user_id);
CREATE INDEX IF NOT EXISTS idx_user_activity_org_id ON user_activity_log(org_id);
CREATE INDEX IF NOT EXISTS idx_user_activity_created_at ON user_activity_log(created_at);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_org_id ON auth_tokens(org_id);
CREATE INDEX IF NOT EXISTS idx_users_org_id ON users(org_id);
CREATE INDEX IF NOT EXISTS idx_user_access_tokens_org_id ON user_access_tokens(org_id); 