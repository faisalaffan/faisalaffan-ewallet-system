package decimal_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	pkgdecimal "github.com/faisalaffan/ewallet-system/pkg/decimal"
)

func TestNewFromString(t *testing.T) {
	t.Parallel()

	t.Run("valid string", func(t *testing.T) {
		d, err := pkgdecimal.NewFromString("12.50")
		assert.NoError(t, err)
		assert.True(t, decimal.NewFromFloat(12.50).Equal(d))
	})

	t.Run("invalid string returns error", func(t *testing.T) {
		_, err := pkgdecimal.NewFromString("abc")
		assert.Error(t, err)
	})

	t.Run("empty string returns error", func(t *testing.T) {
		_, err := pkgdecimal.NewFromString("")
		assert.Error(t, err)
	})
}

func TestNewFromFloat(t *testing.T) {
	t.Parallel()

	t.Run("float 12.5", func(t *testing.T) {
		d := pkgdecimal.NewFromFloat(12.5)
		assert.True(t, decimal.NewFromFloat(12.5).Equal(d))
	})

	t.Run("float 0", func(t *testing.T) {
		d := pkgdecimal.NewFromFloat(0)
		assert.True(t, decimal.NewFromFloat(0).Equal(d))
	})

	t.Run("float negative", func(t *testing.T) {
		d := pkgdecimal.NewFromFloat(-5.25)
		assert.True(t, decimal.NewFromFloat(-5.25).Equal(d))
	})
}

func TestZero(t *testing.T) {
	t.Parallel()

	z := pkgdecimal.Zero()
	assert.True(t, decimal.NewFromFloat(0).Equal(z))
	assert.Equal(t, "0", z.String())
}

func TestRound(t *testing.T) {
	t.Parallel()

	t.Run("round 12.345 to 2 places", func(t *testing.T) {
		d := decimal.NewFromFloat(12.345)
		rounded := pkgdecimal.Round(d, 2)
		assert.Equal(t, "12.35", rounded.StringFixed(2))
	})

	t.Run("round 12.344 to 2 places", func(t *testing.T) {
		d := decimal.NewFromFloat(12.344)
		rounded := pkgdecimal.Round(d, 2)
		assert.Equal(t, "12.34", rounded.StringFixed(2))
	})

	t.Run("round 12.345 to 0 places", func(t *testing.T) {
		d := decimal.NewFromFloat(12.345)
		rounded := pkgdecimal.Round(d, 0)
		assert.Equal(t, "12", rounded.StringFixed(0))
	})

	t.Run("round negative value", func(t *testing.T) {
		d := decimal.NewFromFloat(-12.345)
		rounded := pkgdecimal.Round(d, 1)
		assert.Equal(t, "-12.3", rounded.StringFixed(1))
	})
}

func TestRoundHalfUp(t *testing.T) {
	t.Parallel()

	t.Run("round 12.345 to 2 places half-up", func(t *testing.T) {
		d := decimal.NewFromFloat(12.345)
		rounded := pkgdecimal.RoundHalfUp(d)
		assert.Equal(t, "12.35", rounded.StringFixed(2))
	})

	t.Run("round 12.344 to 2 places half-up", func(t *testing.T) {
		d := decimal.NewFromFloat(12.344)
		rounded := pkgdecimal.RoundHalfUp(d)
		assert.Equal(t, "12.34", rounded.StringFixed(2))
	})

	t.Run("round 1.005", func(t *testing.T) {
		d := decimal.NewFromFloat(1.005)
		rounded := pkgdecimal.RoundHalfUp(d)
		assert.Equal(t, "1.01", rounded.StringFixed(2))
	})
}

func TestValidatePositive(t *testing.T) {
	t.Parallel()

	minAmount := decimal.NewFromFloat(0.01)

	t.Run("amount >= 0.01 returns nil", func(t *testing.T) {
		d := decimal.NewFromFloat(0.01)
		err := pkgdecimal.ValidatePositive(d, minAmount)
		assert.NoError(t, err)
	})

	t.Run("amount > 0.01 returns nil", func(t *testing.T) {
		d := decimal.NewFromFloat(100.00)
		err := pkgdecimal.ValidatePositive(d, minAmount)
		assert.NoError(t, err)
	})

	t.Run("amount < 0.01 returns ErrAmountTooSmall", func(t *testing.T) {
		d := decimal.NewFromFloat(0.001)
		err := pkgdecimal.ValidatePositive(d, minAmount)
		assert.ErrorIs(t, err, pkgdecimal.ErrAmountTooSmall)
	})

	t.Run("amount equal to minAmount returns nil", func(t *testing.T) {
		d := decimal.NewFromFloat(0.01)
		err := pkgdecimal.ValidatePositive(d, d)
		assert.NoError(t, err)
	})

	t.Run("zero amount returns ErrAmountTooSmall", func(t *testing.T) {
		d := decimal.NewFromFloat(0)
		err := pkgdecimal.ValidatePositive(d, minAmount)
		assert.ErrorIs(t, err, pkgdecimal.ErrAmountTooSmall)
	})
}

func TestErrAmountTooSmall(t *testing.T) {
	t.Parallel()

	err := pkgdecimal.ErrAmountTooSmall
	assert.Error(t, err)
	assert.Equal(t, "amount must be at least 0.01 after rounding", err.Error())
}
