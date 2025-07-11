-- Add token_type column to distinguish API tokens and session tokens
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns 
    WHERE table_name='auth_tokens' AND column_name='token_type'
  ) THEN
    ALTER TABLE auth_tokens ADD COLUMN token_type TEXT DEFAULT 'api';
  END IF;
  UPDATE auth_tokens SET token_type = 'api' WHERE token_type IS NULL;
  ALTER TABLE auth_tokens ALTER COLUMN token_type SET NOT NULL;
  CREATE INDEX IF NOT EXISTS idx_auth_tokens_token_type ON auth_tokens(token_type); 
END $$;