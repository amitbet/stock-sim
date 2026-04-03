package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

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

type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
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

	return &Store{db: db}, nil
}

func (s *Store) ListSymbols(ctx context.Context) ([]string, error) {
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
