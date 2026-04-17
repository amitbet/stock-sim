package details

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"stock-sim/internal/data"
)

const (
	defaultSCTRURL             = "https://stockcharts.com/j-sum/sum?cmd=sctr&view=L&timeframe=I"
	detailsRecordCacheTTL      = 6 * time.Hour
	sctrSnapshotCacheTTL       = 6 * time.Hour
	classificationCacheTTL     = 180 * 24 * time.Hour
	industryMA50CacheTTL       = 24 * time.Hour
	historicalPricesCacheTTL   = 24 * time.Hour
	earningsDateCacheTTL       = 7 * 24 * time.Hour
	defaultHistoricalLookback  = 180
	maxIndustryStocksForMA50   = 20
	defaultRequestTimeout      = 20 * time.Second
	defaultClassificationDelay = 150 * time.Millisecond
	// Keep these modest: Yahoo history requests already pass through a global
	// rate limiter in data.Store, and Finviz classification remains delayed and
	// sequential. These bounds reduce wall-clock time without creating a bursty
	// request pattern.
	maxConcurrentIndustryMA50 = 4
	maxConcurrentStockHistory = 4
)

type Service struct {
	httpClient *http.Client
	yahooStore *data.Store

	mu                  sync.RWMutex
	sctrSnapshotCache   sctrSnapshotCacheEntry
	detailRecordCache   map[string]detailRecordCacheEntry
	finvizCache         map[string]classificationCacheEntry
	yahooCache          map[string]classificationCacheEntry
	earningsDateCache   map[string]earningsDateCacheEntry
	earningsCalendar    map[string]earningsCalendarCacheEntry
	historicalPriceData map[string]historicalPriceCacheEntry
	industryMA50        map[string]industryMA50CacheEntry
}

type classificationCacheEntry struct {
	value     *Classification
	expiresAt time.Time
}

type detailRecordCacheEntry struct {
	value     Record
	expiresAt time.Time
}

type sctrSnapshotCacheEntry struct {
	value     []Record
	expiresAt time.Time
}

type historicalPriceCacheEntry struct {
	value     []pricePoint
	expiresAt time.Time
}

type earningsDateCacheEntry struct {
	value     *string
	expiresAt time.Time
}

type earningsCalendarCacheEntry struct {
	value     map[string]string
	expiresAt time.Time
}

type industryMA50CacheEntry struct {
	value     *IndustryMA50
	expiresAt time.Time
}

type ParseCSVResult struct {
	Columns           []string `json:"columns"`
	TickerColumnIndex int      `json:"tickerColumnIndex"`
	TickerColumnName  string   `json:"tickerColumnName"`
	Tickers           []string `json:"tickers"`
}

type FetchRequest struct {
	Tickers        []string `json:"tickers"`
	IndustrySource string   `json:"industrySource"`
}

type FetchResponse struct {
	Records        []Record    `json:"records"`
	Stats          StatsResult `json:"stats"`
	MissingTickers []string    `json:"missingTickers"`
}

type Record struct {
	Date                   string   `json:"date"`
	Symbol                 string   `json:"symbol"`
	Name                   string   `json:"name"`
	SCTR                   *float64 `json:"SCTR"`
	Delta                  *float64 `json:"delta"`
	Close                  *float64 `json:"close"`
	MarketCap              *float64 `json:"marketCap"`
	Vol                    *int64   `json:"vol"`
	Industry               string   `json:"industry"`
	Sector                 string   `json:"sector"`
	IndustrySource         string   `json:"industrySource,omitempty"`
	SectorSource           string   `json:"sectorSource,omitempty"`
	EarningsReportDate     string   `json:"earningsReportDate,omitempty"`
	IndustryRS             *float64 `json:"industryRS,omitempty"`
	SectorRS               *float64 `json:"sectorRS,omitempty"`
	IndustryAboveMA50      *bool    `json:"industryAboveMA50,omitempty"`
	IndustryPercentAbove50 *float64 `json:"industryPercentAboveMA50,omitempty"`
}

type StatsResult struct {
	Industries   map[string]AggregateStat `json:"industries"`
	Sectors      map[string]AggregateStat `json:"sectors"`
	IndustryMA50 map[string]IndustryMA50  `json:"industryMA50"`
}

