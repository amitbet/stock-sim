package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const StooqDataSource = "stooq"
const YahooDataSource = "yahoo"
const YFinanceDataSource = "yfinance"

var stooqRowPattern = regexp.MustCompile(`<tr><td align=center id=t03>\d+</td><td nowrap>([^<]+)</td><td>([^<]+)</td><td>([^<]+)</td><td>([^<]+)</td><td>([^<]+)</td><td id=c[12]>[^<]+</td><td id=c[12]>[^<]+</td><td>([^<]+)</td></tr>`)

type Bar struct {
	Symbol string    `json:"symbol"`
	Date   time.Time `json:"date"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
	VWAP   float64   `json:"vwap"`
}

type SymbolInfo struct {
	Symbol      string `json:"symbol"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Store struct {
	db     *sql.DB
	stooq  *stooqClient
	yahoo  *yahooClient
	source string
}

type stooqClient struct {
	baseURL    string
	httpClient *http.Client
	symbols    []string

	mu    sync.RWMutex
	cache map[string][]Bar
}

type yahooClient struct {
	baseURL    string
	httpClient *http.Client
	limiter    *time.Ticker
	symbols    []string

	mu        sync.RWMutex
	cache     map[string][]Bar
	infoCache map[string]SymbolInfo
}

type yahooChartResponse struct {
	Chart struct {
		Result []yahooChartResult `json:"result"`
		Error  *yahooChartError   `json:"error"`
	} `json:"chart"`
}

type yahooChartError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type yahooChartResult struct {
	Timestamp  []int64 `json:"timestamp"`
	Indicators struct {
		Quote []struct {
			Open   []*float64 `json:"open"`
			High   []*float64 `json:"high"`
			Low    []*float64 `json:"low"`
			Close  []*float64 `json:"close"`
			Volume []*float64 `json:"volume"`
		} `json:"quote"`
	} `json:"indicators"`
}

type yahooQuoteResponse struct {
	QuoteResponse struct {
		Result []yahooQuoteResult `json:"result"`
	} `json:"quoteResponse"`
}

type yahooQuoteResult struct {
	Symbol    string `json:"symbol"`
	ShortName string `json:"shortName"`
	LongName  string `json:"longName"`
	QuoteType string `json:"quoteType"`
}

func NewStore(path string) (*Store, error) {
	if strings.EqualFold(strings.TrimSpace(path), StooqDataSource) {
		return &Store{
			source: StooqDataSource,
			stooq:  newStooqClient(),
		}, nil
	}
	if isYahooDataSource(path) {
		return &Store{
			source: YahooDataSource,
			yahoo:  newYahooClient(),
		}, nil
	}

	if path == "" {
		return nil, errors.New("sqlite path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("sqlite file not found: %s", path)
		}
		return nil, fmt.Errorf("stat sqlite file %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("sqlite path is a directory, not a file: %s", path)
	}

	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}

	dsnPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(dsnPath, "/") {
		dsnPath = "/" + dsnPath
	}

	dsn := url.URL{
		Scheme:   "file",
		Path:     dsnPath,
		RawQuery: "mode=ro",
	}

	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	return &Store{
		db:     db,
		source: "sqlite",
	}, nil
}

func newStooqClient() *stooqClient {
	return &stooqClient{
		baseURL: "https://stooq.com",
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
		symbols: configuredSymbols(),
		cache:   make(map[string][]Bar),
	}
}

func newYahooClient() *yahooClient {
	client := &yahooClient{
		baseURL: "https://query1.finance.yahoo.com",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		symbols:   configuredSymbols(),
		cache:     make(map[string][]Bar),
		infoCache: make(map[string]SymbolInfo),
	}
	client.setRateLimit(configuredYahooRateLimit())
	return client
}

func (s *Store) ListSymbols(ctx context.Context) ([]string, error) {
	if s.stooq != nil {
		return append([]string(nil), s.stooq.symbols...), nil
	}
	if s.yahoo != nil {
		return append([]string(nil), s.yahoo.symbols...), nil
	}

	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT symbol FROM bars_daily ORDER BY symbol`)
	if err != nil {
		return nil, fmt.Errorf("list symbols: %w", err)
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scan symbol: %w", err)
		}
		symbols = append(symbols, symbol)
	}

	return symbols, rows.Err()
}

func (s *Store) LoadBars(ctx context.Context, symbol string, from, to time.Time) ([]Bar, error) {
	if s.stooq != nil {
		return s.stooq.loadBars(ctx, symbol, from, to)
	}
	if s.yahoo != nil {
		return s.yahoo.loadBars(ctx, symbol, from, to)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT symbol, date, open, high, low, close, volume, COALESCE(vwap, 0)
		FROM bars_daily
		WHERE symbol = ? AND date BETWEEN ? AND ?
		ORDER BY date ASC
	`, strings.ToUpper(symbol), from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("load bars: %w", err)
	}
	defer rows.Close()

	var bars []Bar
	for rows.Next() {
		var bar Bar
		var dateString string
		if err := rows.Scan(&bar.Symbol, &dateString, &bar.Open, &bar.High, &bar.Low, &bar.Close, &bar.Volume, &bar.VWAP); err != nil {
			return nil, fmt.Errorf("scan bar: %w", err)
		}
		bar.Date, err = parseDBDate(dateString)
		if err != nil {
			return nil, err
		}
		bars = append(bars, bar)
	}

	return bars, rows.Err()
}

