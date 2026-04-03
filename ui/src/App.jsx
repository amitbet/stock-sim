import { useEffect, useMemo, useRef, useState } from "react";
import BatchReportModal from "./components/BatchReportModal.jsx";
import CandleChart from "./components/CandleChart.jsx";
import Controls from "./components/Controls.jsx";
import PlanEditor from "./components/PlanEditor.jsx";
import ResultsPanel from "./components/ResultsPanel.jsx";
import {
  fetchBars,
  fetchDefaultPlan,
  fetchSymbols,
  runBatchSimulation,
  runSimulation,
  validatePlan
} from "./lib/api.js";

const RANGE_FROM = "2023-01-01";
const RANGE_TO = "2026-12-31";
const AUTO_RUN_DEBOUNCE_MS = 350;

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
  const fileInputRef = useRef(null);
  const latestSingleRunIdRef = useRef(0);
  const autoRunKeyRef = useRef("");
  const [symbols, setSymbols] = useState([]);
  const [symbol, setSymbol] = useState("QQQ");
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

  useEffect(() => {
    async function bootstrap() {
      let nextError = "";

      try {
        const planPayload = await fetchDefaultPlan();
        setPlanText(planPayload.plan || "");
      } catch (err) {
        nextError = err.message;
      }

      try {
        const symbolsPayload = await fetchSymbols();
        setSymbols(symbolsPayload.symbols || []);
        if (symbolsPayload.symbols?.includes("QQQ")) {
          setSymbol("QQQ");
        } else if (symbolsPayload.symbols?.length) {
          setSymbol(symbolsPayload.symbols[0]);
        }
      } catch (err) {
        nextError = nextError || err.message;
      }

      if (nextError) {
        setError(nextError);
      }
    }
    bootstrap();
  }, []);

  useEffect(() => {
    if (!symbol) {
      return;
    }
    async function loadBars() {
      try {
        setError("");
        const payload = await fetchBars(symbol, RANGE_FROM, RANGE_TO);
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
  }, [symbol]);

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
      const payload = await validatePlan(symbol, planText);
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

  return (
    <div className="app-shell">
      <header className="hero">
        <div className="hero-copy">
          <div className="eyebrow">Market replay workstation</div>
          <h1>Stock Simulator</h1>
          <p>Click a sell date, replay staged re-entry, compare outcomes.</p>
        </div>
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
      </header>

      {error ? <div className="error-banner">{error}</div> : null}

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
          />
        </section>

        <aside className="sidebar-scroll" aria-label="Simulation controls and results">
          <div className="side-grid">
            <Controls
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

      <input ref={fileInputRef} hidden type="file" accept=".yaml,.yml,.json,.txt" onChange={handlePlanFile} />

      <BatchReportModal
        open={batchOpen}
        result={batchResult}
        selectedRunIndex={selectedRunIndex}
        onSelectRun={setSelectedRunIndex}
        onClose={() => setBatchOpen(false)}
      />
    </div>
  );
}
