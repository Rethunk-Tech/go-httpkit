package download

import (
	"strconv"
	"strings"
)

// parseContentRange reads "bytes start-end/total". total is 0 when the server sends "*". It accepts only a range
// that is a real interval of a file that holds it: 0 <= start <= end and total either "*" or above end.
func parseContentRange(h string) (start, end, total int64, ok bool) {
	h = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(h), "bytes"))
	rangePart, totalPart, found := strings.Cut(h, "/")
	if !found {
		return 0, 0, 0, false
	}
	from, to, found := strings.Cut(rangePart, "-")
	if !found {
		return 0, 0, 0, false
	}
	start, err := strconv.ParseInt(from, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, 0, false
	}
	end, err = strconv.ParseInt(to, 10, 64)
	if err != nil || end < start {
		return 0, 0, 0, false
	}
	if totalPart != "*" {
		total, err = strconv.ParseInt(totalPart, 10, 64)
		if err != nil || total <= end {
			return 0, 0, 0, false
		}
	}
	return start, end, total, true
}
