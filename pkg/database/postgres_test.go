package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// brokenDialector implements gorm.Dialector and always fails to initialize,
// providing a deterministic error path for TestNewGorm_Error.
type brokenDialector struct{}

func (brokenDialector) Name() string                                                { return "broken" }
func (brokenDialector) Initialize(*gorm.DB) error                                   { return fmt.Errorf("broken dialector: cannot initialize") }
func (brokenDialector) Migrator(*gorm.DB) gorm.Migrator                             { return nil }
func (brokenDialector) DataTypeOf(*schema.Field) string                             { return "" }
func (brokenDialector) DefaultValueOf(*schema.Field) clause.Expression              { return nil }
func (brokenDialector) BindVarTo(clause.Writer, *gorm.Statement, interface{})       {}
func (brokenDialector) QuoteTo(clause.Writer, string)                               {}
func (brokenDialector) Explain(sql string, vars ...interface{}) string              { return "" }

func TestNewGorm_Success(t *testing.T) {
	db, err := NewGorm(sqlite.Open(":memory:"))
	require.NoError(t, err)
	require.NotNil(t, db)

	// Verify the connection is usable
	sqlDB, err := db.DB()
	require.NoError(t, err)
	err = sqlDB.Ping()
	assert.NoError(t, err)
}

func TestNewGorm_Error(t *testing.T) {
	db, err := NewGorm(brokenDialector{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken dialector")
	assert.Nil(t, db)
}

func TestNewPostgres_Error(t *testing.T) {
	db, err := NewPostgres("")
	require.Error(t, err)
	assert.Nil(t, db)
}
