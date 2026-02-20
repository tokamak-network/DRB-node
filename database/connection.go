package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-pg/pg/v10"
	_ "github.com/lib/pq"
)

var (
	dbClient *pg.DB
	once     sync.Once
)

func InitSQLDB(ctx context.Context, port int, host, user, password, name string) error {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, password, host, port, name)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect SQL DB, %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("error pinging database for migration: %v", err)
	}

	if err := MigrationsUp(db); err != nil {
		return fmt.Errorf("failed to run migration up, %v", err)
	}

	once.Do(func() {
		opts := &pg.Options{
			User:                  user,
			Password:              password,
			Database:              name,
			Addr:                  fmt.Sprintf("%s:%d", host, port),
			MinIdleConns:          10,
			MaxConnAge:            10 * time.Minute,
			IdleTimeout:           5 * time.Minute,
			PoolSize:              45,
			PoolTimeout:           5 * time.Minute,
			IdleCheckFrequency:    1 * time.Minute,
			MaxRetries:            3,
			RetryStatementTimeout: true,
		}
		dbClient = pg.Connect(opts)

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := dbClient.Ping(ctx); err != nil {
			dbClient = nil
			panic(fmt.Sprintf("Error connecting main DB client (go-pg): %v", err))
		}
	})

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := GetDB().Ping(ctx); err != nil {
		return fmt.Errorf("error pinging main DB client after initialization: %v", err)
	}

	log.Println("Database initialised successfully.")
	return nil
}

func GetDB() *pg.DB {
	if dbClient == nil {
		panic("database client is not initialized. Call InitialiseDB first.")
	}
	return dbClient
}

// Close gracefully closes the database connection
func Close() error {
	if dbClient != nil {
		log.Println("Closing database connection...")
		err := dbClient.Close()
		if err != nil {
			return fmt.Errorf("error closing database: %v", err)
		}
		log.Println("Database connection closed successfully.")
		return nil
	}
	return nil
}
