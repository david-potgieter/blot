// Package size parses and formats human-readable byte sizes.
package size

import (
	"fmt"
	"math"
	"strings"

	"github.com/dustin/go-humanize"
)

// Parse converts strings like "500MB", "1.5GiB" or "1024" into bytes.
func Parse(s string) (int64, error) {
	n, err := humanize.ParseBytes(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid size %q (try 500MB or 1GiB)", s)
	}
	if n > math.MaxInt64 {
		return 0, fmt.Errorf("size %q too large", s)
	}
	return int64(n), nil
}

// Format renders bytes with SI units, e.g. 1500000 → "1.5 MB".
func Format(n int64) string {
	if n < 0 {
		n = 0
	}
	return humanize.Bytes(uint64(n))
}
