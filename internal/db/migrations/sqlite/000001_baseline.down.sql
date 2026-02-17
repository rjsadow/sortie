-- Drop all tables in reverse dependency order
DROP TABLE IF EXISTS session_shares;
DROP TABLE IF EXISTS recordings;
DROP TABLE IF EXISTS category_approved_users;
DROP TABLE IF EXISTS category_admins;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS tenants;
DROP TABLE IF EXISTS oidc_states;
DROP TABLE IF EXISTS app_specs;
DROP TABLE IF EXISTS templates;
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS analytics;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS applications;
