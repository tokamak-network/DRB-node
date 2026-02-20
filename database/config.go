package database

import (
	appconfig "github.com/tokamak-network/DRB-node/config"
)

// Config holds the application configuration
type Config struct {
	// HistoryDB configuration
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresName     string
	PostgresSSLMode  string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	env := appconfig.Get()
	dbConfig := env.Database

	config := &Config{
		PostgresHost:     dbConfig.PostgresHost,
		PostgresPort:     dbConfig.PostgresPort,
		PostgresUser:     dbConfig.PostgresUser,
		PostgresPassword: dbConfig.PostgresPassword,
		PostgresName:     dbConfig.PostgresName,
		PostgresSSLMode:  dbConfig.PostgresSSLMode,
	}

	return config
}
