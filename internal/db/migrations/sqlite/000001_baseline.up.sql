-- Baseline schema: consolidated final-state of all tables
-- This represents the complete schema as of the golang-migrate adoption.

CREATE TABLE applications (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    url TEXT NOT NULL,
    icon TEXT NOT NULL,
    category TEXT NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'public',
    launch_type TEXT NOT NULL DEFAULT 'url',
    os_type TEXT DEFAULT 'linux',
    container_image TEXT DEFAULT '',
    container_port INTEGER DEFAULT 0,
    container_args TEXT DEFAULT '[]',
    cpu_request TEXT DEFAULT '',
    cpu_limit TEXT DEFAULT '',
    memory_request TEXT DEFAULT '',
    memory_limit TEXT DEFAULT '',
    egress_policy TEXT DEFAULT '',
    tenant_id TEXT DEFAULT 'default'
);

CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    user TEXT NOT NULL,
    action TEXT NOT NULL,
    details TEXT NOT NULL,
    tenant_id TEXT DEFAULT 'default'
);

CREATE TABLE analytics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    tenant_id TEXT DEFAULT 'default',
    FOREIGN KEY (app_id) REFERENCES applications(id)
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    app_id TEXT NOT NULL,
    pod_name TEXT NOT NULL,
    pod_ip TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    idle_timeout INTEGER DEFAULT 0,
    tenant_id TEXT DEFAULT 'default',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (app_id) REFERENCES applications(id)
);

CREATE INDEX idx_audit_timestamp ON audit_log(timestamp);
CREATE INDEX idx_analytics_app_id ON analytics(app_id);
CREATE INDEX idx_analytics_timestamp ON analytics(timestamp);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_status ON sessions(status);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT,
    display_name TEXT,
    password_hash TEXT NOT NULL,
    roles TEXT DEFAULT '["user"]',
    auth_provider TEXT DEFAULT 'local',
    auth_provider_id TEXT DEFAULT '',
    tenant_id TEXT DEFAULT 'default',
    tenant_roles TEXT DEFAULT '[]',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_users_username ON users(username);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id TEXT UNIQUE NOT NULL,
    template_version TEXT NOT NULL DEFAULT '1.0.0',
    template_category TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    url TEXT DEFAULT '',
    icon TEXT DEFAULT '',
    category TEXT NOT NULL,
    launch_type TEXT NOT NULL DEFAULT 'container',
    os_type TEXT DEFAULT 'linux',
    container_image TEXT,
    container_port INTEGER DEFAULT 8080,
    container_args TEXT DEFAULT '[]',
    tags TEXT DEFAULT '[]',
    maintainer TEXT,
    documentation_url TEXT,
    cpu_request TEXT DEFAULT '',
    cpu_limit TEXT DEFAULT '',
    memory_request TEXT DEFAULT '',
    memory_limit TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_templates_template_id ON templates(template_id);
CREATE INDEX idx_templates_category ON templates(template_category);

CREATE TABLE app_specs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    image TEXT NOT NULL,
    launch_command TEXT DEFAULT '',
    cpu_request TEXT DEFAULT '',
    cpu_limit TEXT DEFAULT '',
    memory_request TEXT DEFAULT '',
    memory_limit TEXT DEFAULT '',
    env_vars TEXT DEFAULT '[]',
    volumes TEXT DEFAULT '[]',
    network_rules TEXT DEFAULT '[]',
    egress_policy TEXT DEFAULT '',
    tenant_id TEXT DEFAULT 'default',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE oidc_states (
    state TEXT PRIMARY KEY,
    redirect_url TEXT NOT NULL DEFAULT '',
    expires_at DATETIME NOT NULL
);
CREATE INDEX idx_oidc_states_expires ON oidc_states(expires_at);

CREATE TABLE tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    settings TEXT DEFAULT '{}',
    quotas TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_tenants_slug ON tenants(slug);

-- Seed default tenant
INSERT INTO tenants (id, name, slug, settings, quotas) VALUES ('default', 'Default', 'default', '{}', '{}');

CREATE TABLE categories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    tenant_id TEXT DEFAULT 'default',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_categories_tenant ON categories(tenant_id);

CREATE TABLE category_admins (
    category_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    PRIMARY KEY (category_id, user_id)
);

CREATE TABLE category_approved_users (
    category_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    PRIMARY KEY (category_id, user_id)
);

CREATE TABLE recordings (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    size_bytes INTEGER DEFAULT 0,
    duration_seconds REAL DEFAULT 0,
    format TEXT NOT NULL DEFAULT 'webm',
    storage_backend TEXT NOT NULL DEFAULT 'local',
    storage_path TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'recording',
    tenant_id TEXT DEFAULT 'default',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    video_path TEXT DEFAULT '',
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);
CREATE INDEX idx_recordings_session ON recordings(session_id);
CREATE INDEX idx_recordings_user ON recordings(user_id);
CREATE INDEX idx_recordings_status ON recordings(status);

CREATE TABLE session_shares (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    permission TEXT NOT NULL DEFAULT 'read_only',
    share_token TEXT UNIQUE,
    created_by TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
CREATE INDEX idx_session_shares_session ON session_shares(session_id);
CREATE INDEX idx_session_shares_user ON session_shares(user_id);
CREATE INDEX idx_session_shares_token ON session_shares(share_token);
