function summaryItem(label, value) {
  return (
    <div className="summary-item" key={label}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export default function ResultsPanel({ result }) {
  const actions = result?.actions ?? [];

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <h2>Run Results</h2>
          <p>Single-date simulation summary with the full action ledger.</p>
        </div>
      </div>

      {result ? (
        <>
          <div className="summary-grid">
            {summaryItem("Reference", result.summary.reference_sell_date)}
            {summaryItem("S Price", result.summary.reference_price.toFixed(2))}
            {summaryItem("Full Invest", result.summary.full_invest_date || "Not reached")}
            {summaryItem("End Date", result.summary.end_date)}
            {summaryItem("Gain %", `${result.summary.gain_pct.toFixed(2)}%`)}
            {summaryItem("Invested", `${result.summary.total_invested_pct.toFixed(2)}%`)}
            {summaryItem("Max Drawdown", `${result.stats.max_drawdown_pct.toFixed(2)}%`)}
          </div>

          <div className="table-scroll">
            <table className="ledger-table">
              <thead>
                <tr>
                  <th>Trigger Date</th>
                  <th>Trigger Price</th>
                  <th>Date</th>
                  <th>Rule</th>
                  <th>Trigger Reason</th>
                  <th>Allocation</th>
                  <th>Buy Price</th>
                  <th>Notes</th>
                </tr>
              </thead>
              <tbody>
                {actions.map((action) => (
                  <tr key={`${action.date}-${action.trigger_id}`}>
                    <td>{action.trigger_date}</td>
                    <td>{action.trigger_price != null ? action.trigger_price.toFixed(2) : "--"}</td>
                    <td>{action.date}</td>
                    <td>{action.label || action.trigger_id}</td>
                    <td>{action.trigger_reason}</td>
                    <td>{action.allocation_pct}%</td>
                    <td>{action.fill_price.toFixed(2)}</td>
                    <td>{action.notes}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      ) : (
        <div className="empty-state">Run a simulation to see the result summary and trade ledger here.</div>
      )}
    </section>
  );
}
