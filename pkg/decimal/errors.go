package decimal

import "errors"

var ErrAmountTooSmall = errors.New("amount must be at least 0.01 after rounding")
