-- Add settings and templates tables

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS templates (
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

CREATE INDEX IF NOT EXISTS idx_templates_template_id ON templates(template_id);
CREATE INDEX IF NOT EXISTS idx_templates_category ON templates(template_category);
