import { useEffect, useMemo, useState } from "react";

export default function Controls({
  symbols,
  symbol,
  onSymbolChange,
  executionMode,
  onExecutionModeChange,
  holdDaysOverride,
  onHoldDaysOverrideChange,
  selectedDate,
  multiSelectEnabled,
  onToggleMultiSelect,
  selectedBatchCount,
  onRunSingle,
  onRunBatch,
  running
}) {
  const [query, setQuery] = useState(symbol);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    setQuery(symbol);
  }, [symbol]);

  const filteredSymbols = useMemo(() => {
    const normalized = query.trim().toUpperCase();
    if (!normalized) {
      return symbols.slice(0, 50);
    }

    const startsWith = [];
    const contains = [];
    for (const item of symbols) {
      const upper = item.toUpperCase();
      if (upper.startsWith(normalized)) {
        startsWith.push(item);
      } else if (upper.includes(normalized)) {
        contains.push(item);
      }
    }
    return [...startsWith, ...contains].slice(0, 50);
  }, [symbols, query]);

  function selectSymbol(nextSymbol) {
    setQuery(nextSymbol);
    setOpen(false);
    onSymbolChange(nextSymbol);
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <h2>Run Settings</h2>
          <p>Select a symbol, choose execution mode, then click candles on the chart.</p>
        </div>
      </div>

      <div className="form-grid">
        <label>
          Symbol
          <div className="symbol-combobox">
            <input
              type="text"
              value={query}
              onChange={(event) => {
                setQuery(event.target.value.toUpperCase());
                setOpen(true);
              }}
              onFocus={() => setOpen(true)}
              onBlur={() => {
                window.setTimeout(() => {
                  setOpen(false);
                  setQuery(symbol);
                }, 120);
              }}
              placeholder="Search ticker"
              className="settings-input"
            />
            {open ? (
              <div className="symbol-menu">
                {filteredSymbols.length > 0 ? (
                  filteredSymbols.map((item) => (
                    <button
                      key={item}
                      type="button"
                      className={`symbol-option ${item === symbol ? "active" : ""}`}
                      onMouseDown={(event) => event.preventDefault()}
                      onClick={() => selectSymbol(item)}
                    >
                      {item}
                    </button>
                  ))
                ) : (
                  <div className="symbol-empty">No matching symbol</div>
                )}
              </div>
            ) : null}
          </div>
        </label>

        <label>
          Execution price
          <select value={executionMode} onChange={(event) => onExecutionModeChange(event.target.value)}>
            <option value="next_day_open">Next day open</option>
            <option value="same_day_close">Same day close</option>
          </select>
        </label>

        <label>
          Hold days override
          <input
            type="number"
            min="0"
            step="1"
            value={holdDaysOverride}
            onChange={(event) => onHoldDaysOverrideChange(event.target.value)}
            placeholder="Auto exit on reclaim"
            className="settings-input"
          />
        </label>
      </div>

      <div className="toggle-row">
        <label className="toggle">
          <input type="checkbox" checked={multiSelectEnabled} onChange={(event) => onToggleMultiSelect(event.target.checked)} />
          <span>Multi-select mode</span>
        </label>
        <span className="muted">
          {multiSelectEnabled
            ? `${selectedBatchCount} dates selected`
            : selectedDate
              ? `Reference date ${selectedDate}`
              : "Pick a reference candle"}
        </span>
      </div>

      <div className="button-row">
        <button type="button" className="primary-button" onClick={onRunSingle} disabled={!selectedDate || running}>
          {running ? "Running..." : "Run Selected Date"}
        </button>
        <button type="button" className="ghost-button" onClick={onRunBatch} disabled={selectedBatchCount === 0 || running}>
          Batch Simulate
        </button>
      </div>
    </section>
  );
}
