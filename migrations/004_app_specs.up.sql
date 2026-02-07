-- App specifications for container-based application definitions
CREATE TABLE IF NOT EXISTS app_specs (
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
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
