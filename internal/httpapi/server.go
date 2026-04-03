package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"stock-sim/internal/data"
	"stock-sim/internal/plan"
	"stock-sim/internal/sim"
)

type Config struct {
	Addr       string
	DBPath     string
	UIDistPath string
}

type Server struct {
	httpServer *http.Server
	store      *data.Store
}

//go:embed dist/*
var embeddedUIDist embed.FS

func NewServer(cfg Config) (*Server, error) {
	store, err := data.NewStore(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	api := &apiHandler{store: store}
	mux.HandleFunc("/api/health", api.health)
	mux.HandleFunc("/api/default-plan", api.defaultPlan)
	mux.HandleFunc("/api/symbols", api.symbols)
	mux.HandleFunc("/api/bars", api.bars)
	mux.HandleFunc("/api/plans/validate", api.validatePlan)
	mux.HandleFunc("/api/simulations/run", api.runSimulation)
	mux.HandleFunc("/api/simulations/batch", api.runBatch)

	uiHandler, err := staticHandler(cfg.UIDistPath)
	if err != nil {
		return nil, err
	}
	mux.Handle("/", uiHandler)

	return &Server{
		httpServer: &http.Server{
			Addr:    cfg.Addr,
			Handler: mux,
		},
		store: store,
	}, nil
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

type apiHandler struct {
	store *data.Store
}

func (h *apiHandler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *apiHandler) defaultPlan(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan.DefaultQQQPlanYAML})
}

func (h *apiHandler) symbols(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	symbols, err := h.store.ListSymbols(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"symbols": symbols})
}

func (h *apiHandler) bars(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	from, err := time.Parse("2006-01-02", r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid from date"))
		return
	}
	to, err := time.Parse("2006-01-02", r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid to date"))
		return
	}

	bars, err := h.store.LoadBars(r.Context(), symbol, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bars": bars})
}

func (h *apiHandler) validatePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	body, err := decodeRequestBody[planRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	parsed, validation, err := h.parseAndValidate(r.Context(), body.Symbol, body.Plan)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":    validation.Valid,
		"errors":   validation.Errors,
		"warnings": validation.Warnings,
		"plan":     parsed,
	})
}

