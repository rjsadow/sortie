-- Hard multi-tenancy: add tenants table and tenant_id to existing tables

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    settings TEXT DEFAULT '{}',
    quotas TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);

-- Seed a default tenant for backwards compatibility
INSERT OR IGNORE INTO tenants (id, name, slug, settings, quotas) VALUES (
    'default',
    'Default',
    'default',
    '{}',
    '{}'
);
