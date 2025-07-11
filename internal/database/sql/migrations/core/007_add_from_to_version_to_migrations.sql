-- Add from_version and to_version columns with default values (if they don't exist)
DO $$
BEGIN
    -- Check if from_version column exists
    IF NOT EXISTS (
        SELECT FROM information_schema.columns 
        WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'from_version'
    ) THEN
        ALTER TABLE migrations ADD COLUMN from_version DOUBLE PRECISION DEFAULT 0.0;
    END IF;
    
    -- Check if to_version column exists
    IF NOT EXISTS (
        SELECT FROM information_schema.columns 
        WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'to_version'
    ) THEN
        ALTER TABLE migrations ADD COLUMN to_version DOUBLE PRECISION DEFAULT 0.0;
    END IF;
END
$$;

-- Update existing rows to have default values
-- For now, we'll assume existing migrations implicitly moved from 0.0 to 1.0 or similar.
UPDATE migrations SET from_version = 0.0, to_version = 1.0 WHERE from_version IS NULL AND to_version IS NULL;

-- Add category_name column for concept-based migration management (if it doesn't exist)
DO $$
BEGIN
    -- Check if category_name column exists
    IF NOT EXISTS (
        SELECT FROM information_schema.columns 
        WHERE table_schema = 'public' AND table_name = 'migrations' AND column_name = 'category_name'
    ) THEN
        ALTER TABLE migrations ADD COLUMN category_name TEXT;
    END IF;
END
$$;

-- Update category_name values and set constraints
UPDATE migrations SET category_name = 'unknown' WHERE category_name IS NULL;

-- Check if the column is already NOT NULL before trying to set it
DO $$
BEGIN
    -- Check if category_name is already NOT NULL
    IF EXISTS (
        SELECT FROM information_schema.columns 
        WHERE table_schema = 'public' AND table_name = 'migrations' 
        AND column_name = 'category_name' AND is_nullable = 'YES'
    ) THEN
        ALTER TABLE migrations ALTER COLUMN category_name SET NOT NULL;
    END IF;
    
    -- Check if from_version is already NOT NULL
    IF EXISTS (
        SELECT FROM information_schema.columns 
        WHERE table_schema = 'public' AND table_name = 'migrations' 
        AND column_name = 'from_version' AND is_nullable = 'YES'
    ) THEN
        ALTER TABLE migrations ALTER COLUMN from_version SET NOT NULL;
    END IF;
    
    -- Check if to_version is already NOT NULL
    IF EXISTS (
        SELECT FROM information_schema.columns 
        WHERE table_schema = 'public' AND table_name = 'migrations' 
        AND column_name = 'to_version' AND is_nullable = 'YES'
    ) THEN
        ALTER TABLE migrations ALTER COLUMN to_version SET NOT NULL;
    END IF;
END
$$;

-- Create index if it doesn't exist (already using IF NOT EXISTS)
CREATE INDEX IF NOT EXISTS idx_migrations_category_name ON migrations(category_name);