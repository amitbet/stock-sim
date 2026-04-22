package details

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

const persistentCacheVersion = 3

type persistentClassificationEntry struct {
	Value     *Classification `json:"value"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

type persistentIndustryMA50Entry struct {
	Value     *IndustryMA50 `json:"value"`
	ExpiresAt time.Time     `json:"expiresAt"`
}

type persistentDetailRecordEntry struct {
	Value     Record    `json:"value"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type persistentSnapshotEntry struct {
	Value     []Record  `json:"value"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type persistentHistoricalPriceEntry struct {
	Value     []pricePoint `json:"value"`
	ExpiresAt time.Time    `json:"expiresAt"`
}

type persistentEarningsDateEntry struct {
	Value     *string   `json:"value"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type persistentEarningsCalendarEntry struct {
	Value     map[string]string `json:"value"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

type persistentCacheState struct {
	Version          int                                        `json:"version"`
	SCTRSnapshots    map[string]persistentSnapshotEntry         `json:"sctrSnapshots,omitempty"`
	DetailRecords    map[string]persistentDetailRecordEntry     `json:"detailRecords"`
	Finviz           map[string]persistentClassificationEntry   `json:"finviz"`
	Yahoo            map[string]persistentClassificationEntry   `json:"yahoo"`
	EarningsDates    map[string]persistentEarningsDateEntry     `json:"earningsDates"`
	EarningsCalendar map[string]persistentEarningsCalendarEntry `json:"earningsCalendar"`
	HistoricalPrices map[string]persistentHistoricalPriceEntry  `json:"historicalPrices"`
	IndustryMA50     map[string]persistentIndustryMA50Entry     `json:"industryMA50"`
}

func (s *Service) loadPersistentCaches() error {
	if s.cacheFilePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.cacheFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state persistentCacheState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Version != persistentCacheVersion {
		return nil
	}

	now := time.Now()
	loadedViews := 0
	detailCount := 0
	finvizCount := 0
	yahooCount := 0
	earningsDateCount := 0
	earningsCalendarCount := 0
	historicalPriceCount := 0
	industryMA50Count := 0
	s.mu.Lock()
	defer s.mu.Unlock()

	for view, entry := range state.SCTRSnapshots {
		if entry.ExpiresAt.After(now) {
			s.sctrSnapshotCache[view] = sctrSnapshotCacheEntry{
				value:     cloneRecords(entry.Value),
				expiresAt: entry.ExpiresAt,
			}
			loadedViews++
		}
	}
	for key, entry := range state.DetailRecords {
		if entry.ExpiresAt.After(now) {
			s.detailRecordCache[key] = detailRecordCacheEntry{
				value:     cloneRecord(entry.Value),
				expiresAt: entry.ExpiresAt,
			}
			detailCount++
		}
	}
	for key, entry := range state.Finviz {
		if entry.ExpiresAt.After(now) {
			s.finvizCache[key] = classificationCacheEntry{
				value:     cloneClassification(entry.Value),
				expiresAt: entry.ExpiresAt,
			}
			finvizCount++
		}
	}
	for key, entry := range state.Yahoo {
		if entry.ExpiresAt.After(now) {
			s.yahooCache[key] = classificationCacheEntry{
				value:     cloneClassification(entry.Value),
				expiresAt: entry.ExpiresAt,
			}
			yahooCount++
		}
	}
	for key, entry := range state.EarningsDates {
		if entry.ExpiresAt.After(now) {
			s.earningsDateCache[key] = earningsDateCacheEntry{
				value:     cloneStringPtr(entry.Value),
				expiresAt: entry.ExpiresAt,
			}
			earningsDateCount++
		}
	}
	for key, entry := range state.EarningsCalendar {
		if entry.ExpiresAt.After(now) {
			s.earningsCalendar[key] = earningsCalendarCacheEntry{
				value:     cloneStringMap(entry.Value),
				expiresAt: entry.ExpiresAt,
			}
			earningsCalendarCount++
		}
	}
	for key, entry := range state.HistoricalPrices {
		if entry.ExpiresAt.After(now) {
			s.historicalPriceData[key] = historicalPriceCacheEntry{
				value:     clonePricePoints(entry.Value),
				expiresAt: entry.ExpiresAt,
			}
			historicalPriceCount++
		}
	}
	for key, entry := range state.IndustryMA50 {
		if entry.ExpiresAt.After(now) && entry.Value != nil {
			s.industryMA50[key] = industryMA50CacheEntry{
				value:     cloneIndustryMA50(entry.Value),
				expiresAt: entry.ExpiresAt,
			}
			industryMA50Count++
		}
	}

	log.Printf(
		"stock-sim: loaded details cache from disk (sctrViews=%d detailRecords=%d finviz=%d yahoo=%d earningsDates=%d earningsCalendar=%d historicalPrices=%d industryMA50=%d)",
		loadedViews,
		detailCount,
		finvizCount,
		yahooCount,
		earningsDateCount,
		earningsCalendarCount,
		historicalPriceCount,
		industryMA50Count,
	)

	return nil
}

func (s *Service) persistCaches() {
	if !s.persistentCacheReady() {
		return
	}

	state := s.snapshotPersistentCaches()

	s.diskMu.Lock()
	defer s.diskMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.cacheFilePath), 0o755); err != nil {
		s.disablePersistentCache(err)
		return
	}

	payload, err := json.Marshal(state)
	if err != nil {
		s.disablePersistentCache(err)
		return
	}

	tmpPath := s.cacheFilePath + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		s.disablePersistentCache(err)
		return
	}
	_ = os.Remove(s.cacheFilePath)
	if err := os.Rename(tmpPath, s.cacheFilePath); err != nil {
		_ = os.Remove(tmpPath)
		s.disablePersistentCache(err)
		return
	}
}

func (s *Service) snapshotPersistentCaches() persistentCacheState {
	now := time.Now()
	state := persistentCacheState{
		Version:          persistentCacheVersion,
		SCTRSnapshots:    make(map[string]persistentSnapshotEntry),
		DetailRecords:    make(map[string]persistentDetailRecordEntry),
		Finviz:           make(map[string]persistentClassificationEntry),
		Yahoo:            make(map[string]persistentClassificationEntry),
		EarningsDates:    make(map[string]persistentEarningsDateEntry),
		EarningsCalendar: make(map[string]persistentEarningsCalendarEntry),
		HistoricalPrices: make(map[string]persistentHistoricalPriceEntry),
		IndustryMA50:     make(map[string]persistentIndustryMA50Entry),
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for view, entry := range s.sctrSnapshotCache {
		if entry.value != nil && entry.expiresAt.After(now) {
			state.SCTRSnapshots[view] = persistentSnapshotEntry{
				Value:     cloneRecords(entry.value),
				ExpiresAt: entry.expiresAt,
			}
		}
	}
	for key, entry := range s.detailRecordCache {
		if entry.expiresAt.After(now) {
			state.DetailRecords[key] = persistentDetailRecordEntry{
				Value:     cloneRecord(entry.value),
				ExpiresAt: entry.expiresAt,
			}
		}
	}
	for key, entry := range s.finvizCache {
		if entry.expiresAt.After(now) {
			state.Finviz[key] = persistentClassificationEntry{
				Value:     cloneClassification(entry.value),
				ExpiresAt: entry.expiresAt,
			}
		}
	}
	for key, entry := range s.yahooCache {
		if entry.expiresAt.After(now) {
			state.Yahoo[key] = persistentClassificationEntry{
				Value:     cloneClassification(entry.value),
				ExpiresAt: entry.expiresAt,
			}
		}
	}
	for key, entry := range s.earningsDateCache {
		if entry.expiresAt.After(now) {
			state.EarningsDates[key] = persistentEarningsDateEntry{
				Value:     cloneStringPtr(entry.value),
				ExpiresAt: entry.expiresAt,
			}
		}
	}
	for key, entry := range s.earningsCalendar {
		if entry.expiresAt.After(now) {
			state.EarningsCalendar[key] = persistentEarningsCalendarEntry{
				Value:     cloneStringMap(entry.value),
				ExpiresAt: entry.expiresAt,
			}
		}
	}
	for key, entry := range s.historicalPriceData {
		if entry.expiresAt.After(now) {
			state.HistoricalPrices[key] = persistentHistoricalPriceEntry{
				Value:     clonePricePoints(entry.value),
				ExpiresAt: entry.expiresAt,
			}
		}
	}
	for key, entry := range s.industryMA50 {
		if entry.expiresAt.After(now) && entry.value != nil {
			state.IndustryMA50[key] = persistentIndustryMA50Entry{
				Value:     cloneIndustryMA50(entry.value),
				ExpiresAt: entry.expiresAt,
			}
		}
	}

	return state
}

func (s *Service) persistentCacheReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.persistentCacheOK && s.cacheFilePath != ""
}

func (s *Service) disablePersistentCache(err error) {
	s.mu.Lock()
	alreadyDisabled := !s.persistentCacheOK
	s.persistentCacheOK = false
	s.mu.Unlock()
	if !alreadyDisabled {
		log.Printf("stock-sim: details persistent cache disabled, falling back to memory-only cache: %v", err)
	}
}

func cloneClassification(value *Classification) *Classification {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneIndustryMA50(value *IndustryMA50) *IndustryMA50 {
	if value == nil {
		return nil
	}
	copyValue := *value
	if value.StocksUsed != nil {
		stocksUsed := *value.StocksUsed
		copyValue.StocksUsed = &stocksUsed
	}
	if value.TotalStocks != nil {
		totalStocks := *value.TotalStocks
		copyValue.TotalStocks = &totalStocks
	}
	if value.SectorFallbackUsed != nil {
		sector := *value.SectorFallbackUsed
		copyValue.SectorFallbackUsed = &sector
	}
	return &copyValue
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	out := make(map[string]string, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func clonePricePoints(points []pricePoint) []pricePoint {
	if points == nil {
		return nil
	}
	out := make([]pricePoint, len(points))
	copy(out, points)
	return out
}
