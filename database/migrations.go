package database

import (
	"database/sql"
	"fmt"

	"github.com/gobuffalo/packr/v2"
	migrate "github.com/rubenv/sql-migrate"
)

var migrations *migrate.PackrMigrationSource

func init() {
	migrations = &migrate.PackrMigrationSource{
		Box: packr.New("drb-db-migrations", "./migrations"),
	}
	validateMigrations(migrations)
}

// validateMigrations checks if migrations are valid and panics if not
func validateMigrations(migrationSource *migrate.PackrMigrationSource) {
	ms, err := migrationSource.FindMigrations()
	if err != nil {
		panic(err)
	}
	if len(ms) == 0 {
		panic(fmt.Errorf("no SQL migrations found"))
	}
}


func MigrationsUp(db *sql.DB) error {
	n, err := migrate.Exec(db, "postgres", migrations, migrate.Up)
	if err != nil {
		return err
	}
	fmt.Printf("successfully ran migration up: %d \n", n)
	return nil
}

func MigrationsDown(db *sql.DB) error {
	_, err := migrate.Exec(db, "postgres", migrations, migrate.Down)
	if err != nil {
		return err
	}
	fmt.Println("successfully ran migration down")
	return nil
}
