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
          <select value={symbol} onChange={(event) => onSymbolChange(event.target.value)}>
            {symbols.map((item) => (
              <option key={item} value={item}>
                {item}
              </option>
            ))}
          </select>
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
