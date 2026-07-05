package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setEnvs(vals map[string]string) func() {
	saved := make(map[string]string, len(vals))
	for k := range vals {
		saved[k] = os.Getenv(k)
	}
	for k, v := range vals {
		os.Setenv(k, v)
	}
	return func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}
}

func TestConfig_Envs(t *testing.T) {
	restore := setEnvs(map[string]string{
		"APP_PORT":    "8080",
		"DB_HOST":     "localhost",
		"DB_PORT":     "5432",
		"DB_USER":     "postgres",
		"DB_PASSWORD": "postgres",
		"DB_NAME":     "ewallet",
	})
	defer restore()

	viper.Reset()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "8080", cfg.AppPort)
	assert.Equal(t, "localhost", cfg.DBHost)
	assert.Equal(t, "5432", cfg.DBPort)
	assert.Equal(t, "postgres", cfg.DBUser)
	assert.Equal(t, "postgres", cfg.DBPassword)
	assert.Equal(t, "ewallet", cfg.DBName)
}

func TestConfig_CustomEnvs(t *testing.T) {
	restore := setEnvs(map[string]string{
		"APP_PORT":    "9090",
		"DB_HOST":     "db-test.example.com",
		"DB_PORT":     "15432",
		"DB_USER":     "testuser",
		"DB_PASSWORD": "testpass",
		"DB_NAME":     "testdb",
	})
	defer restore()

	viper.Reset()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "9090", cfg.AppPort)
	assert.Equal(t, "db-test.example.com", cfg.DBHost)
	assert.Equal(t, "15432", cfg.DBPort)
	assert.Equal(t, "testuser", cfg.DBUser)
	assert.Equal(t, "testpass", cfg.DBPassword)
	assert.Equal(t, "testdb", cfg.DBName)
}

func TestConfig_DSN(t *testing.T) {
	c := &Config{
		DBHost:     "myhost",
		DBPort:     "9999",
		DBUser:     "admin",
		DBPassword: "s3cret",
		DBName:     "mydb",
	}

	dsn := c.DSN()

	assert.Contains(t, dsn, "host=myhost")
	assert.Contains(t, dsn, "port=9999")
	assert.Contains(t, dsn, "user=admin")
	assert.Contains(t, dsn, "password=s3cret")
	assert.Contains(t, dsn, "dbname=mydb")
	assert.Contains(t, dsn, "sslmode=disable")
	assert.Contains(t, dsn, "TimeZone=Asia/Jakarta")
}
