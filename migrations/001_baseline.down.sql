-- Rollback baseline migration
-- WARNING: This will drop all core tables and data

DROP INDEX IF EXISTS idx_sessions_status;
DROP INDEX IF EXISTS idx_sessions_user_id;
DROP INDEX IF EXISTS idx_analytics_timestamp;
DROP INDEX IF EXISTS idx_analytics_app_id;
DROP INDEX IF EXISTS idx_audit_timestamp;

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS analytics;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS applications;
