import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import BatchReportModal from "./components/BatchReportModal.jsx";
import CandleChart from "./components/CandleChart.jsx";
import Controls from "./components/Controls.jsx";
import PlanEditor from "./components/PlanEditor.jsx";
import ResultsPanel from "./components/ResultsPanel.jsx";
import StockDetailsPanel from "./components/StockDetailsPanel.jsx";
import {
  fetchBars,
  fetchDataSources,
  fetchDefaultPlan,
  fetchSymbolInfo,
  fetchSymbols,
  runBatchSimulation,
  runSimulation,
  validatePlan
} from "./lib/api.js";

const AUTO_RUN_DEBOUNCE_MS = 350;

/** Desktop-only: GitHub release check on load and on this interval (ms). */
const UPDATE_CHECK_INTERVAL_MS = 60 * 60 * 1000;

const THEME_STORAGE_KEY = "stock-sim-theme";

/** Injected in vite.config.js from package.json (always available in dev/build). */
const UI_PKG_VERSION = import.meta.env.VITE_UI_PKG_VERSION || "";

function readStoredTheme() {
  try {
    const v = localStorage.getItem(THEME_STORAGE_KEY);
    if (v === "light" || v === "dark") {
      return v;
    }
  } catch {
    /* ignore */
  }
  return "dark";
}

function formatISODate(value) {
  return value.toISOString().slice(0, 10);
}

function historyRange() {
  const today = new Date();
  const to = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate()));
  const from = new Date(Date.UTC(to.getUTCFullYear() - 30, to.getUTCMonth(), to.getUTCDate()));
  return {
    from: formatISODate(from),
    to: formatISODate(to)
  };
}

function normalizeBarDate(value) {
  return value ? String(value).slice(0, 10) : "";
}

function findBarForDate(bars, date) {
  return bars.find((bar) => normalizeBarDate(bar.date) === date) || null;
}

function referencePriceFromBar(bar, source) {
  if (!bar) {
    return "";
  }
  switch (source) {
    case "open":
      return String(bar.open);
    case "high":
      return String(bar.high);
    case "low":
      return String(bar.low);
    default:
      return String(bar.close);
  }
}

function parseReferencePrice(value) {
  if (String(value ?? "").trim() === "") {
    return undefined;
  }
  const parsed = Number.parseFloat(value);
  return Number.isNaN(parsed) ? undefined : parsed;
}

function referencePriceValidation(bar, value) {
  if (!bar) {
    return "";
  }

  const trimmed = String(value ?? "").trim();
  if (!trimmed) {
    return "";
  }

  const parsed = Number.parseFloat(trimmed);
  if (Number.isNaN(parsed)) {
    return "S price override must be a number.";
  }

  if (parsed < bar.low || parsed > bar.high) {
    return `S price override must stay within the selected candle range (${bar.low} to ${bar.high}).`;
  }

  return "";
}

