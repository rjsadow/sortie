-- Revert hard multi-tenancy

DROP INDEX IF EXISTS idx_tenants_slug;
DROP TABLE IF EXISTS tenants;
