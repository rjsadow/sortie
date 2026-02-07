-- Baseline migration: core application schema
-- Tables: applications, audit_log, analytics, sessions

CREATE TABLE IF NOT EXISTS applications (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    url TEXT NOT NULL,
    icon TEXT NOT NULL,
    category TEXT NOT NULL,
    launch_type TEXT NOT NULL DEFAULT 'url',
    os_type TEXT DEFAULT 'linux',
    container_image TEXT DEFAULT '',
    container_port INTEGER DEFAULT 0,
    container_args TEXT DEFAULT '[]',
    cpu_request TEXT DEFAULT '',
    cpu_limit TEXT DEFAULT '',
    memory_request TEXT DEFAULT '',
    memory_limit TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    user TEXT NOT NULL,
    action TEXT NOT NULL,
    details TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS analytics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (app_id) REFERENCES applications(id)
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    app_id TEXT NOT NULL,
    pod_name TEXT NOT NULL,
    pod_ip TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (app_id) REFERENCES applications(id)
);

CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_analytics_app_id ON analytics(app_id);
CREATE INDEX IF NOT EXISTS idx_analytics_timestamp ON analytics(timestamp);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