func parseDBDate(value string) (time.Time, error) {
	layouts := []string{
		"2006-01-02",
		time.RFC3339,
		"2006-01-02 15:04:05-07:00",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse bar date: unsupported date format %q", value)
}

func (s *Store) LoadBarsAround(ctx context.Context, symbol string, center time.Time, before, after int) ([]Bar, error) {
	from := center.AddDate(0, 0, -before*2)
	to := center.AddDate(0, 0, after*2)
	return s.LoadBars(ctx, symbol, from, to)
}

func (s *Store) SymbolInfo(ctx context.Context, symbol string) (SymbolInfo, error) {
	normalized := normalizeSymbol(symbol)
	if s.yahoo != nil {
		return s.yahoo.symbolInfo(ctx, normalized)
	}
	return fallbackSymbolInfo(normalized), nil
}

func configuredSymbols() []string {
	raw := strings.TrimSpace(os.Getenv("SIM_SYMBOLS"))
	if raw == "" {
		return []string{"QQQ", "SPY", "DIA", "IWM", "TQQQ", "SQQQ"}
	}

	seen := make(map[string]struct{})
	var symbols []string
	for _, part := range strings.Split(raw, ",") {
		symbol := strings.ToUpper(strings.TrimSpace(part))
		if symbol == "" {
			continue
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}

	if len(symbols) == 0 {
		return []string{"QQQ"}
	}
	return symbols
}

func configuredYahooRateLimit() int {
	const fallback = 30

	raw := strings.TrimSpace(os.Getenv("YAHOO_RATE_LIMIT_PER_MIN"))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func fallbackSymbolInfo(symbol string) SymbolInfo {
	fallbacks := map[string]SymbolInfo{
		"QQQ":  {Symbol: "QQQ", Name: "QQQ", Description: "Invesco QQQ Trust, Nasdaq-100 ETF."},
		"SPY":  {Symbol: "SPY", Name: "SPY", Description: "SPDR S&P 500 ETF Trust."},
		"DIA":  {Symbol: "DIA", Name: "DIA", Description: "SPDR Dow Jones Industrial Average ETF Trust."},
		"IWM":  {Symbol: "IWM", Name: "IWM", Description: "iShares Russell 2000 ETF."},
		"TQQQ": {Symbol: "TQQQ", Name: "TQQQ", Description: "ProShares UltraPro QQQ leveraged Nasdaq-100 ETF."},
		"SQQQ": {Symbol: "SQQQ", Name: "SQQQ", Description: "ProShares UltraPro Short QQQ inverse Nasdaq-100 ETF."},
		"AAPL": {Symbol: "AAPL", Name: "AAPL", Description: "Apple Inc."},
	}
	if info, ok := fallbacks[symbol]; ok {
		return info
	}
	return SymbolInfo{
		Symbol:      symbol,
		Name:        symbol,
		Description: "Yahoo symbol metadata unavailable.",
	}
}

func isYahooDataSource(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == YahooDataSource || normalized == YFinanceDataSource
}

func (c *stooqClient) loadBars(ctx context.Context, symbol string, from, to time.Time) ([]Bar, error) {
	normalized := normalizeSymbol(symbol)

	c.mu.RLock()
	cached, ok := c.cache[normalized]
	c.mu.RUnlock()
	if !ok {
		var err error
		cached, err = c.fetchAllBars(ctx, normalized)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.cache[normalized] = cached
		c.mu.Unlock()
	}

	from = startOfDayUTC(from)
	to = startOfDayUTC(to)

	filtered := make([]Bar, 0, len(cached))
	for _, bar := range cached {
		if bar.Date.Before(from) || bar.Date.After(to) {
			continue
		}
		filtered = append(filtered, bar)
	}
	return filtered, nil
}

func (c *yahooClient) loadBars(ctx context.Context, symbol string, from, to time.Time) ([]Bar, error) {
	normalized := normalizeSymbol(symbol)
	from = startOfDayUTC(from)
	to = startOfDayUTC(to)

	c.mu.RLock()
	cached, ok := c.cache[normalized]
	c.mu.RUnlock()
	if !ok {
		var err error
		cached, err = c.fetchAllBars(ctx, normalized)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.cache[normalized] = append([]Bar(nil), cached...)
		c.mu.Unlock()
	}

	filtered := make([]Bar, 0, len(cached))
	for _, bar := range cached {
		if bar.Date.Before(from) || bar.Date.After(to) {
			continue
		}
		filtered = append(filtered, bar)
	}
	return filtered, nil
}

func (c *yahooClient) symbolInfo(ctx context.Context, symbol string) (SymbolInfo, error) {
	c.mu.RLock()
	cached, ok := c.infoCache[symbol]
	c.mu.RUnlock()
	if ok {
		return cached, nil
	}

	info, err := c.fetchSymbolInfo(ctx, symbol)
	if err != nil {
		return fallbackSymbolInfo(symbol), nil
	}

	c.mu.Lock()
	c.infoCache[symbol] = info
	c.mu.Unlock()
	return info, nil
}

func (c *stooqClient) fetchAllBars(ctx context.Context, symbol string) ([]Bar, error) {
	const maxPages = 250

	var (
		allBars   []Bar
		oldestKey string
	)

	for page := 1; page <= maxPages; page++ {
		pageBars, err := c.fetchPage(ctx, symbol, page)
		if err != nil {
			return nil, err
		}
		if len(pageBars) == 0 {
			break
		}

		pageOldest := pageBars[len(pageBars)-1].Date.Format("2006-01-02")
		if pageOldest == oldestKey {
			break
		}
		oldestKey = pageOldest
		allBars = append(allBars, pageBars...)
	}

	if len(allBars) == 0 {
		return nil, fmt.Errorf("no Stooq bars found for %s", symbol)
	}

	sort.Slice(allBars, func(i, j int) bool {
		return allBars[i].Date.Before(allBars[j].Date)
	})

	return dedupeBars(allBars), nil
}

func (c *stooqClient) fetchPage(ctx context.Context, symbol string, page int) ([]Bar, error) {
	values := url.Values{
		"s": {stooqTicker(symbol)},
		"i": {"d"},
	}
	if page > 1 {
		values.Set("l", strconv.Itoa(page))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/q/d/?"+values.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Stooq request: %w", err)
	}
	req.Header.Set("User-Agent", "stock-sim/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Stooq page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch Stooq page: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Stooq page: %w", err)
	}

	bars, err := parseStooqHistoryRows(normalizedDisplaySymbol(symbol), string(body))
	if err != nil {
		return nil, err
	}
	return bars, nil
}

func parseStooqHistoryRows(symbol, body string) ([]Bar, error) {
	matches := stooqRowPattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	bars := make([]Bar, 0, len(matches))
	for _, match := range matches {
		dateValue, err := time.Parse("2 Jan 2006", match[1])
		if err != nil {
			return nil, fmt.Errorf("parse Stooq date %q: %w", match[1], err)
		}

		openValue, err := parseStooqNumber(match[2])
		if err != nil {
			return nil, fmt.Errorf("parse Stooq open %q: %w", match[2], err)
		}
		highValue, err := parseStooqNumber(match[3])
		if err != nil {
			return nil, fmt.Errorf("parse Stooq high %q: %w", match[3], err)
		}
		lowValue, err := parseStooqNumber(match[4])
		if err != nil {
			return nil, fmt.Errorf("parse Stooq low %q: %w", match[4], err)
		}
		closeValue, err := parseStooqNumber(match[5])
		if err != nil {
			return nil, fmt.Errorf("parse Stooq close %q: %w", match[5], err)
		}
		volumeValue, err := parseStooqNumber(match[6])
		if err != nil {
			return nil, fmt.Errorf("parse Stooq volume %q: %w", match[6], err)
		}

		bars = append(bars, Bar{
			Symbol: symbol,
			Date:   startOfDayUTC(dateValue),
			Open:   openValue,
			High:   highValue,
			Low:    lowValue,
			Close:  closeValue,
			Volume: volumeValue,
		})
	}

	return bars, nil
}

func (c *yahooClient) fetchBars(ctx context.Context, symbol string, from, to time.Time) ([]Bar, error) {
	if err := c.waitRateLimit(ctx); err != nil {
		return nil, err
	}

	periodStart := from.Unix()
	periodEnd := to.Add(24 * time.Hour).Unix()

	endpoint := fmt.Sprintf("%s/v8/finance/chart/%s", c.baseURL, url.PathEscape(symbol))
	params := url.Values{}
	params.Set("period1", strconv.FormatInt(periodStart, 10))
	params.Set("period2", strconv.FormatInt(periodEnd, 10))
	params.Set("interval", "1d")
	params.Set("includeAdjustedClose", "true")
	params.Set("events", "div,splits")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Yahoo request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("yahoo api error: status %d", resp.StatusCode)
	}

	var payload yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode yahoo chart response: %w", err)
	}
	if payload.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo chart error: %s", payload.Chart.Error.Description)
	}
	if len(payload.Chart.Result) == 0 {
		return nil, nil
	}

	return barsFromYahooChart(symbol, payload.Chart.Result[0]), nil
}

