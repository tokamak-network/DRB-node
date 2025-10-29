package database

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig_WithDefaults(t *testing.T) {
	// Clear all environment variables
	os.Unsetenv("POSTGRES_HOST")
	os.Unsetenv("POSTGRES_PORT")
	os.Unsetenv("POSTGRES_USER")
	os.Unsetenv("POSTGRES_PASSWORD")
	os.Unsetenv("POSTGRES_NAME")
	os.Unsetenv("POSTGRES_SSLMODE")

	config := LoadConfig()

	assert.NotNil(t, config)
	assert.Equal(t, "localhost", config.PostgresHost)
	assert.Equal(t, 5432, config.PostgresPort)
	assert.Equal(t, "postgres", config.PostgresUser)
	assert.Equal(t, "", config.PostgresPassword)
	assert.Equal(t, "postgres", config.PostgresName)
	assert.Equal(t, "disable", config.PostgresSSLMode)
}

func TestLoadConfig_WithEnvironmentVariables(t *testing.T) {
	// Set environment variables
	os.Setenv("POSTGRES_HOST", "custom-host")
	os.Setenv("POSTGRES_PORT", "5433")
	os.Setenv("POSTGRES_USER", "custom-user")
	os.Setenv("POSTGRES_PASSWORD", "custom-password")
	os.Setenv("POSTGRES_NAME", "custom-db")
	os.Setenv("POSTGRES_SSLMODE", "require")

	config := LoadConfig()

	assert.NotNil(t, config)
	assert.Equal(t, "custom-host", config.PostgresHost)
	assert.Equal(t, 5433, config.PostgresPort)
	assert.Equal(t, "custom-user", config.PostgresUser)
	assert.Equal(t, "custom-password", config.PostgresPassword)
	assert.Equal(t, "custom-db", config.PostgresName)
	assert.Equal(t, "require", config.PostgresSSLMode)

	// Cleanup
	os.Unsetenv("POSTGRES_HOST")
	os.Unsetenv("POSTGRES_PORT")
	os.Unsetenv("POSTGRES_USER")
	os.Unsetenv("POSTGRES_PASSWORD")
	os.Unsetenv("POSTGRES_NAME")
	os.Unsetenv("POSTGRES_SSLMODE")
}

func TestLoadConfig_WithPartialEnvironmentVariables(t *testing.T) {
	// Set only some environment variables
	os.Setenv("POSTGRES_HOST", "partial-host")
	os.Setenv("POSTGRES_PORT", "9999")
	os.Unsetenv("POSTGRES_USER")
	os.Unsetenv("POSTGRES_PASSWORD")
	os.Unsetenv("POSTGRES_NAME")
	os.Unsetenv("POSTGRES_SSLMODE")

	config := LoadConfig()

	assert.NotNil(t, config)
	assert.Equal(t, "partial-host", config.PostgresHost)
	assert.Equal(t, 9999, config.PostgresPort)
	assert.Equal(t, "postgres", config.PostgresUser)   // default
	assert.Equal(t, "", config.PostgresPassword)       // default
	assert.Equal(t, "postgres", config.PostgresName)   // default
	assert.Equal(t, "disable", config.PostgresSSLMode) // default

	// Cleanup
	os.Unsetenv("POSTGRES_HOST")
	os.Unsetenv("POSTGRES_PORT")
}

func TestGetEnv_WithValueSet(t *testing.T) {
	key := "TEST_ENV_VAR"
	expectedValue := "test-value"

	os.Setenv(key, expectedValue)
	defer os.Unsetenv(key)

	result := getEnv(key, "default-value")
	assert.Equal(t, expectedValue, result)
}

func TestGetEnv_WithEmptyValue(t *testing.T) {
	key := "TEST_EMPTY_VAR"
	defaultValue := "default-value"

	os.Unsetenv(key)

	result := getEnv(key, defaultValue)
	assert.Equal(t, defaultValue, result)
}

func TestGetEnv_WithEmptyStringSet(t *testing.T) {
	key := "TEST_EMPTY_STRING"
	defaultValue := "default-value"

	// Explicitly set to empty string
	os.Setenv(key, "")
	defer os.Unsetenv(key)

	result := getEnv(key, defaultValue)
	// When env var is set to empty string, it should return default
	assert.Equal(t, defaultValue, result)
}

func TestGetEnvInt_WithValidInteger(t *testing.T) {
	key := "TEST_INT_VAR"
	expectedValue := 12345

	os.Setenv(key, "12345")
	defer os.Unsetenv(key)

	result := getEnvInt(key, 999)
	assert.Equal(t, expectedValue, result)
}

func TestGetEnvInt_WithInvalidInteger(t *testing.T) {
	key := "TEST_INVALID_INT"
	defaultValue := 999

	os.Setenv(key, "not-a-number")
	defer os.Unsetenv(key)

	result := getEnvInt(key, defaultValue)
	// Should return default when value is not a valid integer
	assert.Equal(t, defaultValue, result)
}

