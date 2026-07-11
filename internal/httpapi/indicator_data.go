package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var stockChartsBreadthSymbols = map[string]string{
	"USI:ADVN.NY": "$NYADV",
	"USI:DECL.NY": "$NYDEC",
}

type indicatorBar struct {
	Symbol string  `json:"symbol"`
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
	VWAP   float64 `json:"vwap"`
}

func (h *apiHandler) indicatorData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	from, err := time.Parse("2006-01-02", r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid from date"))
		return
	}
	to, err := time.Parse("2006-01-02", r.URL.Query().Get("to"))
	if err != nil || to.Before(from) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid to date"))
		return
	}

	requested := strings.Split(r.URL.Query().Get("symbols"), ",")
	series := make(map[string][]indicatorBar, len(requested))
	for _, rawSymbol := range requested {
		symbol := strings.ToUpper(strings.TrimSpace(rawSymbol))
		stockChartsSymbol, ok := stockChartsBreadthSymbols[symbol]
		if !ok {
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported indicator data symbol: %s", symbol))
			return
		}
		bars, err := fetchStockChartsBreadth(r, symbol, stockChartsSymbol, from, to)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		series[symbol] = bars
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": "StockCharts", "series": series})
}

func fetchStockChartsBreadth(r *http.Request, symbol, ticker string, from, to time.Time) ([]indicatorBar, error) {
	params := url.Values{
		"ticker":       {ticker},
		"start":        {from.Format("20060102")},
		"barwidth":     {"D"},
		"out":          {"text"},
		"memberrt":     {"false"},
		"randomNumber": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	endpoint := "https://stockcharts.com/quotebrain/pastdata?" + params.Encode()
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain,*/*;q=0.8")
	req.Header.Set("Referer", "https://stockcharts.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 StockSim/1.0")
	client := &http.Client{Timeout: 45 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch StockCharts breadth data for %s: %w", symbol, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("StockCharts breadth request for %s failed (%d)", symbol, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read StockCharts breadth data for %s: %w", symbol, err)
	}
	bars := parseStockChartsPastData(string(body), symbol, from, to)
	if len(bars) == 0 {
		return nil, fmt.Errorf("no StockCharts breadth data returned for %s (%s)", symbol, ticker)
	}
	return bars, nil
}

func parseStockChartsPastData(text, symbol string, from, to time.Time) []indicatorBar {
	rows := make([]indicatorBar, 0)
	for _, rawLine := range strings.Split(text, "\n") {
		parts := strings.Fields(rawLine)
		if len(parts) < 7 {
			continue
		}
		date, err := time.Parse("1-2-2006", parts[1])
		if err != nil || date.Before(from) || date.After(to) {
			continue
		}
		values := make([]float64, 5)
		valid := true
		for index := range values {
			values[index], err = strconv.ParseFloat(strings.ReplaceAll(parts[index+2], ",", ""), 64)
			if err != nil {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		rows = append(rows, indicatorBar{
			Symbol: symbol, Date: date.Format("2006-01-02"),
			Open: values[0], High: values[1], Low: values[2], Close: values[3], Volume: values[4], VWAP: values[3],
		})
	}
	return rows
}
