package response

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuccessCreated(t *testing.T) {
	data := map[string]string{"wallet_id": "abc-123"}
	env := SuccessCreated(data)

	assert.Equal(t, "success", env.Status)
	assert.Equal(t, data, env.Data)
	assert.Nil(t, env.Error_)
	assert.Nil(t, env.Pagination)
}

func TestSuccessOK(t *testing.T) {
	data := map[string]interface{}{
		"balance": "100.00",
		"currency": "USD",
	}
	env := SuccessOK(data)

	assert.Equal(t, "success", env.Status)
	assert.Equal(t, data, env.Data)
	assert.Nil(t, env.Error_)
	assert.Nil(t, env.Pagination)
}

func TestSuccessPaginated(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		data := []string{"a", "b", "c"}
		env := SuccessPaginated(data, 1, 10, 100)

		assert.Equal(t, "success", env.Status)
		assert.Equal(t, data, env.Data)
		assert.Nil(t, env.Error_)
		require := assert.New(t)
		if require.NotNil(env.Pagination) {
			assert.Equal(t, 1, env.Pagination.Page)
			assert.Equal(t, 10, env.Pagination.PerPage)
			assert.Equal(t, int64(100), env.Pagination.Total)
			assert.Equal(t, int64(10), env.Pagination.TotalPages)
		}
	})

	t.Run("total is zero", func(t *testing.T) {
		data := []string{}
		env := SuccessPaginated(data, 1, 10, 0)

		assert.Equal(t, "success", env.Status)
		if assert.NotNil(t, env.Pagination) {
			assert.Equal(t, 1, env.Pagination.Page)
			assert.Equal(t, 10, env.Pagination.PerPage)
			assert.Equal(t, int64(0), env.Pagination.Total)
			assert.Equal(t, int64(0), env.Pagination.TotalPages)
		}
	})

	t.Run("uneven pages", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		env := SuccessPaginated(data, 2, 15, 50)

		assert.Equal(t, "success", env.Status)
		if assert.NotNil(t, env.Pagination) {
			assert.Equal(t, 2, env.Pagination.Page)
			assert.Equal(t, 15, env.Pagination.PerPage)
			assert.Equal(t, int64(50), env.Pagination.Total)
			// ceil(50/15) = 4
			assert.Equal(t, int64(4), env.Pagination.TotalPages)
		}
	})
}

func TestError(t *testing.T) {
	env := Error(400, "BAD_REQUEST", "invalid request payload")

	assert.Equal(t, "error", env.Status)
	assert.Nil(t, env.Data)
	assert.Nil(t, env.Pagination)

	if assert.NotNil(t, env.Error_) {
		assert.Equal(t, "BAD_REQUEST", env.Error_.Code)
		assert.Equal(t, "invalid request payload", env.Error_.Message)
		assert.Empty(t, env.Error_.Details)
	}
}

func TestValidationError(t *testing.T) {
	t.Run("without details", func(t *testing.T) {
		env := ValidationError("validation failed", nil)

		assert.Equal(t, "fail", env.Status)
		assert.Nil(t, env.Data)
		assert.Nil(t, env.Pagination)

		if assert.NotNil(t, env.Error_) {
			assert.Equal(t, "VALIDATION_ERROR", env.Error_.Code)
			assert.Equal(t, "validation failed", env.Error_.Message)
			assert.Empty(t, env.Error_.Details)
		}
	})

	t.Run("with details", func(t *testing.T) {
		details := []ValidationDetail{
			{Field: "amount", Message: "must be positive"},
			{Field: "currency", Message: "must be ISO 4217"},
		}
		env := ValidationError("validation failed", details)

		assert.Equal(t, "fail", env.Status)
		if assert.NotNil(t, env.Error_) {
			assert.Equal(t, "VALIDATION_ERROR", env.Error_.Code)
			assert.Equal(t, "validation failed", env.Error_.Message)
			assert.Len(t, env.Error_.Details, 2)
			assert.Equal(t, "amount", env.Error_.Details[0].Field)
			assert.Equal(t, "must be positive", env.Error_.Details[0].Message)
			assert.Equal(t, "currency", env.Error_.Details[1].Field)
			assert.Equal(t, "must be ISO 4217", env.Error_.Details[1].Message)
		}
	})
}