func (c *yahooClient) fetchAllBars(ctx context.Context, symbol string) ([]Bar, error) {
	from := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	return c.fetchBars(ctx, symbol, from, to)
}

func (c *yahooClient) fetchSymbolInfo(ctx context.Context, symbol string) (SymbolInfo, error) {
	if err := c.waitRateLimit(ctx); err != nil {
		return SymbolInfo{}, err
	}

	endpoint := fmt.Sprintf("%s/v7/finance/quote", c.baseURL)
	params := url.Values{}
	params.Set("symbols", symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return SymbolInfo{}, fmt.Errorf("build Yahoo quote request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return SymbolInfo{}, fmt.Errorf("yahoo quote request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return SymbolInfo{}, fmt.Errorf("yahoo quote api error: status %d", resp.StatusCode)
	}

	var payload yahooQuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return SymbolInfo{}, fmt.Errorf("decode yahoo quote response: %w", err)
	}
	if len(payload.QuoteResponse.Result) == 0 {
		return SymbolInfo{}, fmt.Errorf("empty yahoo quote response")
	}

	quote := payload.QuoteResponse.Result[0]
	name := strings.TrimSpace(quote.LongName)
	if name == "" {
		name = strings.TrimSpace(quote.ShortName)
	}
	if name == "" {
		name = symbol
	}

	description := name
	if description == symbol && quote.QuoteType != "" {
		description = fmt.Sprintf("%s %s", symbol, strings.ToLower(quote.QuoteType))
	}

	return SymbolInfo{
		Symbol:      symbol,
		Name:        name,
		Description: description,
	}, nil
}

