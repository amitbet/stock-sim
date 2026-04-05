import { useMemo, useState } from "react";

function summaryItem(label, value) {
  return (
    <div className="summary-item" key={label}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function formatMoney(value) {
  return value != null ? value.toFixed(2) : "--";
}

export default function ResultsPanel({ result }) {
  const actions = result?.actions ?? [];
  const pendingTriggers = result?.pending_triggers ?? [];
  const [cashAmount, setCashAmount] = useState("");

  const parsedCashAmount = useMemo(() => {
    const trimmed = String(cashAmount).trim();
    if (!trimmed) {
      return 0;
    }
    const parsed = Number.parseFloat(trimmed);
    return Number.isNaN(parsed) ? 0 : parsed;
  }, [cashAmount]);

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

          <div className="pending-triggers-section">
            <div className="panel-header pending-triggers-header">
              <div>
                <h3>Unbought Triggers</h3>
                <p>Projected entries that did not execute in this run. Cash to invest is based on the amount you enter below.</p>
              </div>
              <label className="pending-cash-input">
                <span>Cash Amount</span>
                <input
                  type="number"
                  min="0"
                  step="0.01"
                  inputMode="decimal"
                  className="settings-input"
                  value={cashAmount}
                  onChange={(event) => setCashAmount(event.target.value)}
                  placeholder="10000"
                />
              </label>
            </div>

            <div className="table-scroll">
              <table className="ledger-table">
                <thead>
                  <tr>
                    <th>Rule</th>
                    <th>Trigger</th>
                    <th>S-Based Trigger Price</th>
                    <th>Buy Price</th>
                    <th>Invest %</th>
                    <th>Cash To Invest</th>
                  </tr>
                </thead>
                <tbody>
                  {pendingTriggers.length > 0 ? (
                    pendingTriggers.map((trigger) => (
                      <tr key={trigger.trigger_id}>
                        <td>{trigger.label || trigger.trigger_id}</td>
                        <td>{trigger.trigger_reason}</td>
                        <td>{formatMoney(trigger.trigger_price)}</td>
                        <td>{formatMoney(trigger.buy_price)}</td>
                        <td>{trigger.cash_to_invest_pct.toFixed(2)}%</td>
                        <td>{formatMoney((parsedCashAmount * trigger.cash_to_invest_pct) / 100)}</td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan="6" className="empty-table-cell">
                        No unbought triggers remain for this run.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </>
      ) : (
        <div className="empty-state">Run a simulation to see the result summary and trade ledger here.</div>
      )}
    </section>
  );
}
