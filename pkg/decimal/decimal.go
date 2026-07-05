package decimal

import (
	"github.com/shopspring/decimal"
)

func NewFromString(s string) (decimal.Decimal, error) {
	return decimal.NewFromString(s)
}

func NewFromFloat(f float64) decimal.Decimal {
	return decimal.NewFromFloat(f)
}

func Zero() decimal.Decimal {
	return decimal.Zero
}

func Round(d decimal.Decimal, places int32) decimal.Decimal {
	return d.Round(places)
}

func RoundHalfUp(d decimal.Decimal) decimal.Decimal {
	return d.Round(2)
}

func ValidatePositive(d decimal.Decimal, minAmount decimal.Decimal) error {
	if d.LessThan(minAmount) {
		return ErrAmountTooSmall
	}
	return nil
}
