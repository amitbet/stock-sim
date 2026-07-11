package data

import (
	"testing"
	"time"
)

func TestYahooRateLimitErrorParsesRetryAfter(t *testing.T) {
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	if got := newRateLimitError("Yahoo Finance", "75", now).RetryAfterSeconds; got != 75 {
		t.Fatalf("numeric Retry-After = %d, want 75", got)
	}
	if got := newRateLimitError("Yahoo Finance", "Sat, 11 Jul 2026 09:01:30 GMT", now).RetryAfterSeconds; got != 90 {
		t.Fatalf("date Retry-After = %d, want 90", got)
	}
}

func TestParseStooqHistoryRows(t *testing.T) {
	body := `<table><tbody><tr><td align=center id=t03>2</td><td nowrap>2 Apr 2026</td><td>573.97</td><td>586.05</td><td>571.92</td><td>584.98</td><td id=c1>+0.11%</td><td id=c1>+0.670</td><td>50,941,709</td></tr><tr><td align=center id=t03>1</td><td nowrap>1 Apr 2026</td><td>581.48</td><td>587.739</td><td>580.42</td><td>584.31</td><td id=c1>+1.24%</td><td id=c1>+7.130</td><td>79,435,132</td></tr></tbody></table>`

	bars, err := parseStooqHistoryRows("QQQ", body)
	if err != nil {
		t.Fatalf("parse rows: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}

	if got := bars[0].Date.Format("2006-01-02"); got != "2026-04-02" {
		t.Fatalf("unexpected first date: %s", got)
	}
	if bars[0].Open != 573.97 || bars[0].High != 586.05 || bars[0].Low != 571.92 || bars[0].Close != 584.98 {
		t.Fatalf("unexpected first OHLC: %+v", bars[0])
	}
	if bars[0].Volume != 50941709 {
		t.Fatalf("unexpected first volume: %.0f", bars[0].Volume)
	}
}

func TestStooqTicker(t *testing.T) {
	tests := map[string]string{
		"QQQ":    "qqq.us",
		"spy":    "spy.us",
		"qqq.us": "qqq.us",
		"^spx":   "^spx",
	}

	for input, want := range tests {
		if got := stooqTicker(input); got != want {
			t.Fatalf("stooqTicker(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBarsFromYahooChartSkipsIncompleteRows(t *testing.T) {
	open1 := 100.0
	high1 := 101.0
	low1 := 99.0
	close1 := 100.5
	vol1 := 1000.0
	open2 := 101.0

	result := yahooChartResult{
		Timestamp: []int64{
			1711929600,
			1712016000,
		},
	}
	result.Indicators.Quote = []struct {
		Open   []*float64 `json:"open"`
		High   []*float64 `json:"high"`
		Low    []*float64 `json:"low"`
		Close  []*float64 `json:"close"`
		Volume []*float64 `json:"volume"`
	}{
		{
			Open:   []*float64{&open1, &open2},
			High:   []*float64{&high1, nil},
			Low:    []*float64{&low1, nil},
			Close:  []*float64{&close1, nil},
			Volume: []*float64{&vol1, nil},
		},
	}

	bars := barsFromYahooChart("SPY", result)
	if len(bars) != 1 {
		t.Fatalf("len(bars) = %d, want 1", len(bars))
	}
	if bars[0].Symbol != "SPY" {
		t.Fatalf("bars[0].Symbol = %s, want SPY", bars[0].Symbol)
	}
	if got := bars[0].Date.Format("2006-01-02"); got != "2024-04-01" {
		t.Fatalf("bars[0].Date = %s, want 2024-04-01", got)
	}
}
