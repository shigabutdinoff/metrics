// Package server реализует HTTP-сервер сбора метрик.
//
// Сервер хранит метрики в памяти, при необходимости сохраняет их в файл
// или в PostgreSQL и публикует события аудита.
//
// Маршруты:
//
//	GET  /                              список метрик в виде HTML
//	POST /update/{type}/{name}/{value}  приём одной метрики в text/plain
//	GET  /value/{type}/{name}           чтение одной метрики в text/plain
//	POST /update/                       приём одной метрики в JSON
//	POST /updates/                      приём пачки метрик в JSON
//	POST /value/                        чтение одной метрики в JSON
//	GET  /ping                          проверка соединения с БД
//
// Маршруты text/plain и application/json разведены по группам с проверкой
// Content-Type. К обеим группам подключены распаковка и сжатие gzip,
// а также проверка подписи HMAC-SHA256, если задан Server.Key.
//
// На адресе Server.GRPCAddress сервер обслуживает сервис gRPC Metrics.
// Пустой адрес отключает gRPC.
package server
