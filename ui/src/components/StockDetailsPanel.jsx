import { useMemo, useRef, useState } from "react";
import { fetchStockDetails, parseStockDetailsCsvFile } from "../lib/api.js";

const INDUSTRY_SOURCES = [
  { value: "finviz", label: "Finviz", description: "Finviz industry definitions with cached scraping." },
  { value: "stockcharts", label: "StockCharts", description: "Use the classifications embedded in StockCharts SCTR." },
  { value: "yahoo", label: "Yahoo Finance", description: "Yahoo asset profile sector and industry data." }
];

const COLUMNS = [
  { key: "date", label: "Date" },
  { key: "symbol", label: "Symbol" },
  { key: "earningsReportDate", label: "Earnings" },
  { key: "name", label: "Name" },
  { key: "SCTR", label: "SCTR", numeric: true },
  { key: "industryRS", label: "Ind RS", numeric: true, suffix: "%" },
  { key: "sectorRS", label: "Sec RS", numeric: true, suffix: "%" },
  { key: "industryPercentAboveMA50", label: "Ind vs MA50", numeric: true, suffix: "%" },
  { key: "delta", label: "Delta", numeric: true },
  { key: "close", label: "Close", numeric: true },
  { key: "marketCap", label: "MktCap", numeric: true },
  { key: "vol", label: "Vol", numeric: true, integer: true },
  { key: "industry", label: "Industry" },
  { key: "sector", label: "Sector" }
];

function extractTickers(text) {
  return Array.from(
    new Set(
      String(text || "")
        .split(/[\s,;\t\r\n]+/g)
        .map((value) => value.trim().replace(/^"+|"+$/g, "").toUpperCase())
        .filter(Boolean)
    )
  );
}

function compareValues(a, b) {
  if (a == null && b == null) {
    return 0;
  }
  if (a == null) {
    return 1;
  }
  if (b == null) {
    return -1;
  }
  if (typeof a === "number" && typeof b === "number") {
    return a - b;
  }
  return String(a).localeCompare(String(b));
}

function formatValue(value, options = {}) {
  if (value == null || value === "") {
    return "—";
  }
  if (typeof value === "boolean") {
    return value ? "Yes" : "No";
  }
  const numberValue = Number(value);
  if (!Number.isNaN(numberValue) && Number.isFinite(numberValue)) {
    if (options.integer) {
      return Math.trunc(numberValue).toLocaleString();
    }
    const precision = options.precision ?? 1;
    return `${numberValue.toFixed(precision)}${options.suffix || ""}`;
  }
  return String(value);
}

