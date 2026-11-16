package download

import "testing"

func TestParseContentRangeUnitAnyCase(t *testing.T) {
	start, end, total, ok := parseContentRange("Bytes 0-3/4")
	if !ok || start != 0 || end != 3 || total != 4 {
		t.Fatalf("parseContentRange(Bytes 0-3/4) = %d, %d, %d, %v; want 0, 3, 4, true", start, end, total, ok)
	}
}
