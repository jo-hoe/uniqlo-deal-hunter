package config

import (
	"bytes"

	"github.com/shopspring/decimal"
)

// newBytesReader wraps a byte slice as an io.Reader for the yaml decoder.
// A tiny helper so the caller does not have to import bytes.
func newBytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// mustDecForTest parses a decimal literal that is guaranteed to be
// well-formed at compile time. Only called from test helpers with
// hard-coded strings; NEVER on data from an external source. Runtime
// price data goes through decimal.NewFromFloat / NewFromString error
// returns and is handled with a proper error path in the mapper.
func mustDecForTest(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic("mustDecForTest: " + err.Error())
	}
	return d
}
