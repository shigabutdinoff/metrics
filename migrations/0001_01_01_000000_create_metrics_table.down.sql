-- migrations/0001_01_01_000000_create_metrics_table.down.sql
-- Откат создания таблицы метрик
DROP INDEX IF EXISTS idx_metrics_type;
DROP INDEX IF EXISTS idx_metrics_id_type;
DROP TABLE IF EXISTS metrics;