type AggregateStat struct {
	Avg    float64 `json:"avg"`
	Count  int     `json:"count"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Median float64 `json:"median"`
}

type IndustryMA50 struct {
	CurrentIndex       float64 `json:"currentIndex"`
	MA50               float64 `json:"ma50"`
	AboveMA            bool    `json:"aboveMA"`
	PercentAboveMA50   float64 `json:"percentAboveMA50"`
	Source             string  `json:"source"`
	StocksUsed         *int    `json:"stocksUsed,omitempty"`
	TotalStocks        *int    `json:"totalStocks,omitempty"`
	ETF                string  `json:"etf,omitempty"`
	SectorFallbackUsed *string `json:"sectorFallbackUsed,omitempty"`
}

type Classification struct {
	Industry string `json:"industry"`
	Sector   string `json:"sector"`
	Source   string `json:"source"`
}

type pricePoint struct {
	Date  string
	Close float64
}

type sctrRawItem struct {
	Date      any `json:"date"`
	Symbol    any `json:"symbol"`
	Name      any `json:"name"`
	SCTR      any `json:"SCTR"`
	Delta     any `json:"delta"`
	Close     any `json:"close"`
	MarketCap any `json:"marketCap"`
	Vol       any `json:"vol"`
	Industry  any `json:"industry"`
	Sector    any `json:"sector"`
}

var (
	finvizSectorPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)Sector</td>\s*<td[^>]*>([^<]+)</td>`),
		regexp.MustCompile(`(?i)Sector[:\s]*</td>\s*<td[^>]*>([^<]+)</td>`),
		regexp.MustCompile(`(?i)snapshot-td2[^>]*>Sector</td>\s*<td[^>]*>([^<]+)</td>`),
		regexp.MustCompile(`(?i)Sector[^<]*</td>\s*<td[^>]*class="snapshot-td2"[^>]*>([^<]+)</td>`),
	}
	finvizIndustryPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)Industry</td>\s*<td[^>]*>([^<]+)</td>`),
		regexp.MustCompile(`(?i)Industry[:\s]*</td>\s*<td[^>]*>([^<]+)</td>`),
		regexp.MustCompile(`(?i)snapshot-td2[^>]*>Industry</td>\s*<td[^>]*>([^<]+)</td>`),
		regexp.MustCompile(`(?i)Industry[^<]*</td>\s*<td[^>]*class="snapshot-td2"[^>]*>([^<]+)</td>`),
	}
	finvizTooltipPattern = regexp.MustCompile(`(?i)<b>[^<]+</b>([^<•]+)<span[^>]*>•</span>`)
)

func NewService(yahooStore *data.Store) *Service {
	return &Service{
		httpClient:          &http.Client{Timeout: defaultRequestTimeout},
		yahooStore:          yahooStore,
		detailRecordCache:   make(map[string]detailRecordCacheEntry),
		finvizCache:         make(map[string]classificationCacheEntry),
		yahooCache:          make(map[string]classificationCacheEntry),
		earningsDateCache:   make(map[string]earningsDateCacheEntry),
		earningsCalendar:    make(map[string]earningsCalendarCacheEntry),
		historicalPriceData: make(map[string]historicalPriceCacheEntry),
		industryMA50:        make(map[string]industryMA50CacheEntry),
	}
}

func (s *Service) ParseCSV(text string) (*ParseCSVResult, error) {
	delimiter := detectDelimiter(text)
	rows, err := parseCSVRows(text, delimiter)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &ParseCSVResult{
			Columns:           []string{},
			TickerColumnIndex: 0,
			TickerColumnName:  "",
			Tickers:           []string{},
		}, nil
	}

	columns, records := csvRowsToRecords(rows)
	if len(columns) == 0 {
		return nil, fmt.Errorf("unable to detect CSV columns")
	}

	columnIndex := detectTickerColumnIndex(records, columns)
	columnName := columns[columnIndex]
	tickers := extractTickersFromRecords(records, columnName)

	return &ParseCSVResult{
		Columns:           columns,
		TickerColumnIndex: columnIndex,
		TickerColumnName:  columnName,
		Tickers:           tickers,
	}, nil
}

func (s *Service) FetchSCTRForTickers(ctx context.Context, tickers []string, industrySource string) (*FetchResponse, error) {
	normalized := normalizeTickers(tickers)
	normalizedSource := strings.ToLower(strings.TrimSpace(industrySource))
	if normalizedSource == "" {
		normalizedSource = "finviz"
	}
	if len(normalized) == 0 {
		return &FetchResponse{
			Records:        []Record{},
			Stats:          StatsResult{Industries: map[string]AggregateStat{}, Sectors: map[string]AggregateStat{}, IndustryMA50: map[string]IndustryMA50{}},
			MissingTickers: []string{},
		}, nil
	}

	allRecords, err := s.fetchSCTRJSON(ctx)
	if err != nil {
		return nil, err
	}

	recordMap := make(map[string]Record, len(allRecords))
	for _, record := range allRecords {
		recordMap[strings.ToUpper(record.Symbol)] = record
	}

	records := make([]Record, 0, len(normalized))
	cachedRecords := make([]Record, 0, len(normalized))
	recordsToEnrich := make([]Record, 0, len(normalized))
	missing := make([]string, 0)
	for _, ticker := range normalized {
		record, ok := recordMap[ticker]
		if !ok {
			missing = append(missing, ticker)
			continue
		}
		records = append(records, record)
		if cached, ok := s.cachedDetailRecord(ticker, normalizedSource); ok {
			cachedRecords = append(cachedRecords, cached)
			continue
		}
		recordsToEnrich = append(recordsToEnrich, record)
	}

	classifiedFresh := make([]Record, 0, len(recordsToEnrich))
	if len(recordsToEnrich) > 0 {
		classifiedFresh, err = s.enrichRecordsWithClassification(ctx, recordsToEnrich, normalizedSource)
		if err != nil {
			return nil, err
		}
		s.enrichRecordsWithEarningsDates(ctx, classifiedFresh)
	}

	classified := append(cachedRecords, classifiedFresh...)

	stats := calculateStats(allRecords)
	industryMap := buildIndustryLookupMap(allRecords, classified)
	industryMA50 := s.calculateIndustryMA50Map(ctx, classified)

	for idx := range classified {
		rsIndustry, rsSector := calculateRelativeStrength(classified[idx], stats, industryMap)
		classified[idx].IndustryRS = rsIndustry
		classified[idx].SectorRS = rsSector
		if ma50, ok := industryMA50[strings.TrimSpace(classified[idx].Industry)]; ok {
			classified[idx].IndustryAboveMA50 = boolPtr(ma50.AboveMA)
			classified[idx].IndustryPercentAbove50 = floatPtr(ma50.PercentAboveMA50)
		}
		s.storeDetailRecordCache(classified[idx], normalizedSource)
	}

	if len(missing) > 0 {
		fallbackRecords := s.fetchFallbackRecords(ctx, missing, normalizedSource)
		for _, record := range fallbackRecords {
			classified = append(classified, record)
			s.storeDetailRecordCache(record, normalizedSource)
		}
	}

	sort.SliceStable(classified, func(i, j int) bool {
		si := recordScore(classified[i].SCTR)
		sj := recordScore(classified[j].SCTR)
		if si == sj {
			return classified[i].Symbol < classified[j].Symbol
		}
		return si > sj
	})

	return &FetchResponse{
		Records: classified,
		Stats: StatsResult{
			Industries:   stats.Industries,
			Sectors:      stats.Sectors,
			IndustryMA50: industryMA50,
		},
		MissingTickers: missing,
	}, nil
}

func (s *Service) fetchFallbackRecords(ctx context.Context, tickers []string, source string) []Record {
	if len(tickers) == 0 {
		return nil
	}

	type result struct {
		record Record
	}

	results := make(chan result, len(tickers))
	sem := make(chan struct{}, maxConcurrentStockHistory)
	var wg sync.WaitGroup

	for _, ticker := range tickers {
		symbol := strings.ToUpper(strings.TrimSpace(ticker))
		if symbol == "" {
			continue
		}
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			record := Record{Symbol: symbol}

			if s.yahooStore != nil {
				if info, err := s.yahooStore.SymbolInfo(ctx, symbol); err == nil {
					if name := strings.TrimSpace(info.Name); name != "" && name != symbol {
						record.Name = name
					}
				}

				now := time.Now()
				from := now.AddDate(0, 0, -30)
				if bars, err := s.yahooStore.LoadBars(ctx, symbol, from, now); err == nil && len(bars) > 0 {
					latest := bars[len(bars)-1]
					record.Date = latest.Date.Format("2006-01-02")
					record.Close = floatPtr(latest.Close)
					record.Vol = int64Ptr(int64(latest.Volume))
				}
			}

			classificationSource := source
			if classificationSource == "stockcharts" {
				classificationSource = "yahoo"
			}
			enriched, err := s.enrichRecordsWithClassification(ctx, []Record{record}, classificationSource)
			if err == nil && len(enriched) > 0 {
				record = enriched[0]
			}

			enrichedRecord := []Record{record}
			s.enrichRecordsWithEarningsDates(ctx, enrichedRecord)
			results <- result{record: enrichedRecord[0]}
		}(symbol)
	}

	wg.Wait()
	close(results)

	out := make([]Record, 0, len(tickers))
	for item := range results {
		out = append(out, item.record)
	}
	return out
}

func (s *Service) calculateIndustryMA50Map(ctx context.Context, records []Record) map[string]IndustryMA50 {
	industryMA50 := make(map[string]IndustryMA50)
	seenIndustries := make(map[string]Record)
	for _, record := range records {
		industry := strings.TrimSpace(record.Industry)
		if industry == "" {
			continue
		}
		if _, ok := seenIndustries[industry]; ok {
			continue
		}
		seenIndustries[industry] = record
	}
	if len(seenIndustries) == 0 {
		return industryMA50
	}

	type result struct {
		industry string
		value    *IndustryMA50
	}

	sem := make(chan struct{}, maxConcurrentIndustryMA50)
	results := make(chan result, len(seenIndustries))
	var wg sync.WaitGroup

	for industry, record := range seenIndustries {
		wg.Add(1)
		go func(industry string, record Record) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			ma50, err := s.calculateIndustryMA50(ctx, record, records)
			if err == nil && ma50 != nil {
				results <- result{industry: industry, value: ma50}
			}
		}(industry, record)
	}

	wg.Wait()
	close(results)

	for item := range results {
		industryMA50[item.industry] = *item.value
	}

	return industryMA50
}

func (s *Service) enrichRecordsWithEarningsDates(ctx context.Context, records []Record) {
	symbols := make([]string, 0, len(records))
	for _, record := range records {
		if symbol := strings.ToUpper(strings.TrimSpace(record.Symbol)); symbol != "" {
			symbols = append(symbols, symbol)
		}
	}

	upcomingDates, err := s.fetchUpcomingEarningsDates(ctx, symbols)
	if err != nil {
		return
	}

	for idx := range records {
		records[idx].EarningsReportDate = upcomingDates[strings.ToUpper(records[idx].Symbol)]
	}
}

func (s *Service) fetchSCTRJSON(ctx context.Context) ([]Record, error) {
	now := time.Now()
	s.mu.RLock()
	cachedSnapshot := s.sctrSnapshotCache
	s.mu.RUnlock()
	if cachedSnapshot.value != nil && now.Before(cachedSnapshot.expiresAt) {
		return cloneRecords(cachedSnapshot.value), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultSCTRURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; stock-sim/1.0)")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://stockcharts.com/freecharts/sctr.html")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch stockcharts sctr: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch stockcharts sctr: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read stockcharts sctr response: %w", err)
	}

	var raw []sctrRawItem
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode stockcharts sctr: %w", err)
	}

	records := normalizeSCTRRecords(raw)
	s.mu.Lock()
	s.sctrSnapshotCache = sctrSnapshotCacheEntry{
		value:     cloneRecords(records),
		expiresAt: now.Add(sctrSnapshotCacheTTL),
	}
	s.mu.Unlock()
	return records, nil
}

func normalizeSCTRRecords(raw []sctrRawItem) []Record {
	records := make([]Record, 0, len(raw))
	asOfDate := ""
	for _, item := range raw {
		itemDate := toString(item.Date)
		if parsed := parseFlexibleDate(itemDate); parsed != "" {
			asOfDate = parsed
		}
		symbol := strings.ToUpper(strings.TrimSpace(toString(item.Symbol)))
		if symbol == "" {
			continue
		}
		dateValue := parseFlexibleDate(itemDate)
		if dateValue == "" {
			dateValue = asOfDate
		}
		records = append(records, Record{
			Date:      dateValue,
			Symbol:    symbol,
			Name:      toString(item.Name),
			SCTR:      toFloatPtr(item.SCTR),
			Delta:     toFloatPtr(item.Delta),
			Close:     toFloatPtr(item.Close),
			MarketCap: toFloatPtr(item.MarketCap),
			Vol:       toInt64Ptr(item.Vol),
			Industry:  toString(item.Industry),
			Sector:    toString(item.Sector),
		})
	}
	return records
}

func (s *Service) enrichRecordsWithClassification(ctx context.Context, records []Record, source string) ([]Record, error) {
	normalizedSource := strings.ToLower(strings.TrimSpace(source))
	if normalizedSource == "" {
		normalizedSource = "finviz"
	}
	out := make([]Record, len(records))
	copy(out, records)

	if normalizedSource == "stockcharts" {
		for idx := range out {
			out[idx].IndustrySource = "StockCharts"
			out[idx].SectorSource = "StockCharts"
		}
		return out, nil
	}

	for idx := range out {
		var (
			classification *Classification
			err            error
		)
		switch normalizedSource {
		case "yahoo":
			classification, err = s.fetchYahooClassification(ctx, out[idx].Symbol)
		default:
			classification, err = s.fetchFinvizClassification(ctx, out[idx].Symbol)
		}
		if err != nil {
			continue
		}
		if classification != nil {
			if classification.Industry != "" {
				out[idx].Industry = classification.Industry
				out[idx].IndustrySource = classification.Source
			}
			if classification.Sector != "" {
				out[idx].Sector = classification.Sector
				out[idx].SectorSource = classification.Source
			}
		}
		if out[idx].IndustrySource == "" {
			out[idx].IndustrySource = "StockCharts"
		}
		if out[idx].SectorSource == "" {
			out[idx].SectorSource = "StockCharts"
		}
		if idx < len(out)-1 && normalizedSource == "finviz" {
			time.Sleep(defaultClassificationDelay)
		}
	}

	return out, nil
}

func (s *Service) fetchYahooClassification(ctx context.Context, ticker string) (*Classification, error) {
	return s.fetchClassificationWithCache("yahoo:"+ticker, classificationCacheTTL, s.yahooCache, func() (*Classification, error) {
		endpoint := fmt.Sprintf("https://query1.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=assetProfile", url.PathEscape(ticker))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; stock-sim/1.0)")
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("yahoo classification: status %d", resp.StatusCode)
		}
		var payload struct {
			QuoteSummary struct {
				Result []struct {
					AssetProfile struct {
						Industry string `json:"industry"`
						Sector   string `json:"sector"`
					} `json:"assetProfile"`
				} `json:"result"`
			} `json:"quoteSummary"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if len(payload.QuoteSummary.Result) == 0 {
			return nil, nil
		}
		profile := payload.QuoteSummary.Result[0].AssetProfile
		if strings.TrimSpace(profile.Industry) == "" && strings.TrimSpace(profile.Sector) == "" {
			return nil, nil
		}
		return &Classification{
			Industry: strings.TrimSpace(profile.Industry),
			Sector:   strings.TrimSpace(profile.Sector),
			Source:   "Yahoo",
		}, nil
	})
}