export default function StockDetailsPanel() {
  const fileInputRef = useRef(null);
  const [industrySource, setIndustrySource] = useState("finviz");
  const [manualInput, setManualInput] = useState("");
  const [records, setRecords] = useState([]);
  const [missingTickers, setMissingTickers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");
  const [sortKey, setSortKey] = useState("SCTR");
  const [sortDir, setSortDir] = useState("desc");
  const [dragOver, setDragOver] = useState(false);
  const detectedTickers = useMemo(() => extractTickers(manualInput), [manualInput]);

  const sortedRecords = useMemo(() => {
    const copy = [...records];
    copy.sort((left, right) => {
      const result = compareValues(left?.[sortKey], right?.[sortKey]);
      return sortDir === "asc" ? result : -result;
    });
    return copy;
  }, [records, sortDir, sortKey]);

  function toggleSort(nextKey) {
    if (sortKey === nextKey) {
      setSortDir((value) => (value === "asc" ? "desc" : "asc"));
      return;
    }
    setSortKey(nextKey);
    setSortDir(nextKey === "SCTR" || nextKey === "industryRS" || nextKey === "sectorRS" ? "desc" : "asc");
  }

  async function runFetch(tickers, sourceLabel) {
    if (!Array.isArray(tickers) || tickers.length === 0) {
      return;
    }
    setLoading(true);
    setMessage("");
    try {
      const payload = await fetchStockDetails({
        tickers,
        industrySource
      });
      setRecords(Array.isArray(payload.records) ? payload.records : []);
      setMissingTickers(Array.isArray(payload.missingTickers) ? payload.missingTickers : []);
      setMessage(`${sourceLabel}: loaded ${Array.isArray(payload.records) ? payload.records.length : 0} records.`);
    } catch (error) {
      setMessage(error?.message || String(error));
      setRecords([]);
      setMissingTickers([]);
    } finally {
      setLoading(false);
    }
  }

  async function handleFile(file) {
    if (!file) {
      return;
    }
    setLoading(true);
    setMessage(`Parsing ${file.name}...`);
    try {
      const payload = await parseStockDetailsCsvFile(file);
      const tickers = Array.isArray(payload.tickers) ? payload.tickers : [];
      setMessage(`Parsed ${tickers.length} tickers from ${payload.tickerColumnName || `column ${payload.tickerColumnIndex}`}.`);
      await runFetch(tickers, "CSV");
    } catch (error) {
      setLoading(false);
      setMessage(error?.message || String(error));
    }
  }

  return (
    <section className="stock-details-shell" aria-label="Stock details workspace">
      <div className="stock-details-header">
        <div>
          <div className="stock-details-kicker">Ticker ranking</div>
          <h2>Stock Details</h2>
          <p>Fetch SCTR rankings, enrich sector and industry classifications, and compare industry strength.</p>
        </div>
        <div className="stock-details-meta">
          <span>{records.length} records</span>
          <span>{missingTickers.length} missing</span>
          <span>{industrySource}</span>
        </div>
      </div>

      <div className="stock-details-source">
        <label htmlFor="industry-source">Industry Source</label>
        <select id="industry-source" value={industrySource} onChange={(event) => setIndustrySource(event.target.value)}>
          {INDUSTRY_SOURCES.map((source) => (
            <option key={source.value} value={source.value}>
              {source.label}
            </option>
          ))}
        </select>
        <p>{INDUSTRY_SOURCES.find((source) => source.value === industrySource)?.description}</p>
      </div>

      <div className="stock-details-entry-grid">
        <div className="stock-details-card">
          <div className="stock-details-card-title">CSV Upload</div>
          <button type="button" className={`stock-details-dropzone${dragOver ? " active" : ""}`} onClick={() => fileInputRef.current?.click()}
            onDragEnter={(event) => {
              event.preventDefault();
              setDragOver(true);
            }}
            onDragOver={(event) => {
              event.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={(event) => {
              event.preventDefault();
              setDragOver(false);
            }}
            onDrop={(event) => {
              event.preventDefault();
              setDragOver(false);
              void handleFile(event.dataTransfer?.files?.[0]);
            }}
          >
            <strong>Drop a CSV here</strong>
            <span>We’ll detect the ticker column and fetch SCTR automatically.</span>
          </button>
          <input
            ref={fileInputRef}
            hidden
            type="file"
            accept=".csv,text/csv,application/vnd.ms-excel"
            onChange={(event) => void handleFile(event.target.files?.[0])}
          />
        </div>

        <div className="stock-details-card">
          <div className="stock-details-card-title">Manual Input</div>
          <textarea
            className="stock-details-textarea"
            value={manualInput}
            onChange={(event) => setManualInput(event.target.value)}
            placeholder={"Paste tickers separated by comma, space, or newline.\nExample: AAPL MSFT NVDA"}
          />
          <div className="stock-details-card-row">
            <span>{detectedTickers.length} detected</span>
            <button
              type="button"
              className="primary-button"
              disabled={loading || detectedTickers.length === 0}
              onClick={() => void runFetch(detectedTickers, "Manual")}
            >
              {loading ? "Loading..." : "Fetch SCTR"}
            </button>
          </div>
        </div>
      </div>

      {message ? <div className={`stock-details-message${message.toLowerCase().includes("error") ? " error" : ""}`}>{message}</div> : null}
      {missingTickers.length > 0 ? (
        <div className="stock-details-message warning">
          Missing tickers: {missingTickers.join(", ")}
        </div>
      ) : null}

      <div className="stock-details-table-wrap">
        {sortedRecords.length === 0 && !loading ? (
          <div className="empty-state">No stock details yet. Upload a CSV or paste tickers to begin.</div>
        ) : null}

        {sortedRecords.length > 0 ? (
          <div className="table-scroll">
            <table className="ledger-table stock-details-table">
              <thead>
                <tr>
                  {COLUMNS.map((column) => (
                    <th key={column.key}>
                      <button type="button" className="stock-details-sort" onClick={() => toggleSort(column.key)}>
                        <span>{column.label}</span>
                        {sortKey === column.key ? <span>{sortDir === "asc" ? "▲" : "▼"}</span> : null}
                      </button>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {sortedRecords.map((record) => (
                  <tr key={record.symbol}>
                    {COLUMNS.map((column) => {
                      if (column.key === "industryPercentAboveMA50") {
                        const prefix = record.industryAboveMA50 == null ? "" : record.industryAboveMA50 ? "↑ " : "↓ ";
                        const content = record.industryPercentAboveMA50 == null
                          ? "—"
                          : `${prefix}${Math.abs(record.industryPercentAboveMA50).toFixed(1)}%`;
                        return <td key={column.key} className={column.numeric ? "numeric-cell" : ""}>{content}</td>;
                      }
                      const value = record[column.key];
                      return (
                        <td key={column.key} className={column.numeric ? "numeric-cell" : ""}>
                          {formatValue(value, {
                            integer: column.integer,
                            precision: column.key === "close" || column.key === "marketCap" ? 2 : 1,
                            suffix: column.suffix
                          })}
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </div>
    </section>
  );
}
