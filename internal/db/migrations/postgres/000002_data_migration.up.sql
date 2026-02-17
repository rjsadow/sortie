-- Data migration: create category records for existing app categories.
-- Both operations are idempotent.

-- Migrate existing app categories into the categories table.
-- Uses ON CONFLICT DO NOTHING to skip categories that already exist (by name uniqueness).
INSERT INTO categories (id, name, description, tenant_id, created_at, updated_at)
SELECT
    'cat-' || category || '-migrated',
    category,
    '',
    'default',
    NOW(),
    NOW()
FROM applications
WHERE category != ''
GROUP BY category
ON CONFLICT DO NOTHING;
