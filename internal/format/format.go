package format

import (
	"fmt"
	"math"
)

// Timestamp formats seconds as M:SS.mmm or H:MM:SS.mmm
func Timestamp(seconds float64) string {
	ms := int(math.Round(seconds * 1000))
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	ms %= 1000
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d.%03d", h, m, s, ms)
	}
	return fmt.Sprintf("%d:%02d.%03d", m, s, ms)
}