function downloadTextFile(filename, content) {
  const blob = new Blob([content], { type: "text/yaml;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

export default function App() {
  const [theme, setTheme] = useState(() => {
    const t = readStoredTheme();
    if (typeof document !== "undefined") {
      document.documentElement.dataset.theme = t;
    }
    return t;
  });
  const [activeTab, setActiveTab] = useState("simulator");

  useLayoutEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch {
      /* ignore */
    }
  }, [theme]);

  const fileInputRef = useRef(null);
  const latestSingleRunIdRef = useRef(0);
  const autoRunKeyRef = useRef("");
  const [dataSources, setDataSources] = useState([]);
  const [dataSource, setDataSource] = useState("");
  const [symbols, setSymbols] = useState([]);
  const [symbol, setSymbol] = useState("QQQ");
  const [symbolDescription, setSymbolDescription] = useState("");
  const [bars, setBars] = useState([]);
  const [planText, setPlanText] = useState("");
  const [validation, setValidation] = useState(null);
  const [selectedDate, setSelectedDate] = useState("");
  const [multiSelectedDates, setMultiSelectedDates] = useState([]);
  const [multiSelectEnabled, setMultiSelectEnabled] = useState(false);
  const [executionMode, setExecutionMode] = useState("exact");
  const [referencePriceMode, setReferencePriceMode] = useState("close");
  const [referencePrice, setReferencePrice] = useState("");
  const [holdDaysOverride, setHoldDaysOverride] = useState("");
  const [singleResult, setSingleResult] = useState(null);
  const [batchResult, setBatchResult] = useState(null);
  const [batchOpen, setBatchOpen] = useState(false);
  const [selectedRunIndex, setSelectedRunIndex] = useState(0);
  const [loading, setLoading] = useState(false);
  const [validating, setValidating] = useState(false);
  const [error, setError] = useState("");
  const [updateStatus, setUpdateStatus] = useState(null);
  const [updateBannerDismissed, setUpdateBannerDismissed] = useState(false);
  const [updateBusy, setUpdateBusy] = useState(false);
  const [appVersion, setAppVersion] = useState("");
  const [isDesktopApp, setIsDesktopApp] = useState(false);
  /** Latest ticker; used when fetchSymbols completes async so we preserve user choice across data source changes. */
  const symbolRef = useRef(symbol);
  symbolRef.current = symbol;

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        // Wails injects window.go after load; production builds are not DEV, so we must wait for
        // bindings in release too — otherwise isDesktopApp/appVersion never set (no footer version).
        const ua = typeof navigator !== "undefined" ? navigator.userAgent || "" : "";
        const wailsHost =
          typeof window !== "undefined" &&
          (typeof window.runtime !== "undefined" ||
            typeof window.chrome?.webview !== "undefined" ||
            /\bWails\b/i.test(ua));
        if (typeof window !== "undefined" && !window.go?.main?.App && wailsHost) {
          for (let i = 0; i < 80 && !window.go?.main?.App; i++) {
            await new Promise((r) => setTimeout(r, 25));
          }
        }
        if (typeof window !== "undefined" && window.go?.main?.App) {
          setIsDesktopApp(true);
          const { Version } = await import("../wailsjs/go/main/App.js");
          let v = await Version();
          for (let i = 0; i < 40 && !v && !cancelled; i++) {
            await new Promise((r) => setTimeout(r, 25));
            v = await Version();
          }
          if (!cancelled) {
            const s = v != null ? String(v).trim() : "";
            setAppVersion(s || "unknown");
          }
        } else if (import.meta.env.DEV) {
          setAppVersion("dev");
        }
      } catch {
        if (!cancelled && import.meta.env.DEV) {
          setAppVersion("dev");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const refreshUpdateStatus = useCallback(async () => {
    if (typeof window === "undefined" || !window.go?.main?.App) {
      return;
    }
    try {
      const { CheckForUpdates } = await import("../wailsjs/go/main/App.js");
      const status = await CheckForUpdates();
      setUpdateStatus(status);
    } catch {
      /* ignore */
    }
  }, []);

  useEffect(() => {
    if (!isDesktopApp) {
      return undefined;
    }
    void refreshUpdateStatus();
    const id = window.setInterval(() => {
      void refreshUpdateStatus();
    }, UPDATE_CHECK_INTERVAL_MS);
    return () => {
      window.clearInterval(id);
    };
  }, [isDesktopApp, refreshUpdateStatus]);

  useEffect(() => {
    async function bootstrap() {
      let nextError = "";

      try {
        const sourcePayload = await fetchDataSources();
        const nextSources = sourcePayload.sources || [];
        setDataSources(nextSources);
        if (sourcePayload.default_source) {
          setDataSource(sourcePayload.default_source);
        } else if (nextSources.length > 0) {
          setDataSource(nextSources[0]);
        }
      } catch (err) {
        nextError = nextError || err.message;
      }

      try {
        const planPayload = await fetchDefaultPlan();
        setPlanText(planPayload.plan || "");
      } catch (err) {
        nextError = err.message;
      }

      if (nextError) {
        setError(nextError);
      }
    }
    bootstrap();
  }, []);

  useEffect(() => {
    if (!symbol || !dataSource) {
      return;
    }
    async function loadBars() {
      try {
        setError("");
        const range = historyRange();
        const payload = await fetchBars(dataSource, symbol, range.from, range.to);
        setBars(payload.bars || []);
        setSelectedDate("");
        setReferencePrice("");
        setMultiSelectedDates([]);
        setSingleResult(null);
      } catch (err) {
        setError(err.message);
      }
    }
    loadBars();
  }, [dataSource, symbol]);

  useEffect(() => {
    if (!dataSource) {
      return;
    }
    async function loadSymbols() {
      try {
        setError("");
        const symbolsPayload = await fetchSymbols(dataSource);
        const list = symbolsPayload.symbols || [];
        const prev = symbolRef.current;
        const prevUpper = String(prev || "").toUpperCase();

        let next;
        if (!list.length) {
          next = prevUpper || "";
        } else {
          const match = list.find((s) => String(s).toUpperCase() === prevUpper);
          if (match !== undefined) {
            next = match;
          } else if (prevUpper) {
            // Yahoo preset list starts with QQQ, but the API can load any ticker — keep the user's symbol.
            next = prevUpper;
          } else {
            next = list[0];
          }
        }

        const inList = list.some((s) => String(s).toUpperCase() === String(next).toUpperCase());
        setSymbols(inList ? list : [next, ...list]);
        setSymbol(next);
      } catch (err) {
        setError(err.message);
      }
    }
    loadSymbols();
  }, [dataSource]);

  useEffect(() => {
    if (!symbol) {
      setSymbolDescription("");
      return;
    }

    let cancelled = false;
    async function loadSymbolInfo() {
      try {
        const payload = await fetchSymbolInfo(dataSource, symbol);
        if (!cancelled) {
          const info = payload.info || {};
          setSymbolDescription(info.description || info.name || "");
        }
      } catch {
        if (!cancelled) {
          setSymbolDescription("");
        }
      }
    }
    loadSymbolInfo();

    return () => {
      cancelled = true;
    };
  }, [dataSource, symbol]);

  const selectedBatchCount = multiSelectedDates.length;
  const selectedBar = useMemo(() => findBarForDate(bars, selectedDate), [bars, selectedDate]);
  const referencePriceError = useMemo(
    () => referencePriceValidation(selectedBar, referencePrice),
    [referencePrice, selectedBar]
  );
  const canRunSingle = Boolean(selectedDate) && !referencePriceError;

  const actionOverlay = useMemo(() => singleResult?.actions || [], [singleResult]);
  const endDate = singleResult?.summary?.end_date || "";

  async function handleValidate() {
    setValidating(true);
    try {
      const payload = await validatePlan(dataSource, symbol, planText);
      setValidation(payload);
      setError("");
    } catch (err) {
      setError(err.message);
    } finally {
      setValidating(false);
    }
  }

  function normalizedHoldDaysOverride() {
    if (holdDaysOverride.trim() === "") {
      return undefined;
    }
    const parsed = Number.parseInt(holdDaysOverride, 10);
    return Number.isNaN(parsed) ? undefined : parsed;
  }

  function normalizedReferencePrice() {
    return parseReferencePrice(referencePrice);
  }

  async function runSingleForDate(date, referencePriceValue = normalizedReferencePrice()) {
    if (!date) {
      return;
    }
    const runId = latestSingleRunIdRef.current + 1;
    latestSingleRunIdRef.current = runId;
    setLoading(true);
    try {
      const payload = await runSimulation({
        symbol,
        data_source: dataSource,
        reference_sell_date: date,
        plan: planText,
        execution_price_mode: executionMode,
        reference_price_mode: referencePriceMode,
        reference_price: referencePriceValue,
        hold_days_after_full_invest: normalizedHoldDaysOverride()
      });
      if (latestSingleRunIdRef.current === runId) {
        setSingleResult(payload);
        setError("");
      }
    } catch (err) {
      if (latestSingleRunIdRef.current === runId) {
        setError(err.message);
      }
    } finally {
      if (latestSingleRunIdRef.current === runId) {
        setLoading(false);
      }
    }
  }

  async function handleRunSingle() {
    if (referencePriceError) {
      return;
    }
    return runSingleForDate(selectedDate);
  }

  async function handleRunBatch() {
    if (multiSelectedDates.length === 0) {
      return;
    }
    setLoading(true);
    try {
      const payload = await runBatchSimulation({
        symbol,
        data_source: dataSource,
        reference_sell_dates: multiSelectedDates,
        plan: planText,
        execution_price_mode: executionMode,
        reference_price_mode: referencePriceMode,
        hold_days_after_full_invest: normalizedHoldDaysOverride()
      });
      setBatchResult(payload);
      setSelectedRunIndex(0);
      setBatchOpen(true);
      setError("");
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  function handleSelectDate(date) {
    if (multiSelectEnabled) {
      setMultiSelectedDates((current) =>
        current.includes(date) ? current.filter((item) => item !== date) : [...current, date].sort()
      );
      return;
    }
    setSelectedDate(date);
    const nextReferencePrice = referencePriceFromBar(findBarForDate(bars, date), referencePriceMode);
    setReferencePrice(nextReferencePrice);
    setError("");
    setSingleResult(null);
    runSingleForDate(date, parseReferencePrice(nextReferencePrice));
  }

  function handleToggleMultiSelect(nextValue) {
    setMultiSelectEnabled(nextValue);
    if (!nextValue) {
      setMultiSelectedDates([]);
    }
  }

  function handleSavePlan() {
    downloadTextFile(`${symbol.toLowerCase()}-strategy.yaml`, planText);
  }

  function handleLoadPlan() {
    fileInputRef.current?.click();
  }

  function handlePlanFile(event) {
    const file = event.target.files?.[0];
    if (!file) {
      return;
    }
    file.text().then((text) => {
      setPlanText(text);
      setValidation(null);
    });
  }

  useEffect(() => {
    if (!selectedDate || multiSelectEnabled) {
      return;
    }

    setReferencePrice(referencePriceFromBar(findBarForDate(bars, selectedDate), referencePriceMode));
  }, [bars, multiSelectEnabled, referencePriceMode, selectedDate]);

  useEffect(() => {
    if (!selectedDate || multiSelectEnabled) {
      autoRunKeyRef.current = "";
      return;
    }

    if (referencePriceError) {
      return;
    }

    const autoRunKey = JSON.stringify({
      selectedDate,
      planText,
      executionMode,
      referencePriceMode,
      referencePrice,
      holdDaysOverride
    });

    if (!autoRunKeyRef.current) {
      autoRunKeyRef.current = autoRunKey;
      return;
    }

    if (autoRunKeyRef.current === autoRunKey) {
      return;
    }

    const timeoutId = window.setTimeout(() => {
      autoRunKeyRef.current = autoRunKey;
      runSingleForDate(selectedDate);
    }, AUTO_RUN_DEBOUNCE_MS);

    return () => window.clearTimeout(timeoutId);
  }, [executionMode, holdDaysOverride, multiSelectEnabled, planText, referencePrice, referencePriceError, referencePriceMode, selectedDate]);

  async function handleApplyUpdate() {
    setUpdateBusy(true);
    setError("");
    try {
      const { ApplyUpdateAndRestart } = await import("../wailsjs/go/main/App.js");
      await ApplyUpdateAndRestart();
    } catch (err) {
      setError(err?.message || String(err));
      setUpdateBusy(false);
    }
  }

  const isSimulatorTab = activeTab === "simulator";

  return (
    <div className="app-shell">
      <header className="hero">
        <div className="hero-copy">
          <div className="eyebrow">Market replay workstation</div>
          <h1>Stock Simulator</h1>
          <p>Click a sell date, replay staged re-entry, compare outcomes.</p>
        </div>
        <div className="hero-right">
          <button
            type="button"
            className="theme-switch"
            role="switch"
            aria-checked={theme === "light"}
            aria-label={theme === "light" ? "Use dark theme" : "Use light theme"}
            onClick={() => setTheme((prev) => (prev === "dark" ? "light" : "dark"))}
          >
            <span className="theme-switch-track" aria-hidden>
              <span className="theme-switch-thumb" />
            </span>
            <span className="theme-switch-label">{theme === "light" ? "Light" : "Dark"}</span>
          </button>
          <div className="hero-card">
            <div className="hero-metric">
              <span>Symbol</span>
              <strong>{symbol || "--"}</strong>
            </div>
            <div className="hero-metric">
              <span>Bars loaded</span>
              <strong>{bars.length}</strong>
            </div>
            <div className="hero-metric">
              <span>Batch picks</span>
              <strong>{selectedBatchCount}</strong>
            </div>
          </div>
        </div>
      </header>

      {updateStatus?.update_available && !updateBannerDismissed ? (
        <div className="update-banner" role="status">
          <span>
            A new version is available: <strong>{updateStatus.latest}</strong> (you have {updateStatus.current}).
          </span>
          <div className="update-banner-actions">
            <button type="button" className="update-banner-primary" disabled={updateBusy} onClick={() => void handleApplyUpdate()}>
              {updateBusy ? "Updating…" : "Update"}
            </button>
            <button
              type="button"
              className="update-banner-dismiss"
              disabled={updateBusy}
              onClick={() => {
                setUpdateBannerDismissed(true);
              }}
            >
              Dismiss
            </button>
          </div>
        </div>
      ) : null}

      {error ? <div className="error-banner">{error}</div> : null}

      <div className="tab-bar" role="tablist" aria-label="Workspace views">
        <button
          type="button"
          role="tab"
          aria-selected={isSimulatorTab}
          className={`tab-button${isSimulatorTab ? " active" : ""}`}
          onClick={() => setActiveTab("simulator")}
        >
          Simulator
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={!isSimulatorTab}
          className={`tab-button${!isSimulatorTab ? " active" : ""}`}
          onClick={() => setActiveTab("details")}
        >
          Stock Details
        </button>
      </div>

      <div className={`tab-panel${isSimulatorTab ? " active" : ""}`} role="tabpanel" aria-label="Simulator" hidden={!isSimulatorTab}>
        <div className="main-grid">
          <section className="panel chart-panel">
            <div className="panel-header">
              <div>
                <h2>Daily Candles</h2>
                <p>Scroll and pinch to zoom, drag to pan, click once for a single run or toggle batch mode to collect many dates.</p>
              </div>
            </div>
            <CandleChart
              bars={bars}
              selectedDate={selectedDate}
              multiSelectedDates={multiSelectedDates}
              multiSelectEnabled={multiSelectEnabled}
              onSelectDate={handleSelectDate}
              actions={actionOverlay}
              endDate={endDate}
              theme={theme}
            />
          </section>

          <aside className="sidebar-scroll" aria-label="Simulation controls and results">
            <div className="side-grid">
              <Controls
                dataSources={dataSources}
                dataSource={dataSource}
                onDataSourceChange={setDataSource}
                symbolDescription={symbolDescription}
                symbols={symbols}
                symbol={symbol}
                onSymbolChange={setSymbol}
                executionMode={executionMode}
                onExecutionModeChange={setExecutionMode}
                referencePriceMode={referencePriceMode}
                onReferencePriceModeChange={setReferencePriceMode}
                referencePrice={referencePrice}
                onReferencePriceChange={setReferencePrice}
                referencePriceError={referencePriceError}
                selectedBar={selectedBar}
                holdDaysOverride={holdDaysOverride}
                onHoldDaysOverrideChange={setHoldDaysOverride}
                selectedDate={selectedDate}
                multiSelectEnabled={multiSelectEnabled}
                onToggleMultiSelect={handleToggleMultiSelect}
                selectedBatchCount={selectedBatchCount}
                onRunSingle={handleRunSingle}
                onRunBatch={handleRunBatch}
                canRunSingle={canRunSingle}
                running={loading}
              />

              <PlanEditor
                value={planText}
                onChange={setPlanText}
                validation={validation}
                onValidate={handleValidate}
                onLoad={handleLoadPlan}
                onSave={handleSavePlan}
                validating={validating}
              />

              <ResultsPanel result={singleResult} />
            </div>
          </aside>
        </div>
      </div>

      <div className={`tab-panel${!isSimulatorTab ? " active" : ""}`} role="tabpanel" aria-label="Stock Details" hidden={isSimulatorTab}>
        <section className="panel details-tab-panel">
          <StockDetailsPanel />
        </section>
      </div>

      <input ref={fileInputRef} hidden type="file" accept=".yaml,.yml,.json,.txt" onChange={handlePlanFile} />

      <BatchReportModal
        open={batchOpen}
        result={batchResult}
        selectedRunIndex={selectedRunIndex}
        onSelectRun={setSelectedRunIndex}
        onClose={() => setBatchOpen(false)}
      />

      <footer className="app-version-footer" aria-label="Application version">
        <span>
          {appVersion
            ? `v${appVersion}`
            : import.meta.env.DEV
              ? `v${UI_PKG_VERSION || "?"} dev`
              : `v${UI_PKG_VERSION || "…"}`}
        </span>
      </footer>
    </div>
  );
}