func barsFromYahooChart(symbol string, result yahooChartResult) []Bar {
	if len(result.Indicators.Quote) == 0 {
		return nil
	}

	quote := result.Indicators.Quote[0]
	bars := make([]Bar, 0, len(result.Timestamp))
	for i := 0; i < len(result.Timestamp); i++ {
		if i >= len(quote.Open) || i >= len(quote.High) || i >= len(quote.Low) || i >= len(quote.Close) || i >= len(quote.Volume) {
			continue
		}
		if quote.Open[i] == nil || quote.High[i] == nil || quote.Low[i] == nil || quote.Close[i] == nil || quote.Volume[i] == nil {
			continue
		}

		bars = append(bars, Bar{
			Symbol: symbol,
			Date:   startOfDayUTC(time.Unix(result.Timestamp[i], 0).UTC()),
			Open:   *quote.Open[i],
			High:   *quote.High[i],
			Low:    *quote.Low[i],
			Close:  *quote.Close[i],
			Volume: *quote.Volume[i],
		})
	}

	return bars
}

func (c *yahooClient) waitRateLimit(ctx context.Context) error {
	if c.limiter == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.limiter.C:
		return nil
	}
}

func (c *yahooClient) setRateLimit(rate int) {
	if c.limiter != nil {
		c.limiter.Stop()
		c.limiter = nil
	}
	if rate <= 0 {
		return
	}
	interval := time.Minute / time.Duration(rate)
	if interval <= 0 {
		interval = time.Minute
	}
	c.limiter = time.NewTicker(interval)
}

func parseStooqNumber(value string) (float64, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	return strconv.ParseFloat(cleaned, 64)
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}

func normalizedDisplaySymbol(symbol string) string {
	return normalizeSymbol(symbol)
}

func stooqTicker(symbol string) string {
	symbol = strings.ToLower(strings.TrimSpace(symbol))
	if symbol == "" {
		return symbol
	}
	if strings.Contains(symbol, ".") || strings.HasPrefix(symbol, "^") {
		return symbol
	}
	return symbol + ".us"
}

func startOfDayUTC(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func dedupeBars(bars []Bar) []Bar {
	if len(bars) == 0 {
		return nil
	}

	result := make([]Bar, 0, len(bars))
	lastDate := ""
	for _, bar := range bars {
		currentDate := bar.Date.Format("2006-01-02")
		if currentDate == lastDate {
			result[len(result)-1] = bar
			continue
		}
		result = append(result, bar)
		lastDate = currentDate
	}
	return result
}
