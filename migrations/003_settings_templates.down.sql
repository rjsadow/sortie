-- Rollback settings and templates tables

DROP INDEX IF EXISTS idx_templates_category;
DROP INDEX IF EXISTS idx_templates_template_id;
DROP TABLE IF EXISTS templates;
DROP TABLE IF EXISTS settings;
