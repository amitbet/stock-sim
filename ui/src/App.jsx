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
  const [symbols, setSymbols] = useState([]);
  const [symbol, setSymbol] = useState("QQQ");
  const [bars, setBars] = useState([]);
  const [planText, setPlanText] = useState("");
  const [validation, setValidation] = useState(null);
  const [selectedDate, setSelectedDate] = useState("");
  const [multiSelectedDates, setMultiSelectedDates] = useState([]);
  const [multiSelectEnabled, setMultiSelectEnabled] = useState(false);
  const [executionMode, setExecutionMode] = useState("next_day_open");
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
      try {
        const [symbolsPayload, planPayload] = await Promise.all([fetchSymbols(), fetchDefaultPlan()]);
        setSymbols(symbolsPayload.symbols || []);
        if (symbolsPayload.symbols?.includes("QQQ")) {
          setSymbol("QQQ");
        } else if (symbolsPayload.symbols?.length) {
          setSymbol(symbolsPayload.symbols[0]);
        }
        setPlanText(planPayload.plan || "");
      } catch (err) {
        setError(err.message);
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
        setMultiSelectedDates([]);
        setSingleResult(null);
      } catch (err) {
        setError(err.message);
      }
    }
    loadBars();
  }, [symbol]);

  const selectedBatchCount = multiSelectedDates.length;

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

  async function runSingleForDate(date) {
    if (!date) {
      return;
    }
    setLoading(true);
    try {
      const payload = await runSimulation({
        symbol,
        reference_sell_date: date,
        plan: planText,
        execution_price_mode: executionMode,
        hold_days_after_full_invest: normalizedHoldDaysOverride()
      });
      setSingleResult(payload);
      setError("");
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  async function handleRunSingle() {
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
    setSingleResult(null);
    runSingleForDate(date);
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

        <div className="side-grid">
          <Controls
            symbols={symbols}
            symbol={symbol}
            onSymbolChange={setSymbol}
            executionMode={executionMode}
            onExecutionModeChange={setExecutionMode}
            holdDaysOverride={holdDaysOverride}
            onHoldDaysOverrideChange={setHoldDaysOverride}
            selectedDate={selectedDate}
            multiSelectEnabled={multiSelectEnabled}
            onToggleMultiSelect={handleToggleMultiSelect}
            selectedBatchCount={selectedBatchCount}
            onRunSingle={handleRunSingle}
            onRunBatch={handleRunBatch}
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
