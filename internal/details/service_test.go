package details

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestParseCSVIncludesCriteriaMetadata(t *testing.T) {
	service := &Service{}

	result, err := service.ParseCSV("Symbol,Criteria\nSPY,Breakout\nAAPL,Base\n")
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}

	if len(result.Tickers) != 2 || result.Tickers[0] != "SPY" || result.Tickers[1] != "AAPL" {
		t.Fatalf("unexpected tickers: %#v", result.Tickers)
	}
	if got := result.CriteriaByTicker["SPY"]; got != "Breakout" {
		t.Fatalf("CriteriaByTicker[SPY] = %q, want Breakout", got)
	}
	if got := result.CriteriaByTicker["AAPL"]; got != "Base" {
		t.Fatalf("CriteriaByTicker[AAPL] = %q, want Base", got)
	}
}

func TestNormalizeSCTRRecordsSetsTypeFromView(t *testing.T) {
	records := normalizeSCTRRecords([]sctrRawItem{{Symbol: "SPY", Name: "SPDR S&P 500 ETF Trust"}}, "ETF")
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].Type != "ETF" {
		t.Fatalf("records[0].Type = %q, want ETF", records[0].Type)
	}

	stockRecords := normalizeSCTRRecords([]sctrRawItem{{Symbol: "AAPL", Name: "Apple Inc."}}, "Large")
	if len(stockRecords) != 1 {
		t.Fatalf("len(stockRecords) = %d, want 1", len(stockRecords))
	}
	if stockRecords[0].Type != "Large" {
		t.Fatalf("stockRecords[0].Type = %q, want Large", stockRecords[0].Type)
	}
}

func TestSCTRViewsIncludeETFView(t *testing.T) {
	for _, view := range sctrViews {
		if view.view == "E" && view.recordType == "ETF" && view.url == "https://stockcharts.com/j-sum/sum?cmd=sctr&view=E" {
			return
		}
	}
	t.Fatalf("sctrViews does not include the StockCharts ETF view")
}

func TestSCTRViewsMapStockViewsToDisplayTypes(t *testing.T) {
	want := map[string]string{
		"L": "Large",
		"M": "Med",
		"S": "Small",
	}
	for _, view := range sctrViews {
		if expected := want[view.view]; expected != "" && view.recordType != expected {
			t.Fatalf("view %s recordType = %q, want %q", view.view, view.recordType, expected)
		}
	}
}

func TestMergeSCTRRecordLetsETFTypeWin(t *testing.T) {
	merged := mergeSCTRRecord(
		Record{Symbol: "SPY", Name: "SPDR S&P 500 ETF Trust", Type: "Large"},
		Record{Symbol: "SPY", Type: "ETF"},
	)
	if merged.Type != "ETF" {
		t.Fatalf("merged.Type = %q, want ETF", merged.Type)
	}
}

func TestEnrichRecordsWithClassificationDoesNotPreserveStockChartsValuesForSelectedSource(t *testing.T) {
	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("network unavailable")
			}),
		},
		finvizCache: make(map[string]classificationCacheEntry),
		yahooCache:  make(map[string]classificationCacheEntry),
	}

	records, err := service.enrichRecordsWithClassification(context.Background(), []Record{{
		Symbol:   "AAPL",
		Industry: "Computer Hardware",
		Sector:   "Technology",
	}}, "yahoo")
	if err != nil {
		t.Fatalf("enrichRecordsWithClassification returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].Industry != "" || records[0].Sector != "" {
		t.Fatalf("Yahoo enrichment preserved StockCharts values: industry=%q sector=%q", records[0].Industry, records[0].Sector)
	}
	if records[0].IndustrySource != "Yahoo" || records[0].SectorSource != "Yahoo" {
		t.Fatalf("sources = %q/%q, want Yahoo/Yahoo", records[0].IndustrySource, records[0].SectorSource)
	}
}

