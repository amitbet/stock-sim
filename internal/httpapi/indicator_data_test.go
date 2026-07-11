package httpapi

import (
	"testing"
	"time"
)

func TestParseStockChartsPastData(t *testing.T) {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	bars := parseStockChartsPastData("$NYADV, Daily\n0 7-10-2026 1200 1400 1100 1350 0\n", "USI:ADVN.NY", from, to)
	if len(bars) != 1 {
		t.Fatalf("len(bars) = %d, want 1", len(bars))
	}
	if bars[0].Date != "2026-07-10" || bars[0].Close != 1350 {
		t.Fatalf("unexpected bar: %+v", bars[0])
	}
}
