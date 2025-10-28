package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/go-pg/pg/v10"
	_ "github.com/lib/pq"
)

var testDB *pg.DB

const (
	POSTGRES_HOST     = "localhost"
	POSTGRES_USER     = "postgres"
	POSTGRES_PASSWORD = "123"
	POSTGRES_DB       = "testdb"
	POSTGRES_PORT     = "5433"
)

func TestMain(m *testing.M) {
	// Initialize test database
	var err error
	testDB, err = initTestDB()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize test DB: %v", err))
	}

	// Set the global dbClient for tests
	dbClient = testDB

	log.Println("Database initialized successfully.")

	// Run tests
	code := m.Run()

	// Cleanup
	testDB.Close()

	os.Exit(code)
}

func initTestDB() (*pg.DB, error) {
	// First, run migrations using database/sql
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_HOST, POSTGRES_PORT, POSTGRES_DB)

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQL DB for migrations: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("error pinging SQL DB: %w", err)
	}

	// Run migrations
	if err := MigrationsUp(sqlDB); err != nil {
		return nil, fmt.Errorf("error running migrations: %w", err)
	}

	// Now connect using go-pg
	db := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", POSTGRES_HOST, POSTGRES_PORT),
		User:     POSTGRES_USER,
		Password: POSTGRES_PASSWORD,
		Database: POSTGRES_DB,
	})

	// Test connection
	if err := db.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("error pinging database: %w", err)
	}

	return db, nil
}
