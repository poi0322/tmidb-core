-- 필요한 PostgreSQL 확장 활성화
CREATE EXTENSION IF NOT EXISTS "timescaledb" WITH SCHEMA public; 
GRANT ALL ON SCHEMA public TO admin; 