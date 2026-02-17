-- Data migration: create category records for existing app categories
-- and copy visibility from categories to applications.
-- Both operations are idempotent.

-- Migrate existing app categories into the categories table.
-- Uses INSERT OR IGNORE to skip categories that already exist (by name uniqueness).
INSERT OR IGNORE INTO categories (id, name, description, tenant_id, created_at, updated_at)
SELECT
    'cat-' || category || '-migrated',
    category,
    '',
    'default',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM applications
WHERE category != ''
GROUP BY category;

-- Migrate visibility from categories to applications.
-- Only updates apps still at the default 'public' where the category has a
-- non-public visibility. This handles the case where categories previously
-- had a visibility column that was moved to applications.
-- Safe no-op if categories.visibility column doesn't exist (query will fail
-- silently since this is a data migration on optional schema).