func (s *Service) fetchFinvizClassification(ctx context.Context, ticker string) (*Classification, error) {
	return s.fetchClassificationWithCache("finviz:"+ticker, classificationCacheTTL, s.finvizCache, func() (*Classification, error) {
		endpoint := fmt.Sprintf("https://finviz.com/quote.ashx?t=%s", url.QueryEscape(ticker))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("finviz classification: status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		html := string(body)
		sector := findFirstMatch(finvizSectorPatterns, html)
		industry := findFirstMatch(finvizIndustryPatterns, html)
		if industry == "" {
			if matches := finvizTooltipPattern.FindStringSubmatch(html); len(matches) > 1 {
				industry = strings.TrimSpace(matches[1])
			}
		}
		industry = cleanHTMLValue(industry)
		sector = cleanHTMLValue(sector)
		if industry == "" && sector == "" {
			return nil, nil
		}
		return &Classification{
			Industry: industry,
			Sector:   sector,
			Source:   "Finviz",
		}, nil
	})
}

func (s *Service) fetchUpcomingEarningsDates(ctx context.Context, symbols []string) (map[string]string, error) {
	now := time.Now().UTC()
	targets := make(map[string]struct{}, len(symbols))
	results := make(map[string]string, len(symbols))
	for _, symbol := range symbols {
		normalized := strings.ToUpper(strings.TrimSpace(symbol))
		if normalized != "" {
			targets[normalized] = struct{}{}
		}
	}

	s.mu.RLock()
	for symbol := range targets {
		entry, ok := s.earningsDateCache[symbol]
		if ok && now.Before(entry.expiresAt) && entry.value != nil {
			results[symbol] = *entry.value
		}
	}
	s.mu.RUnlock()

	if len(results) == len(targets) {
		return results, nil
	}

	const lookaheadDays = 120
	for offset := 0; offset < lookaheadDays && len(results) < len(targets); offset++ {
		dateValue := now.AddDate(0, 0, offset).Format("2006-01-02")
		calendar, err := s.fetchNasdaqEarningsCalendar(ctx, dateValue)
		if err != nil {
			continue
		}
		for symbol := range targets {
			if _, ok := results[symbol]; ok {
				continue
			}
			if upcomingDate, ok := calendar[symbol]; ok {
				results[symbol] = upcomingDate
				copyValue := upcomingDate
				s.mu.Lock()
				s.earningsDateCache[symbol] = earningsDateCacheEntry{
					value:     &copyValue,
					expiresAt: now.Add(earningsDateCacheTTL),
				}
				s.mu.Unlock()
			}
		}
	}

	s.mu.Lock()
	for symbol := range targets {
		if _, ok := results[symbol]; ok {
			continue
		}
		s.earningsDateCache[symbol] = earningsDateCacheEntry{
			value:     nil,
			expiresAt: now.Add(earningsDateCacheTTL),
		}
	}
	s.mu.Unlock()

	return results, nil
}

func (s *Service) fetchNasdaqEarningsCalendar(ctx context.Context, dateValue string) (map[string]string, error) {
	now := time.Now()
	s.mu.RLock()
	entry, ok := s.earningsCalendar[dateValue]
	s.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.value, nil
	}

	endpoint := fmt.Sprintf("https://api.nasdaq.com/api/calendar/earnings?date=%s", url.QueryEscape(dateValue))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nasdaq earnings calendar: status %d", resp.StatusCode)
	}

	var payload struct {
		Data struct {
			Rows []struct {
				Symbol string `json:"symbol"`
			} `json:"rows"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	calendar := make(map[string]string, len(payload.Data.Rows))
	for _, row := range payload.Data.Rows {
		symbol := strings.ToUpper(strings.TrimSpace(row.Symbol))
		if symbol != "" {
			calendar[symbol] = dateValue
		}
	}

	s.mu.Lock()
	s.earningsCalendar[dateValue] = earningsCalendarCacheEntry{
		value:     calendar,
		expiresAt: now.Add(earningsDateCacheTTL),
	}
	s.mu.Unlock()

	return calendar, nil
}

func (s *Service) cachedDetailRecord(symbol, source string) (Record, bool) {
	cacheKey := detailRecordCacheKey(symbol, source)
	now := time.Now()
	s.mu.RLock()
	entry, ok := s.detailRecordCache[cacheKey]
	s.mu.RUnlock()
	if !ok || !now.Before(entry.expiresAt) {
		return Record{}, false
	}
	return cloneRecord(entry.value), true
}

func (s *Service) storeDetailRecordCache(record Record, source string) {
	cacheKey := detailRecordCacheKey(record.Symbol, source)
	s.mu.Lock()
	s.detailRecordCache[cacheKey] = detailRecordCacheEntry{
		value:     cloneRecord(record),
		expiresAt: time.Now().Add(detailsRecordCacheTTL),
	}
	s.mu.Unlock()
}

func detailRecordCacheKey(symbol, source string) string {
	return strings.ToUpper(strings.TrimSpace(symbol)) + "|" + strings.ToLower(strings.TrimSpace(source))
}

func cloneRecords(records []Record) []Record {
	out := make([]Record, len(records))
	for idx, record := range records {
		out[idx] = cloneRecord(record)
	}
	return out
}

func cloneRecord(record Record) Record {
	cloned := record
	cloned.SCTR = cloneFloat64Ptr(record.SCTR)
	cloned.Delta = cloneFloat64Ptr(record.Delta)
	cloned.Close = cloneFloat64Ptr(record.Close)
	cloned.MarketCap = cloneFloat64Ptr(record.MarketCap)
	cloned.Vol = cloneInt64Ptr(record.Vol)
	cloned.IndustryRS = cloneFloat64Ptr(record.IndustryRS)
	cloned.SectorRS = cloneFloat64Ptr(record.SectorRS)
	cloned.IndustryAboveMA50 = cloneBoolPtr(record.IndustryAboveMA50)
	cloned.IndustryPercentAbove50 = cloneFloat64Ptr(record.IndustryPercentAbove50)
	return cloned
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func (s *Service) fetchClassificationWithCache(key string, ttl time.Duration, store map[string]classificationCacheEntry, fetcher func() (*Classification, error)) (*Classification, error) {
	now := time.Now()
	s.mu.RLock()
	entry, ok := store[key]
	s.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.value, nil
	}

	value, err := fetcher()
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	store[key] = classificationCacheEntry{
		value:     value,
		expiresAt: now.Add(ttl),
	}
	s.mu.Unlock()
	return value, nil
}

func calculateStats(records []Record) StatsResult {
	industryValues := make(map[string][]float64)
	sectorValues := make(map[string][]float64)
	for _, record := range records {
		if record.SCTR == nil {
			continue
		}
		if industry := strings.TrimSpace(record.Industry); industry != "" {
			industryValues[industry] = append(industryValues[industry], *record.SCTR)
		}
		if sector := strings.TrimSpace(record.Sector); sector != "" {
			sectorValues[sector] = append(sectorValues[sector], *record.SCTR)
		}
	}

	return StatsResult{
		Industries:   aggregateValues(industryValues),
		Sectors:      aggregateValues(sectorValues),
		IndustryMA50: map[string]IndustryMA50{},
	}
}

func aggregateValues(input map[string][]float64) map[string]AggregateStat {
	out := make(map[string]AggregateStat, len(input))
	for key, values := range input {
		if len(values) == 0 {
			continue
		}
		sort.Float64s(values)
		sum := 0.0
		for _, value := range values {
			sum += value
		}
		out[key] = AggregateStat{
			Avg:    sum / float64(len(values)),
			Count:  len(values),
			Min:    values[0],
			Max:    values[len(values)-1],
			Median: values[len(values)/2],
		}
	}
	return out
}

func buildIndustryLookupMap(allRecords, enriched []Record) map[string]string {
	allBySymbol := make(map[string]Record, len(allRecords))
	for _, record := range allRecords {
		allBySymbol[record.Symbol] = record
	}
	mapping := make(map[string]string)
	for _, record := range enriched {
		base, ok := allBySymbol[record.Symbol]
		if !ok {
			continue
		}
		if record.Industry != "" && base.Industry != "" && record.Industry != base.Industry {
			mapping[record.Industry] = base.Industry
		}
	}
	return mapping
}

func calculateRelativeStrength(record Record, stats StatsResult, industryMap map[string]string) (*float64, *float64) {
	if record.SCTR == nil {
		return nil, nil
	}
	industry := strings.TrimSpace(record.Industry)
	sector := strings.TrimSpace(record.Sector)
	statsIndustry := industry
	if mapped, ok := industryMap[industry]; ok && mapped != "" {
		statsIndustry = mapped
	}

	var industryRS *float64
	if agg, ok := stats.Industries[statsIndustry]; ok && agg.Count > 1 && agg.Avg > 0 {
		industryRS = floatPtr(((*record.SCTR - agg.Avg) / agg.Avg) * 100)
	}

	var sectorRS *float64
	if agg, ok := stats.Sectors[sector]; ok && agg.Count > 1 && agg.Avg > 0 {
		sectorRS = floatPtr(((*record.SCTR - agg.Avg) / agg.Avg) * 100)
	}

	return industryRS, sectorRS
}

func (s *Service) calculateIndustryMA50(ctx context.Context, record Record, allRecords []Record) (*IndustryMA50, error) {
	industry := strings.TrimSpace(record.Industry)
	if industry == "" {
		return nil, nil
	}

	now := time.Now()
	s.mu.RLock()
	cached, ok := s.industryMA50[industry]
	s.mu.RUnlock()
	if ok && now.Before(cached.expiresAt) {
		return cached.value, nil
	}

	var result *IndustryMA50
	sector := strings.TrimSpace(record.Sector)
	if etf := findETFForIndustry(industry); etf != "" {
		if ma50, err := s.calculateIndustryMA50FromETF(ctx, etf); err == nil && ma50 != nil {
			ma50.ETF = etf
			result = ma50
		}
	}
	if result == nil && sector != "" {
		if etf := sectorETFMap[sector]; etf != "" {
			if ma50, err := s.calculateIndustryMA50FromETF(ctx, etf); err == nil && ma50 != nil {
				ma50.ETF = etf
				ma50.Source = "sector-etf"
				ma50.SectorFallbackUsed = stringPtr(sector)
				result = ma50
			}
		}
	}
	if result == nil {
		calculated, err := s.calculateIndustryMA50FromStocks(ctx, industry, allRecords)
		if err == nil {
			result = calculated
		}
	}
	if result != nil {
		s.mu.Lock()
		s.industryMA50[industry] = industryMA50CacheEntry{
			value:     result,
			expiresAt: now.Add(industryMA50CacheTTL),
		}
		s.mu.Unlock()
	}
	return result, nil
}

func (s *Service) calculateIndustryMA50FromETF(ctx context.Context, ticker string) (*IndustryMA50, error) {
	prices, err := s.fetchHistoricalPrices(ctx, ticker, defaultHistoricalLookback)
	if err != nil || len(prices) < 50 {
		return nil, err
	}
	ma50 := calculateMA(prices, 50)
	current := prices[len(prices)-1].Close
	if ma50 == 0 {
		return nil, nil
	}
	return &IndustryMA50{
		CurrentIndex:     current,
		MA50:             ma50,
		AboveMA:          current > ma50,
		PercentAboveMA50: ((current - ma50) / ma50) * 100,
		Source:           "ETF",
	}, nil
}

func (s *Service) calculateIndustryMA50FromStocks(ctx context.Context, industry string, allRecords []Record) (*IndustryMA50, error) {
	stocks := make([]Record, 0)
	for _, record := range allRecords {
		if strings.TrimSpace(record.Industry) == industry {
			stocks = append(stocks, record)
		}
	}
	if len(stocks) == 0 {
		return nil, nil
	}
	if len(stocks) > maxIndustryStocksForMA50 {
		stocks = stocks[:maxIndustryStocksForMA50]
	}

	pricesBySymbol := s.fetchIndustryStockHistories(ctx, stocks)
	if len(pricesBySymbol) == 0 {
		return nil, nil
	}

	index := averageIndexFromSeries(pricesBySymbol)
	if len(index) < 50 {
		return nil, nil
	}
	ma50 := calculateMA(index, 50)
	current := index[len(index)-1].Close
	if ma50 == 0 {
		return nil, nil
	}
	stocksUsed := len(pricesBySymbol)
	totalStocks := len(stocks)
	return &IndustryMA50{
		CurrentIndex:     current,
		MA50:             ma50,
		AboveMA:          current > ma50,
		PercentAboveMA50: ((current - ma50) / ma50) * 100,
		Source:           "calculated",
		StocksUsed:       &stocksUsed,
		TotalStocks:      &totalStocks,
	}, nil
}

func (s *Service) fetchIndustryStockHistories(ctx context.Context, stocks []Record) [][]pricePoint {
	type result struct {
		prices []pricePoint
	}

	results := make(chan result, len(stocks))
	sem := make(chan struct{}, maxConcurrentStockHistory)
	var wg sync.WaitGroup

	for _, stock := range stocks {
		symbol := stock.Symbol
		if strings.TrimSpace(symbol) == "" {
			continue
		}
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			prices, err := s.fetchHistoricalPrices(ctx, symbol, defaultHistoricalLookback)
			if err != nil || len(prices) < 50 {
				return
			}
			results <- result{prices: prices}
		}(symbol)
	}

	wg.Wait()
	close(results)

	pricesBySymbol := make([][]pricePoint, 0, len(stocks))
	for item := range results {
		pricesBySymbol = append(pricesBySymbol, item.prices)
	}
	return pricesBySymbol
}

func averageIndexFromSeries(pricesBySymbol [][]pricePoint) []pricePoint {
	type aggregate struct {
		sum   float64
		count int
	}

	byDate := make(map[string]aggregate)
	for _, series := range pricesBySymbol {
		for _, point := range series {
			entry := byDate[point.Date]
			entry.sum += point.Close
			entry.count++
			byDate[point.Date] = entry
		}
	}

	dates := make([]string, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	index := make([]pricePoint, 0, len(dates))
	for _, date := range dates {
		entry := byDate[date]
		if entry.count == 0 {
			continue
		}
		index = append(index, pricePoint{
			Date:  date,
			Close: entry.sum / float64(entry.count),
		})
	}
	return index
}

func (s *Service) fetchHistoricalPrices(ctx context.Context, symbol string, days int) ([]pricePoint, error) {
	key := fmt.Sprintf("%s:%d", strings.ToUpper(symbol), days)
	now := time.Now()

	s.mu.RLock()
	cached, ok := s.historicalPriceData[key]
	s.mu.RUnlock()
	if ok && now.Before(cached.expiresAt) {
		return cached.value, nil
	}

	from := now.AddDate(0, 0, -days)
	bars, err := s.yahooStore.LoadBars(ctx, symbol, from, now)
	if err != nil {
		return nil, err
	}
	out := make([]pricePoint, 0, len(bars))
	for _, bar := range bars {
		if bar.Close <= 0 {
			continue
		}
		out = append(out, pricePoint{
			Date:  bar.Date.Format("2006-01-02"),
			Close: bar.Close,
		})
	}

	s.mu.Lock()
	s.historicalPriceData[key] = historicalPriceCacheEntry{
		value:     out,
		expiresAt: now.Add(historicalPricesCacheTTL),
	}
	s.mu.Unlock()
	return out, nil
}

func calculateMA(points []pricePoint, period int) float64 {
	if len(points) < period {
		return 0
	}
	sum := 0.0
	for _, point := range points[len(points)-period:] {
		sum += point.Close
	}
	return sum / float64(period)
}

func detectDelimiter(text string) rune {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Count(line, "\t") > strings.Count(line, ",") {
			return '\t'
		}
		break
	}
	return ','
}

func parseCSVRows(text string, delimiter rune) ([][]string, error) {
	reader := csv.NewReader(strings.NewReader(text))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	reader.Comma = delimiter
	return reader.ReadAll()
}

func csvRowsToRecords(rows [][]string) ([]string, []map[string]string) {
	if len(rows) == 0 {
		return nil, nil
	}

	firstRow := rows[0]
	if looksLikeHeader(firstRow) {
		columns := make([]string, len(firstRow))
		for i, column := range firstRow {
			columns[i] = strings.TrimSpace(column)
			if columns[i] == "" {
				columns[i] = fmt.Sprintf("col_%d", i)
			}
		}
		records := make([]map[string]string, 0, len(rows)-1)
		for _, row := range rows[1:] {
			record := make(map[string]string, len(columns))
			for i, column := range columns {
				if i < len(row) {
					record[column] = row[i]
				}
			}
			records = append(records, record)
		}
		return columns, records
	}

	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	columns := make([]string, width)
	for i := range columns {
		columns[i] = fmt.Sprintf("col_%d", i)
	}
	records := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		record := make(map[string]string, width)
		for i, column := range columns {
			if i < len(row) {
				record[column] = row[i]
			}
		}
		records = append(records, record)
	}
	return columns, records
}

func looksLikeHeader(row []string) bool {
	for _, value := range row {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if strings.Contains(normalized, "ticker") || strings.Contains(normalized, "symbol") {
			return true
		}
	}
	return false
}

func detectTickerColumnIndex(records []map[string]string, columns []string) int {
	preferred := []string{"ticker", "symbol", "tick", "sym", "symbols", "tickers"}
	normalized := make([]string, len(columns))
	for i, column := range columns {
		normalized[i] = strings.ToLower(strings.TrimSpace(column))
	}
	for _, preferredName := range preferred {
		for idx, column := range normalized {
			if column == preferredName || strings.Contains(column, preferredName) {
				return idx
			}
		}
	}

	bestIndex := 0
	bestScore := -1
	for idx, column := range columns {
		score := 0
		for i, record := range records {
			if i >= 200 {
				break
			}
			if normalizeTickerCandidate(record[column]) != "" {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestIndex = idx
		}
	}
	return bestIndex
}

func extractTickersFromRecords(records []map[string]string, column string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, record := range records {
		ticker := normalizeTickerCandidate(record[column])
		if ticker == "" {
			continue
		}
		if _, ok := seen[ticker]; ok {
			continue
		}
		seen[ticker] = struct{}{}
		out = append(out, ticker)
	}
	return out
}

func normalizeTickerCandidate(value string) string {
	s := strings.TrimSpace(value)
	s = strings.Trim(s, `"`)
	if quoteIdx := strings.Index(s, `"`); quoteIdx >= 0 {
		s = s[:quoteIdx]
	}
	if colonIdx := strings.LastIndex(s, ":"); colonIdx >= 0 && colonIdx < len(s)-1 {
		s = strings.TrimSpace(s[colonIdx+1:])
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" || s == "TICKER" || s == "SYMBOL" {
		return ""
	}
	if matched, _ := regexp.MatchString(`^[A-Z0-9.\-]{1,10}$`, s); !matched {
		return ""
	}
	return s
}

func normalizeTickers(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, value := range values {
		ticker := normalizeTickerCandidate(value)
		if ticker == "" {
			continue
		}
		if _, ok := seen[ticker]; ok {
			continue
		}
		seen[ticker] = struct{}{}
		out = append(out, ticker)
	}
	return out
}

func parseFlexibleDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, value); matched {
		return value
	}
	layouts := []string{"2 Jan 2006", "02 Jan 2006"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return ""
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func toFloatPtr(value any) *float64 {
	str := toString(value)
	if str == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(str, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil
	}
	return &parsed
}

func toInt64Ptr(value any) *int64 {
	str := toString(value)
	if str == "" {
		return nil
	}
	if parsed, err := strconv.ParseInt(str, 10, 64); err == nil {
		return &parsed
	}
	floatValue, err := strconv.ParseFloat(str, 64)
	if err != nil || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
		return nil
	}
	truncated := int64(floatValue)
	return &truncated
}

