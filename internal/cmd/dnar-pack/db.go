package main

import (
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

const SQL_DIALECT = "sqlite"

func migrateDB(db *sql.DB, fs embed.FS) error {
	goose.SetLogger(goose.NopLogger())

	goose.SetBaseFS(fs)

	if err := goose.SetDialect(SQL_DIALECT); err != nil {
		return err
	}

	if err := goose.Up(db, "schema"); err != nil {
		return err
	}

	return nil
}