func TestFetchFinvizClassificationParsesCurrentQuoteLinksMarkup(t *testing.T) {
	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				body := `<div class="quote-links whitespace-nowrap gap-8">
					<div class="flex space-x-0.5 overflow-hidden">
						<a href="screener.ashx?v=111&f=sec_technology" class="tab-link">Technology</a>
						<span class="text-muted-3">•</span>
						<a href="screener.ashx?v=111&f=ind_consumerelectronics" class="tab-link truncate" title="Consumer Electronics">Consumer Electronics</a>
						<span class="text-muted-3">•</span>
						<a href="screener.ashx?v=111&f=geo_usa" class="tab-link">USA</a>
					</div>
				</div>`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
		finvizCache: make(map[string]classificationCacheEntry),
		yahooCache:  make(map[string]classificationCacheEntry),
	}

	classification, err := service.fetchFinvizClassification(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("fetchFinvizClassification returned error: %v", err)
	}
	if classification == nil {
		t.Fatal("classification is nil")
	}
	if classification.Sector != "Technology" || classification.Industry != "Consumer Electronics" {
		t.Fatalf("classification = %#v, want Technology / Consumer Electronics", classification)
	}
	if classification.Source != "Finviz" {
		t.Fatalf("classification.Source = %q, want Finviz", classification.Source)
	}
}

func TestFetchYahooClassificationUsesSearchIndustryFields(t *testing.T) {
	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/v1/finance/search" {
					t.Fatalf("requested path = %q, want /v1/finance/search", req.URL.Path)
				}
				if got := req.URL.Query().Get("q"); got != "AAPL" {
					t.Fatalf("query q = %q, want AAPL", got)
				}
				body := `{
					"quotes": [
						{
							"symbol": "AAPL",
							"sector": "Technology",
							"sectorDisp": "Technology",
							"industry": "Consumer Electronics",
							"industryDisp": "Consumer Electronics"
						}
					]
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
		finvizCache: make(map[string]classificationCacheEntry),
		yahooCache:  make(map[string]classificationCacheEntry),
	}

	classification, err := service.fetchYahooClassification(context.Background(), "aapl")
	if err != nil {
		t.Fatalf("fetchYahooClassification returned error: %v", err)
	}
	if classification == nil {
		t.Fatal("classification is nil")
	}
	if classification.Sector != "Technology" || classification.Industry != "Consumer Electronics" {
		t.Fatalf("classification = %#v, want Technology / Consumer Electronics", classification)
	}
	if classification.Source != "Yahoo" {
		t.Fatalf("classification.Source = %q, want Yahoo", classification.Source)
	}
}

func TestCachedDetailRecordIgnoresBlankExternalClassification(t *testing.T) {
	service := &Service{
		detailRecordCache: map[string]detailRecordCacheEntry{
			detailRecordCacheKey("AAPL", "yahoo"): {
				value:     Record{Symbol: "AAPL", IndustrySource: "Yahoo", SectorSource: "Yahoo"},
				expiresAt: time.Now().Add(time.Hour),
			},
			detailRecordCacheKey("MSFT", "yahoo"): {
				value:     Record{Symbol: "MSFT", Industry: "Software - Infrastructure", Sector: "Technology"},
				expiresAt: time.Now().Add(time.Hour),
			},
			detailRecordCacheKey("SPY", "stockcharts"): {
				value:     Record{Symbol: "SPY"},
				expiresAt: time.Now().Add(time.Hour),
			},
		},
	}

	if _, ok := service.cachedDetailRecord("AAPL", "yahoo"); ok {
		t.Fatal("blank Yahoo detail record was treated as cached")
	}
	if record, ok := service.cachedDetailRecord("MSFT", "yahoo"); !ok || record.Industry == "" || record.Sector == "" {
		t.Fatalf("nonblank Yahoo detail record was not returned: %#v ok=%v", record, ok)
	}
	if _, ok := service.cachedDetailRecord("SPY", "stockcharts"); !ok {
		t.Fatal("blank StockCharts detail record should remain cacheable")
	}
}