func cleanHTMLValue(value string) string {
	value = strings.ReplaceAll(value, "&nbsp;", " ")
	value = strings.TrimSpace(value)
	value = strings.Join(strings.Fields(value), " ")
	if value == "Sector" || value == "Industry" {
		return ""
	}
	return value
}

func findFirstMatch(patterns []*regexp.Regexp, text string) string {
	for _, pattern := range patterns {
		if matches := pattern.FindStringSubmatch(text); len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

func recordScore(value *float64) float64 {
	if value == nil {
		return math.Inf(-1)
	}
	return *value
}

func floatPtr(value float64) *float64 {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

var industryETFMap = map[string]string{
	"Semiconductors":                           "SMH",
	"Semiconductor Equipment":                  "SMH",
	"Software":                                 "IGV",
	"Software - Application":                   "IGV",
	"Software - Infrastructure":                "IGV",
	"Software - System":                        "IGV",
	"Internet Content & Information":           "FDN",
	"Information Technology Services":          "IGV",
	"Broadline Retailers":                      "XRT",
	"Specialty Retail":                         "XRT",
	"Internet Retail":                          "XRT",
	"Discount Stores":                          "XRT",
	"Department Stores":                        "XRT",
	"Apparel Retail":                           "XRT",
	"Home Improvement Retail":                  "XRT",
	"Automotive":                               "CARZ",
	"Automobiles":                              "CARZ",
	"Auto Manufacturers":                       "CARZ",
	"Auto Parts":                               "CARZ",
	"Auto & Truck Dealerships":                 "CARZ",
	"Hotels & Motels":                          "PEJ",
	"Restaurants":                              "PEJ",
	"Entertainment":                            "PEJ",
	"Leisure":                                  "PEJ",
	"Recreational Vehicles":                    "PEJ",
	"Banks - Regional":                         "KRE",
	"Banks - Diversified":                      "KBE",
	"Regional Banks":                           "KRE",
	"Money Center Banks":                       "KBE",
	"Capital Markets":                          "IAI",
	"Investment Brokerage":                     "IAI",
	"Insurance":                                "KIE",
	"Property & Casualty Insurance":            "KIE",
	"Life Insurance":                           "KIE",
	"Biotechnology":                            "XBI",
	"Drug Manufacturers":                       "PJP",
	"Drug Manufacturers - Major":               "PJP",
	"Drug Manufacturers - Specialty & Generic": "PJP",
	"Medical Devices":                          "IHI",
	"Medical Instruments & Supplies":           "IHI",
	"Healthcare Plans":                         "IHF",
	"Health Care Plans":                        "IHF",
	"Oil & Gas":                                "XLE",
	"Oil & Gas E&P":                            "XOP",
	"Oil & Gas Drilling":                       "XOP",
	"Oil & Gas Refining & Marketing":           "XLE",
	"Oil & Gas Pipelines":                      "XLE",
	"Aerospace & Defense":                      "ITA",
	"Aerospace/Defense":                        "ITA",
	"Industrial Machinery":                     "XLI",
	"Railroads":                                "IYT",
	"Airlines":                                 "JETS",
	"Shipping":                                 "SEA",
	"Gold":                                     "GDX",
	"Steel":                                    "SLX",
	"Chemicals":                                "IYM",
	"Chemicals - Major Diversified":            "IYM",
	"Utilities":                                "XLU",
	"Electric Utilities":                       "XLU",
	"Gas Utilities":                            "XLU",
	"REITs":                                    "VNQ",
	"Real Estate":                              "VNQ",
	"REIT - Residential":                       "VNQ",
	"REIT - Retail":                            "VNQ",
	"REIT - Office":                            "VNQ",
}

var sectorETFMap = map[string]string{
	"Technology":             "XLK",
	"Consumer Discretionary": "XLY",
	"Consumer Staples":       "XLP",
	"Financials":             "XLF",
	"Healthcare":             "XLV",
	"Energy":                 "XLE",
	"Industrials":            "XLI",
	"Materials":              "XLB",
	"Utilities":              "XLU",
	"Real Estate":            "XLRE",
	"Communication Services": "XLC",
}

func findETFForIndustry(industry string) string {
	if etf := industryETFMap[industry]; etf != "" {
		return etf
	}
	normalized := strings.ToLower(industry)
	switch {
	case strings.Contains(normalized, "semiconductor"):
		return "SMH"
	case strings.Contains(normalized, "software"):
		return "IGV"
	case strings.Contains(normalized, "retail"), strings.Contains(normalized, "retailer"), strings.Contains(normalized, "discount store"), strings.Contains(normalized, "internet retail"):
		return "XRT"
	case strings.Contains(normalized, "auto"), strings.Contains(normalized, "automobile"):
		return "CARZ"
	case strings.Contains(normalized, "bank"):
		if strings.Contains(normalized, "regional") {
			return "KRE"
		}
		return "KBE"
	case strings.Contains(normalized, "capital market"), strings.Contains(normalized, "investment"), strings.Contains(normalized, "broker"), strings.Contains(normalized, "securities"):
		return "IAI"
	case strings.Contains(normalized, "biotech"):
		return "XBI"
	case strings.Contains(normalized, "medical device"), strings.Contains(normalized, "medical instrument"):
		return "IHI"
	case strings.Contains(normalized, "drug"), strings.Contains(normalized, "pharmaceutical"):
		return "PJP"
	case strings.Contains(normalized, "oil"), strings.Contains(normalized, "gas"):
		if strings.Contains(normalized, "exploration") || strings.Contains(normalized, "e&p") || strings.Contains(normalized, "drilling") {
			return "XOP"
		}
		return "XLE"
	case strings.Contains(normalized, "aerospace"), strings.Contains(normalized, "defense"):
		return "ITA"
	default:
		return ""
	}
}
