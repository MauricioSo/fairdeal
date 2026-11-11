package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Cents represents monetary amounts in USD cents as an integer to avoid float inaccuracies.
type Cents int64

var (
	ErrInvalidAmount = errors.New("invalid monetary amount")
	ErrNegative      = errors.New("monetary amount cannot be negative")
)

// Parse converts a decimal string representation of USD (e.g. "94.00", "0.50", "1") to Cents.
// It rejects negative values, more than 2 decimal places, scientific notation, and non-numeric chars.
func Parse(s string) (Cents, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalidAmount
	}

	// Reject signs
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, ErrNegative
	}

	// Reject scientific notation
	if strings.ContainsAny(s, "eE") {
		return 0, ErrInvalidAmount
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, ErrInvalidAmount
	}

	dollarsPart := parts[0]
	if dollarsPart == "" {
		dollarsPart = "0"
	}
	dollars, err := strconv.ParseInt(dollarsPart, 10, 64)
	if err != nil || dollars < 0 {
		return 0, ErrInvalidAmount
	}

	var cents int64
	if len(parts) == 2 {
		centsPart := parts[1]
		if len(centsPart) > 2 {
			return 0, ErrInvalidAmount
		}
		if len(centsPart) == 1 {
			centsPart += "0"
		} else if len(centsPart) == 0 {
			centsPart = "00"
		}
		c, err := strconv.ParseInt(centsPart, 10, 64)
		if err != nil || c < 0 {
			return 0, ErrInvalidAmount
		}
		cents = c
	}

	total := dollars*100 + cents
	return Cents(total), nil
}

// Format returns the standard 2-decimal USD string representation, e.g. "94.00".
func (c Cents) Format() string {
	dollars := c / 100
	cents := c % 100
	if cents < 0 {
		cents = -cents
	}
	return fmt.Sprintf("%d.%02d", dollars, cents)
}

// String implements fmt.Stringer.
func (c Cents) String() string {
	return c.Format()
}
