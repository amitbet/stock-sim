export default function BatchReportModal({ open, result, selectedRunIndex, onSelectRun, onClose }) {
  if (!open || !result) {
    return null;
  }

  const run = result.runs[selectedRunIndex] || result.runs[0];

  return (
    <div className="modal-backdrop" role="dialog" aria-modal="true">
      <div className="modal-card">
        <div className="panel-header">
          <div>
            <h2>Batch Simulation Report</h2>
            <p>Compare multiple reference sell dates, then inspect a single run in detail.</p>
          </div>
          <button type="button" className="ghost-button" onClick={onClose}>
            Close
          </button>
        </div>

        <div className="modal-grid">
          <div className="modal-table-pane">
            <table className="ledger-table">
              <thead>
                <tr>
                  <th>Reference</th>
                  <th>Full Invest</th>
                  <th>End</th>
                  <th>Gain %</th>
                  <th>Mode</th>
                  <th>Actions</th>
                  <th>Invested</th>
                </tr>
              </thead>
              <tbody>
                {result.runs.map((item, index) => (
                  <tr
                    key={item.summary.reference_sell_date}
                    className={index === selectedRunIndex ? "selected-row" : ""}
                    onClick={() => onSelectRun(index)}
                  >
                    <td>{item.summary.reference_sell_date}</td>
                    <td>{item.summary.full_invest_date || "Not reached"}</td>
                    <td>{item.summary.end_date}</td>
                    <td>{item.summary.gain_pct.toFixed(2)}%</td>
                    <td>{item.summary.execution_mode}</td>
                    <td>{item.actions.length}</td>
                    <td>{item.summary.total_invested_pct.toFixed(2)}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="detail-panel">
            <h3>Drilldown</h3>
            <div className="summary-grid compact">
              <div className="summary-item">
                <span>Reference</span>
                <strong>{run.summary.reference_sell_date}</strong>
              </div>
              <div className="summary-item">
                <span>Gain</span>
                <strong>{run.summary.gain_pct.toFixed(2)}%</strong>
              </div>
              <div className="summary-item">
                <span>Max DD</span>
                <strong>{run.stats.max_drawdown_pct.toFixed(2)}%</strong>
              </div>
            </div>
            <ul className="action-list">
              {run.actions.map((action) => (
                <li key={`${action.date}-${action.trigger_id}`}>
                  <strong>{action.date}</strong> {action.label || action.trigger_id} bought {action.allocation_pct}% at {action.fill_price.toFixed(2)}
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