func TestGetEnvInt_WithEmptyValue(t *testing.T) {
	key := "TEST_UNSET_INT"
	defaultValue := 888

	os.Unsetenv(key)

	result := getEnvInt(key, defaultValue)
	assert.Equal(t, defaultValue, result)
}

func TestGetEnvInt_WithZero(t *testing.T) {
	key := "TEST_ZERO_INT"

	os.Setenv(key, "0")
	defer os.Unsetenv(key)

	result := getEnvInt(key, 999)
	assert.Equal(t, 0, result)
}

func TestGetEnvInt_WithNegativeNumber(t *testing.T) {
	key := "TEST_NEGATIVE_INT"

	os.Setenv(key, "-5432")
	defer os.Unsetenv(key)

	result := getEnvInt(key, 999)
	assert.Equal(t, -5432, result)
}

func TestGetEnvInt_WithFloatingPoint(t *testing.T) {
	key := "TEST_FLOAT_INT"
	defaultValue := 777

	os.Setenv(key, "123.456")
	defer os.Unsetenv(key)

	result := getEnvInt(key, defaultValue)
	// Floating point should fail conversion and return default
	assert.Equal(t, defaultValue, result)
}

func TestLoadConfig_GodotenvError(t *testing.T) {
	// Save current directory
	currentDir, _ := os.Getwd()

	// Change to a directory where we don't have a .env file
	// This will trigger the godotenv.Load() error path (which just logs)
	os.Chdir("/tmp")

	config := LoadConfig()

	// Should still return a valid config with defaults
	assert.NotNil(t, config)
	assert.Equal(t, "localhost", config.PostgresHost)

	// Restore directory
	os.Chdir(currentDir)
}

func TestLoadConfig_WithComplexEnvironment(t *testing.T) {
	// Test with a mix of valid, invalid, and unset values
	os.Setenv("POSTGRES_HOST", "")    // Empty string - should use default
	os.Setenv("POSTGRES_PORT", "abc") // Invalid int - should use default
	os.Setenv("POSTGRES_USER", "valid-user")
	os.Unsetenv("POSTGRES_PASSWORD") // Unset - should use default
	os.Setenv("POSTGRES_NAME", "testdb")
	os.Unsetenv("POSTGRES_SSLMODE") // Unset - should use default

	config := LoadConfig()

	assert.NotNil(t, config)
	assert.Equal(t, "localhost", config.PostgresHost) // default because empty
	assert.Equal(t, 5432, config.PostgresPort)        // default because invalid
	assert.Equal(t, "valid-user", config.PostgresUser)
	assert.Equal(t, "", config.PostgresPassword) // default
	assert.Equal(t, "testdb", config.PostgresName)
	assert.Equal(t, "disable", config.PostgresSSLMode) // default

	// Cleanup
	os.Unsetenv("POSTGRES_HOST")
	os.Unsetenv("POSTGRES_PORT")
	os.Unsetenv("POSTGRES_USER")
	os.Unsetenv("POSTGRES_NAME")
}

func TestLoadConfig_AllFieldsTypes(t *testing.T) {
	// Verify the config struct has all expected fields with correct types
	config := &Config{
		PostgresHost:     "test-host",
		PostgresPort:     1234,
		PostgresUser:     "test-user",
		PostgresPassword: "test-pass",
		PostgresName:     "test-db",
		PostgresSSLMode:  "test-ssl",
	}

	assert.IsType(t, "", config.PostgresHost)
	assert.IsType(t, 0, config.PostgresPort)
	assert.IsType(t, "", config.PostgresUser)
	assert.IsType(t, "", config.PostgresPassword)
	assert.IsType(t, "", config.PostgresName)
	assert.IsType(t, "", config.PostgresSSLMode)
}

func TestGetEnv_MultipleCallsSameKey(t *testing.T) {
	key := "TEST_MULTI_CALL"
	value := "multi-value"

	os.Setenv(key, value)
	defer os.Unsetenv(key)

	// Multiple calls should return same value
	result1 := getEnv(key, "default1")
	result2 := getEnv(key, "default2")

	assert.Equal(t, value, result1)
	assert.Equal(t, value, result2)
}

func TestGetEnvInt_LargeNumber(t *testing.T) {
	key := "TEST_LARGE_INT"

	os.Setenv(key, "2147483647") // Max int32
	defer os.Unsetenv(key)

	result := getEnvInt(key, 0)
	assert.Equal(t, 2147483647, result)
}

func TestGetEnvInt_WithWhitespace(t *testing.T) {
	key := "TEST_WHITESPACE_INT"
	defaultValue := 555

	// Numbers with whitespace should fail conversion
	os.Setenv(key, " 123 ")
	defer os.Unsetenv(key)

	result := getEnvInt(key, defaultValue)
	// strconv.Atoi should fail with whitespace, return default
	assert.Equal(t, defaultValue, result)
}
