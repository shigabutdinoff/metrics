package database

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Connection(ps string) (*sql.DB, error) {
	db, err := sql.Open("pgx", ps)
	if err != nil {
		panic(err)
	}

	return db, nil
}