func (h *apiHandler) runSimulation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	body, err := decodeRequestBody[simulationRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	result, err := h.executeOne(
		r.Context(),
		body.Symbol,
		body.ReferenceSellDate,
		body.Plan,
		body.ExecutionPriceMode,
		body.ReferencePriceMode,
		body.ReferencePrice,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if body.HoldDaysAfterFullInvest != nil {
		result, err = h.executeOneWithHoldOverride(
			r.Context(),
			body.Symbol,
			body.ReferenceSellDate,
			body.Plan,
			body.ExecutionPriceMode,
			body.ReferencePriceMode,
			body.ReferencePrice,
			body.HoldDaysAfterFullInvest,
		)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *apiHandler) runBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	body, err := decodeRequestBody[batchSimulationRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var result sim.BatchResult
	for _, dateString := range body.ReferenceSellDates {
		run, err := h.executeOneWithHoldOverride(
			r.Context(),
			body.Symbol,
			dateString,
			body.Plan,
			body.ExecutionPriceMode,
			body.ReferencePriceMode,
			nil,
			body.HoldDaysAfterFullInvest,
		)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("%s: %w", dateString, err))
			return
		}
		result.Runs = append(result.Runs, run)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *apiHandler) executeOne(ctx context.Context, symbol, referenceSellDate, rawPlan string, mode sim.ExecutionPriceMode, referencePriceMode sim.ReferencePriceMode, referencePriceOverride *float64) (sim.Result, error) {
	return h.executeOneWithHoldOverride(ctx, symbol, referenceSellDate, rawPlan, mode, referencePriceMode, referencePriceOverride, nil)
}

func (h *apiHandler) executeOneWithHoldOverride(ctx context.Context, symbol, referenceSellDate, rawPlan string, mode sim.ExecutionPriceMode, referencePriceMode sim.ReferencePriceMode, referencePriceOverride *float64, holdDaysOverride *int) (sim.Result, error) {
	var result sim.Result
	refDate, err := time.Parse("2006-01-02", referenceSellDate)
	if err != nil {
		return result, fmt.Errorf("invalid reference_sell_date")
	}

	parsed, validation, err := h.parseAndValidate(ctx, symbol, rawPlan)
	if err != nil {
		return result, err
	}
	if !validation.Valid {
		return result, fmt.Errorf("%s", strings.Join(validation.Errors, "; "))
	}
	if holdDaysOverride != nil {
		override := *holdDaysOverride
		parsed.Exit.HoldDaysAfterFullInvest = &override
	}

	bars, err := h.store.LoadBarsAround(ctx, symbol, refDate, 180, 200)
	if err != nil {
		return result, err
	}
	if err := validateReferencePriceOverride(bars, refDate, referencePriceOverride); err != nil {
		return result, err
	}

	return sim.Run(bars, refDate, parsed, mode, referencePriceMode, referencePriceOverride)
}

func validateReferencePriceOverride(bars []data.Bar, refDate time.Time, referencePriceOverride *float64) error {
	if referencePriceOverride == nil {
		return nil
	}

	for _, bar := range bars {
		if bar.Date.Format("2006-01-02") != refDate.Format("2006-01-02") {
			continue
		}
		if *referencePriceOverride < bar.Low || *referencePriceOverride > bar.High {
			return fmt.Errorf(
				"reference_price %.2f must be within the selected candle range %.2f to %.2f",
				*referencePriceOverride,
				bar.Low,
				bar.High,
			)
		}
		return nil
	}

	return fmt.Errorf("reference sell date %s not found in bar set", refDate.Format("2006-01-02"))
}

func (h *apiHandler) parseAndValidate(ctx context.Context, symbol, rawPlan string) (plan.StrategyPlan, plan.ValidationResult, error) {
	parsed, err := plan.Parse(rawPlan)
	if err != nil {
		return parsed, plan.ValidationResult{}, err
	}

	// Use a generous range to validate indicator windows without requiring the run date yet.
	bars, err := h.store.LoadBars(ctx, symbol, time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return parsed, plan.ValidationResult{}, err
	}

	validation := plan.Validate(parsed, len(bars))
	return parsed, validation, nil
}

type planRequest struct {
	Symbol string `json:"symbol"`
	Plan   string `json:"plan"`
}

type simulationRequest struct {
	Symbol                  string                 `json:"symbol"`
	ReferenceSellDate       string                 `json:"reference_sell_date"`
	Plan                    string                 `json:"plan"`
	ExecutionPriceMode      sim.ExecutionPriceMode `json:"execution_price_mode"`
	ReferencePriceMode      sim.ReferencePriceMode `json:"reference_price_mode"`
	ReferencePrice          *float64               `json:"reference_price,omitempty"`
	HoldDaysAfterFullInvest *int                   `json:"hold_days_after_full_invest,omitempty"`
}

type batchSimulationRequest struct {
	Symbol                  string                 `json:"symbol"`
	ReferenceSellDates      []string               `json:"reference_sell_dates"`
	Plan                    string                 `json:"plan"`
	ExecutionPriceMode      sim.ExecutionPriceMode `json:"execution_price_mode"`
	ReferencePriceMode      sim.ReferencePriceMode `json:"reference_price_mode"`
	HoldDaysAfterFullInvest *int                   `json:"hold_days_after_full_invest,omitempty"`
}

func staticHandler(uiDistPath string) (http.Handler, error) {
	if hasBuiltUI(uiDistPath) {
		return spaFileServer(http.Dir(uiDistPath)), nil
	}

	sub, err := fs.Sub(embeddedUIDist, "dist")
	if err != nil {
		return nil, fmt.Errorf("open embedded ui dist: %w", err)
	}
	return spaFileServer(http.FS(sub)), nil
}

func hasBuiltUI(path string) bool {
	info, err := os.Stat(filepath.Join(path, "index.html"))
	return err == nil && !info.IsDir()
}

func spaFileServer(root http.FileSystem) http.Handler {
	files := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if _, err := root.Open(strings.TrimPrefix(r.URL.Path, "/")); err == nil {
			files.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		files.ServeHTTP(w, r)
	})
}

func decodeRequestBody[T any](r *http.Request) (T, error) {
	var payload T
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return payload, fmt.Errorf("decode request body: %w", err)
	}
	return payload, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}
