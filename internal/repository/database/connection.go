package database

import (
	"database/sql"
	"errors"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Connection(ps string) (*sql.DB, error) {
	if ps == "" {
		return nil, errors.New("не удалось получить адрес подключения к БД")
	}

	db, err := sql.Open("pgx", ps)
	if err != nil {
		return nil, err
	}

	return db, nil
}
