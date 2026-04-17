import { useEffect, useMemo, useRef, useState } from "react";
import { fetchIndustryMA50, fetchStockDetails, parseStockDetailsCsvFile } from "../lib/api.js";

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

function downloadCsv(filename, content) {
  const blob = new Blob([content], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function recordsToCsv(records) {
  const columns = COLUMNS.map((column) => column.key);
  const escape = (value) => {
    const text = value == null ? "" : String(value);
    if (/[",\r\n]/.test(text)) {
      return `"${text.replace(/"/g, '""')}"`;
    }
    return text;
  };

  const lines = [columns.join(",")];
  for (const record of records || []) {
    lines.push(
      columns
        .map((column) => {
          if (column === "industryPercentAboveMA50") {
            return escape(record?.industryPercentAboveMA50);
          }
          return escape(record?.[column]);
        })
        .join(",")
    );
  }
  return `${lines.join("\n")}\n`;
}

function buildProgressStages(sourceLabel, industrySource, tickerCount) {
  const sourceName = INDUSTRY_SOURCES.find((source) => source.value === industrySource)?.label || industrySource;
  const scope = tickerCount > 0 ? `${tickerCount} tickers` : "tickers";
  return [
    `${sourceLabel}: fetching SCTR snapshot for ${scope}...`,
    `${sourceLabel}: matching uploaded tickers against the latest SCTR rankings...`,
    `${sourceLabel}: enriching sector and industry data from ${sourceName}...`,
    `${sourceLabel}: preparing table and industry strength data...`
  ];
}

export default function StockDetailsPanel() {
  const fileInputRef = useRef(null);
  const latestRequestIdRef = useRef(0);
  const lastManualAppliedRef = useRef("");
  const progressTimerRef = useRef(0);
  const ma50RunIdRef = useRef(0);
  const [industrySource, setIndustrySource] = useState("finviz");
  const [manualInput, setManualInput] = useState("");
  const [tickers, setTickers] = useState([]);
  const [records, setRecords] = useState([]);
  const [missingTickers, setMissingTickers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [message, setMessage] = useState("");
  const [progressMessage, setProgressMessage] = useState("");
  const [ma50Status, setMA50Status] = useState("");
  const [lastSourceLabel, setLastSourceLabel] = useState("");
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

  function stopProgressUpdates() {
    if (progressTimerRef.current) {
      window.clearInterval(progressTimerRef.current);
      progressTimerRef.current = 0;
    }
    setProgressMessage("");
  }

  function stopMA50Updates() {
    ma50RunIdRef.current += 1;
    setMA50Status("");
  }

  async function loadIndustryMA50(records, requestId) {
    const industries = Array.from(
      new Set(
        (records || [])
          .map((record) => String(record?.industry || "").trim())
          .filter(Boolean)
      )
    ).filter((industry) => !records.some((record) => String(record?.industry || "").trim() === industry && record?.industryPercentAboveMA50 != null));

    if (industries.length === 0) {
      setMA50Status("");
      return;
    }

    const runId = ma50RunIdRef.current + 1;
    ma50RunIdRef.current = runId;
    let completed = 0;
    setMA50Status(`MA50: loading 0/${industries.length} industries...`);

    const workerCount = Math.min(2, industries.length);
    async function worker(startIndex) {
      for (let index = startIndex; index < industries.length; index += workerCount) {
        const industry = industries[index];
        try {
          const payload = await fetchIndustryMA50({ industry, records });
          if (latestRequestIdRef.current !== requestId || ma50RunIdRef.current !== runId) {
            return;
          }
          if (payload?.ma50) {
            setRecords((current) => current.map((record) => {
              if (String(record?.industry || "").trim() !== industry) {
                return record;
              }
              return {
                ...record,
                industryAboveMA50: payload.ma50.aboveMA,
                industryPercentAboveMA50: payload.ma50.percentAboveMA50
              };
            }));
          }
        } catch {
          if (latestRequestIdRef.current !== requestId || ma50RunIdRef.current !== runId) {
            return;
          }
        }
        completed += 1;
        if (latestRequestIdRef.current !== requestId || ma50RunIdRef.current !== runId) {
          return;
        }
        setMA50Status(
          completed >= industries.length
            ? ""
            : `MA50: loaded ${completed}/${industries.length} industries...`
        );
      }
    }

    await Promise.all(Array.from({ length: workerCount }, (_, index) => worker(index)));
    if (latestRequestIdRef.current === requestId && ma50RunIdRef.current === runId) {
      setMA50Status("");
    }
  }

  function startProgressUpdates(stages) {
    stopProgressUpdates();
    if (!Array.isArray(stages) || stages.length === 0) {
      return;
    }
    let index = 0;
    setProgressMessage(stages[0]);
    progressTimerRef.current = window.setInterval(() => {
      index = Math.min(index + 1, stages.length - 1);
      setProgressMessage(stages[index]);
    }, 1400);
  }

  async function runFetch(tickers, sourceLabel, sourceOverride = industrySource, options = {}) {
    if (!Array.isArray(tickers) || tickers.length === 0) {
      return;
    }
    const background = options.background === true;
    const uniqueTickers = Array.from(new Set(tickers.map((ticker) => String(ticker).trim().toUpperCase()).filter(Boolean)));
    const requestId = latestRequestIdRef.current + 1;
    latestRequestIdRef.current = requestId;
    setTickers(uniqueTickers);
    if (sourceLabel) {
      setLastSourceLabel(sourceLabel);
    }
    stopMA50Updates();
    if (background) {
      setRefreshing(true);
    } else {
      setLoading(true);
      setMessage("");
      startProgressUpdates(buildProgressStages(sourceLabel, sourceOverride, uniqueTickers.length));
    }
    try {
      const payload = await fetchStockDetails({
        tickers: uniqueTickers,
        industrySource: sourceOverride
      });
      if (latestRequestIdRef.current !== requestId) {
        return;
      }
      setRecords(Array.isArray(payload.records) ? payload.records : []);
      setMissingTickers(Array.isArray(payload.missingTickers) ? payload.missingTickers : []);
      void loadIndustryMA50(Array.isArray(payload.records) ? payload.records : [], requestId);
      if (!background) {
        setMessage(`${sourceLabel}: loaded ${Array.isArray(payload.records) ? payload.records.length : 0} records.`);
      }
    } catch (error) {
      if (latestRequestIdRef.current !== requestId) {
        return;
      }
      setMessage(error?.message || String(error));
      if (!background) {
        stopMA50Updates();
        setRecords([]);
        setMissingTickers([]);
      }
    } finally {
      if (latestRequestIdRef.current === requestId) {
        stopProgressUpdates();
        if (background) {
          setRefreshing(false);
        } else {
          setLoading(false);
        }
      }
    }
  }

  async function handleFile(file) {
    if (!file) {
      return;
    }
    setLoading(true);
    setMessage(`Parsing ${file.name}...`);
    startProgressUpdates([
      `CSV: reading ${file.name}...`,
      `CSV: detecting ticker column in ${file.name}...`,
      `CSV: preparing tickers for SCTR fetch...`
    ]);
    try {
      const payload = await parseStockDetailsCsvFile(file);
      const tickers = Array.isArray(payload.tickers) ? payload.tickers : [];
      setMessage(`Parsed ${tickers.length} tickers from ${payload.tickerColumnName || `column ${payload.tickerColumnIndex}`}.`);
      await runFetch(tickers, "CSV");
    } catch (error) {
      setLoading(false);
      stopProgressUpdates();
      setMessage(error?.message || String(error));
    }
  }

  useEffect(() => stopProgressUpdates, []);
  useEffect(() => stopMA50Updates, []);

  function handleManualBlur() {
    const normalized = detectedTickers.join(",");
    if (normalized === lastManualAppliedRef.current) {
      return;
    }
    lastManualAppliedRef.current = normalized;
    if (detectedTickers.length > 0) {
      void runFetch(detectedTickers, "Manual");
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
        <div className="stock-details-header-actions">
          <div className="stock-details-meta">
            <span>{records.length} records</span>
            <span>{missingTickers.length} missing</span>
            <span>{industrySource}</span>
            {lastSourceLabel ? <span>{lastSourceLabel}</span> : null}
            {refreshing ? <span>refreshing</span> : null}
          </div>
          <button
            type="button"
            className="ghost-button"
            disabled={records.length === 0}
            onClick={() => downloadCsv("stock-details.csv", recordsToCsv(sortedRecords))}
          >
            Export CSV
          </button>
        </div>
      </div>

      <div className="stock-details-source">
        <label htmlFor="industry-source">Industry Source</label>
        <select
          id="industry-source"
          value={industrySource}
          onChange={(event) => {
            const nextSource = event.target.value;
            setIndustrySource(nextSource);
            if (tickers.length > 0) {
              void runFetch(tickers, lastSourceLabel, nextSource, { background: true });
            }
          }}
        >
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
            onBlur={handleManualBlur}
            placeholder={"Paste tickers separated by comma, space, or newline.\nExample: AAPL MSFT NVDA"}
          />
          <div className="stock-details-card-row">
            <span>{detectedTickers.length} detected</span>
            <button
              type="button"
              className="primary-button"
              disabled={loading || detectedTickers.length === 0}
              onClick={() => {
                lastManualAppliedRef.current = detectedTickers.join(",");
                void runFetch(detectedTickers, "Manual");
              }}
            >
              {loading ? "Loading..." : "Fetch SCTR"}
            </button>
          </div>
        </div>
      </div>

      {progressMessage ? <div className="stock-details-message stock-details-progress" aria-live="polite">{progressMessage}</div> : null}
      {ma50Status ? <div className="stock-details-message stock-details-progress" aria-live="polite">{ma50Status}</div> : null}
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
