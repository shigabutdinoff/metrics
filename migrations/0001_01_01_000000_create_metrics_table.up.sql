-- migrations/0001_01_01_000000_create_metrics_table.up.sql
-- Создание таблицы метрик
CREATE TABLE metrics
(
    id    VARCHAR(255) PRIMARY KEY,
    type  VARCHAR(255)     NOT NULL,
    delta BIGINT           NULL,
    value DOUBLE PRECISION NULL,
    hash  VARCHAR(255)     NULL
);

-- Индекс для поиска по типу
CREATE INDEX idx_metrics_type ON metrics (type);

-- Составной индекс для поиска по id и type
CREATE INDEX idx_metrics_id_type ON metrics (id, type);
