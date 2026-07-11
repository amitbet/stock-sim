export const DEFAULT_BREADTH_SCRIPT = `//@version=6
indicator(title = "ADR + DAR (Only ±3)", shorttitle = "Breadth ±3", format = format.price, precision = 2)

ratio(t1, t2, source) =>
    request.security(t1, timeframe.period, source) / request.security(t2, timeframe.period, source)

adr = ratio("USI:ADVN.NY", "USI:DECL.NY", close)
dar = ratio("USI:DECL.NY", "USI:ADVN.NY", close)

adrPlot = adr >= 3 ? adr : na
darPlot = dar >= 3 ? -dar : na

plot(adrPlot, title="ADR ≥ 3", style=plot.style_histogram, color=color.green, linewidth=6)
plot(darPlot, title="DAR ≥ 3", style=plot.style_histogram, color=color.red, linewidth=6)

hline(0, title="Zero", color=color.gray)
hline(3, title="+3", color=color.white, linestyle=hline.style_solid, linewidth=1)
hline(-3, title="-3", color=color.white, linestyle=hline.style_solid, linewidth=1)
`;

const LEGACY_DEFAULT_NYAD_SCRIPT = `//@version=6
indicator(title = "NYSE Advance-Decline Line (NYAD)", shorttitle = "NYAD", format = format.price, precision = 0)

nyad = request.security("USI:NYAD.NY", timeframe.period, close)
nyadLine = ta.cum(nyad)

plot(nyadLine, title="NYAD", color=color.blue, linewidth=2)
hline(0, title="Zero", color=color.gray)
`;

export const DEFAULT_NYAD_SCRIPT = `//@version=6
indicator(title = "NYSE Advance-Decline Line (NYAD)", shorttitle = "NYAD", format = format.price, precision = 0)

nyad = request.security("USI:NYAD.NY", timeframe.period, close)
nyadLine = ta.cum(nyad)
ema10 = ta.ema(nyadLine, 10)
ema20 = ta.ema(nyadLine, 20)
ema50 = ta.ema(nyadLine, 50)

plot(nyadLine, title="NYAD", color=color.blue, linewidth=2)
plot(ema10, title="EMA 10", color=color.orange, linewidth=2)
plot(ema20, title="EMA 20", color=color.purple, linewidth=2)
plot(ema50, title="EMA 50", color=color.green, linewidth=2)
hline(0, title="Zero", color=color.gray)
`;

const LEGACY_DEFAULT_RSI_SCRIPT = `//@version=6
indicator(title = "Relative Strength Index (RSI)", shorttitle = "RSI", format = format.price, precision = 2)

rsi = ta.rsi(close, 14)
plot(rsi, title="RSI 14", color=color.purple, linewidth=2)
hline(70, title="Overbought", color=color.red)
hline(50, title="Midline", color=color.gray)
hline(30, title="Oversold", color=color.green)
`;

export function upgradeStoredIndicatorScript(script) {
  if (script.id === "nyse-advance-decline-line" && script.source?.trim() === LEGACY_DEFAULT_NYAD_SCRIPT.trim()) {
    return { ...script, source: DEFAULT_NYAD_SCRIPT };
  }
  if (script.id === "relative-strength-index" && script.source?.trim() === LEGACY_DEFAULT_RSI_SCRIPT.trim()) {
    return { ...script, source: DEFAULT_RSI_SCRIPT };
  }
  return script;
}

export const DEFAULT_RSI_SCRIPT = `//@version=6
indicator(title = "Relative Strength Index (RSI)", shorttitle = "RSI", format = format.price, precision = 2)

rsi = ta.rsi(close, 14)
ma14 = ta.sma(rsi, 14)
plot(rsi, title="RSI 14", color=color.purple, linewidth=2)
plot(ma14, title="MA 14", color=color.yellow, linewidth=2)
hline(70, title="Overbought", color=color.red)
hline(50, title="Midline", color=color.gray)
hline(30, title="Oversold", color=color.green)
`;

export const DEFAULT_MACD_SCRIPT = `//@version=6
indicator(title = "Moving Average Convergence Divergence (MACD)", shorttitle = "MACD", format = format.price, precision = 2)

macdLine = ta.ema(close, 12) - ta.ema(close, 26)
signalLine = ta.ema(macdLine, 9)
histogram = macdLine - signalLine

plot(histogram, title="MACD Histogram", style=plot.style_histogram, color=color.gray, linewidth=3)
plot(macdLine, title="MACD", color=color.blue, linewidth=2)
plot(signalLine, title="Signal", color=color.orange, linewidth=2)
hline(0, title="Zero", color=color.gray)
`;

export const DEFAULT_OBV_SCRIPT = `//@version=6
indicator(title = "On-Balance Volume (OBV)", shorttitle = "OBV", format = format.volume, precision = 0)

obv = ta.obv(close, volume)
plot(obv, title="OBV", color=color.teal, linewidth=2)
`;

export const DEFAULT_OBV_SMA14_SCRIPT = `//@version=6
indicator(title="On Balance Volume + SMA 14", shorttitle="OBV SMA14", format=format.volume, timeframe="", timeframe_gaps=true)

var cumVol = 0.
cumVol += nz(volume)

if barstate.islast and cumVol == 0
    runtime.error("No volume is provided by the data vendor.")

src = close
obv = ta.cum(math.sign(ta.change(src)) * volume)

// OBV
plot(obv, color=#2962FF, title="On Balance Volume")

// SMA 14 על OBV
obvSma14 = ta.sma(obv, 14)
plot(obvSma14, color=color.yellow, title="OBV SMA 14", linewidth=1)
`;

export const DEFAULT_INDICATOR_SCRIPTS = [
  {
    id: "breadth-adr-dar-3",
    name: "ADR + DAR ±3",
    visible: true,
    source: DEFAULT_BREADTH_SCRIPT
  },
  {
    id: "nyse-advance-decline-line",
    name: "NYSE Advance-Decline Line (NYAD)",
    visible: false,
    source: DEFAULT_NYAD_SCRIPT
  },
  {
    id: "relative-strength-index",
    name: "Relative Strength Index (RSI)",
    visible: false,
    source: DEFAULT_RSI_SCRIPT
  },
  {
    id: "moving-average-convergence-divergence",
    name: "Moving Average Convergence Divergence (MACD)",
    visible: false,
    source: DEFAULT_MACD_SCRIPT
  },
  {
    id: "on-balance-volume",
    name: "On-Balance Volume (OBV)",
    visible: false,
    source: DEFAULT_OBV_SCRIPT
  },
  {
    id: "on-balance-volume-sma-14",
    name: "On Balance Volume + SMA 14",
    visible: false,
    source: DEFAULT_OBV_SMA14_SCRIPT
  }
];

const COLORS = {
  "color.green": "#22c55e",
  "color.red": "#ef4444",
  "color.blue": "#38bdf8",
  "color.orange": "#f59e0b",
  "color.purple": "#a78bfa",
  "color.teal": "#14b8a6",
  "color.yellow": "#facc15",
  "color.gray": "#94a3b8",
  "color.white": "#f8fafc"
};

function splitTopLevel(value, separator) {
  const parts = [];
  let current = "";
  let depth = 0;
  let quote = "";
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index];
    if (quote) {
      current += char;
      if (char === quote && value[index - 1] !== "\\") quote = "";
    } else if (char === '"' || char === "'") {
      quote = char;
      current += char;
    } else {
      if (char === "(") depth += 1;
      if (char === ")") depth -= 1;
      if (char === separator && depth === 0) {
        parts.push(current.trim());
        current = "";
      } else {
        current += char;
      }
    }
  }
  parts.push(current.trim());
  return parts;
}

function splitTernary(expression) {
  let depth = 0;
  let quote = "";
  let question = -1;
  for (let index = 0; index < expression.length; index += 1) {
    const char = expression[index];
    if (quote) {
      if (char === quote && expression[index - 1] !== "\\") quote = "";
      continue;
    }
    if (char === '"' || char === "'") quote = char;
    else if (char === "(") depth += 1;
    else if (char === ")") depth -= 1;
    else if (char === "?" && depth === 0) { question = index; break; }
  }
  if (question < 0) return null;
  depth = 0;
  quote = "";
  for (let index = question + 1; index < expression.length; index += 1) {
    const char = expression[index];
    if (quote) {
      if (char === quote && expression[index - 1] !== "\\") quote = "";
      continue;
    }
    if (char === '"' || char === "'") quote = char;
    else if (char === "(") depth += 1;
    else if (char === ")") depth -= 1;
    else if (char === ":" && depth === 0) {
      return {
        condition: expression.slice(0, question).trim(),
        whenTrue: expression.slice(question + 1, index).trim(),
        whenFalse: expression.slice(index + 1).trim()
      };
    }
  }
  throw new Error("Unsupported ternary expression.");
}

function splitBinary(expression, operator) {
  let depth = 0;
  let quote = "";
  for (let index = expression.length - 1; index >= 0; index -= 1) {
    const char = expression[index];
    if (quote) {
      if (char === quote && expression[index - 1] !== "\\") quote = "";
      continue;
    }
    if (char === '"' || char === "'") quote = char;
    else if (char === ")") depth += 1;
    else if (char === "(") depth -= 1;
    else if (char === operator && depth === 0) {
      return [expression.slice(0, index).trim(), expression.slice(index + 1).trim()];
    }
  }
  return null;
}

function normalizeSymbol(value) {
  return String(value || "").trim().toUpperCase();
}

export function extractRequestedSymbols(source) {
  const symbols = new Set();
  for (const regex of [
    /request\.security\(\s*["']([^"']+)["']/g,
    /ratio\(\s*["']([^"']+)["']\s*,\s*["']([^"']+)["']/g
  ]) {
    let match = regex.exec(source);
    while (match) {
      symbols.add(normalizeSymbol(match[1]));
      if (match[2]) symbols.add(normalizeSymbol(match[2]));
      match = regex.exec(source);
    }
  }
  return [...symbols];
}

export function indicatorTitle(source, fallback = "Custom Script") {
  return source.match(/indicator\(\s*title\s*=\s*["']([^"']+)["']/)?.[1] || fallback;
}

function namedArgs(body) {
  const result = { positional: [] };
  for (const arg of splitTopLevel(body, ",")) {
    const index = arg.indexOf("=");
    if (index > 0) result[arg.slice(0, index).trim()] = arg.slice(index + 1).trim().replace(/^["']|["']$/g, "");
    else if (arg) result.positional.push(arg);
  }
  return result;
}

function alignSeries(externalSeries, localBars) {
  const entries = Object.entries(externalSeries || {});
  const local = (localBars || [])
    .map((bar) => ({
      date: String(bar.date).slice(0, 10),
      close: Number(bar.close),
      volume: Number(bar.volume || 0)
    }))
    .filter((bar) => bar.date && Number.isFinite(bar.close))
    .sort((left, right) => left.date.localeCompare(right.date));
  let dates = entries.length === 0 ? local.map((bar) => bar.date) : null;
  const bySymbol = {};
  for (const [symbol, rows] of entries) {
    const normalized = rows
      .map((row) => ({ date: String(row.date).slice(0, 10), value: Number(row.close) }))
      .filter((row) => row.date && Number.isFinite(row.value));
    bySymbol[normalizeSymbol(symbol)] = normalized;
    const available = new Set(normalized.map((row) => row.date));
    dates = dates ? dates.filter((date) => available.has(date)) : normalized.map((row) => row.date);
  }
  return { dates: [...(dates || [])].sort(), bySymbol, local };
}

function ema(series, period) {
  const alpha = 2 / (period + 1);
  let previous = null;
  return series.map((row) => {
    if (row.value == null) return { date: row.date, value: null };
    previous = previous == null ? row.value : alpha * row.value + (1 - alpha) * previous;
    return { date: row.date, value: previous };
  });
}

function rsi(series, period) {
  let averageGain = 0;
  let averageLoss = 0;
  return series.map((row, index) => {
    if (index === 0 || row.value == null || series[index - 1]?.value == null) return { date: row.date, value: null };
    const change = row.value - series[index - 1].value;
    const gain = Math.max(change, 0);
    const loss = Math.max(-change, 0);
    if (index <= period) {
      averageGain += gain / period;
      averageLoss += loss / period;
      if (index < period) return { date: row.date, value: null };
    } else {
      averageGain = ((averageGain * (period - 1)) + gain) / period;
      averageLoss = ((averageLoss * (period - 1)) + loss) / period;
    }
    const value = averageGain === 0 && averageLoss === 0
      ? 50
      : averageLoss === 0 ? 100 : 100 - (100 / (1 + averageGain / averageLoss));
    return { date: row.date, value };
  });
}

function obv(close, volume) {
  let total = 0;
  return close.map((row, index) => {
    if (index > 0 && row.value != null && close[index - 1]?.value != null) {
      if (row.value > close[index - 1].value) total += volume[index]?.value || 0;
      else if (row.value < close[index - 1].value) total -= volume[index]?.value || 0;
    }
    return { date: row.date, value: total };
  });
}

function sma(series, period) {
  return series.map((row, index) => {
    const window = series.slice(Math.max(0, index - period + 1), index + 1).map((item) => item.value);
    if (window.length < period || window.some((value) => value == null)) return { date: row.date, value: null };
    return { date: row.date, value: window.reduce((sum, value) => sum + value, 0) / period };
  });
}

function expression(value, context) {
  const expr = value.trim();
  const ternary = splitTernary(expr);
  if (ternary) {
    const matches = condition(ternary.condition, context);
    const yes = expression(ternary.whenTrue, context);
    const no = expression(ternary.whenFalse, context);
    return context.dates.map((date, index) => ({ date, value: matches[index] ? yes[index]?.value ?? null : no[index]?.value ?? null }));
  }
  if (expr === "na") return context.dates.map((date) => ({ date, value: null }));
  if (/^-?\d+(?:\.\d+)?$/.test(expr)) return context.dates.map((date) => ({ date, value: Number(expr) }));
  if (expr.startsWith("-")) return expression(expr.slice(1), context).map((row) => ({ ...row, value: row.value == null ? null : -row.value }));

  const cumulative = expr.match(/^ta\.cum\((.+)\)$/);
  if (cumulative) {
    let total = 0;
    return expression(cumulative[1], context).map((row) => {
      if (row.value != null) total += row.value;
      return { date: row.date, value: row.value == null ? null : total };
    });
  }

  const multiplication = splitBinary(expr, "*");
  if (multiplication) return multiply(expression(multiplication[0], context), expression(multiplication[1], context));

  const change = expr.match(/^ta\.change\((.+)\)$/);
  if (change) {
    const input = expression(change[1], context);
    return input.map((row, index) => ({
      date: row.date,
      value: index === 0 || row.value == null || input[index - 1]?.value == null ? null : row.value - input[index - 1].value
    }));
  }

  const sign = expr.match(/^math\.sign\((.+)\)$/);
  if (sign) return expression(sign[1], context).map((row) => ({ ...row, value: row.value == null ? null : Math.sign(row.value) }));

  const subtraction = splitBinary(expr, "-");
  if (subtraction) return subtract(expression(subtraction[0], context), expression(subtraction[1], context));

  const taCall = expr.match(/^ta\.(rsi|ema|sma|obv)\((.*)\)$/);
  if (taCall) {
    const args = splitTopLevel(taCall[2], ",");
    if (taCall[1] === "obv" && args.length === 2) return obv(expression(args[0], context), expression(args[1], context));
    const period = Number(args[1]);
    if (!Number.isInteger(period) || period <= 0) throw new Error(`Invalid ${taCall[1]} period.`);
    const input = expression(args[0], context);
    if (taCall[1] === "rsi") return rsi(input, period);
    return taCall[1] === "sma" ? sma(input, period) : ema(input, period);
  }

  const ratio = expr.match(/^ratio\(\s*["']([^"']+)["']\s*,\s*["']([^"']+)["']\s*,\s*close\s*\)$/);
  if (ratio) return divide(security(ratio[1], context), security(ratio[2], context));
  const request = expr.match(/^request\.security\(\s*["']([^"']+)["']\s*,\s*timeframe\.period\s*,\s*close\s*\)$/);
  if (request) return security(request[1], context);
  const division = splitBinary(expr, "/");
  if (division) return divide(expression(division[0], context), expression(division[1], context));
  if (context.variables[expr]) return context.variables[expr];
  throw new Error(`Unsupported script expression: ${expr}`);
}

function security(symbol, context) {
  const rows = context.bySymbol[normalizeSymbol(symbol)];
  if (!rows) throw new Error(`Missing data for ${normalizeSymbol(symbol)}.`);
  const lookup = new Map(rows.map((row) => [row.date, row.value]));
  return context.dates.map((date) => ({ date, value: lookup.get(date) ?? null }));
}

function divide(left, right) {
  return left.map((row, index) => ({ date: row.date, value: row.value == null || !right[index]?.value ? null : row.value / right[index].value }));
}

function subtract(left, right) {
  return left.map((row, index) => ({
    date: row.date,
    value: row.value == null || right[index]?.value == null ? null : row.value - right[index].value
  }));
}

function multiply(left, right) {
  return left.map((row, index) => ({
    date: row.date,
    value: row.value == null || right[index]?.value == null ? null : row.value * right[index].value
  }));
}

function condition(value, context) {
  const match = value.match(/^(.+?)\s*(>=|<=|>|<)\s*(.+)$/);
  if (!match) throw new Error(`Unsupported condition: ${value}`);
  const left = expression(match[1], context);
  const right = expression(match[3], context);
  return left.map((row, index) => {
    const a = row.value;
    const b = right[index]?.value;
    if (a == null || b == null) return false;
    if (match[2] === ">=") return a >= b;
    if (match[2] === "<=") return a <= b;
    if (match[2] === ">") return a > b;
    return a < b;
  });
}

export function evaluateIndicatorScript(source, externalSeries, localBars = []) {
  const { dates, bySymbol, local } = alignSeries(externalSeries, localBars);
  if (source.includes("No volume is provided by the data vendor.") && local.length > 0 && local.every((bar) => bar.volume === 0)) {
    throw new Error("No volume is provided by the data vendor.");
  }
  const localByDate = new Map(local.map((bar) => [bar.date, bar]));
  const context = {
    dates,
    bySymbol,
    variables: {
      close: dates.map((date) => ({ date, value: localByDate.get(date)?.close ?? null })),
      volume: dates.map((date) => ({ date, value: localByDate.get(date)?.volume ?? null }))
    }
  };
  const plots = [];
  const hlines = [];
  for (const raw of source.split(/\r?\n/)) {
    const line = raw.replace(/\/\/.*$/, "").trim();
    if (!line || line.startsWith("@") || line.includes("=>") || line.startsWith("indicator(")) continue;
    if (line.startsWith("plot(")) {
      const args = namedArgs(line.slice(5, line.lastIndexOf(")")));
      const name = args.positional[0];
      const data = context.variables[name];
      if (!data) throw new Error(`Plot references unknown series: ${name}`);
      plots.push({
        id: `${name}-${plots.length}`,
        title: args.title || name,
        type: args.style === "plot.style_histogram" ? "histogram" : "line",
        color: COLORS[args.color] || args.color || "#38bdf8",
        lineWidth: Number(args.linewidth) || 2,
        data: data.filter((row) => row.value != null && Number.isFinite(row.value)).map((row) => ({ time: row.date, value: Number(row.value.toFixed(4)) }))
      });
      continue;
    }
    if (line.startsWith("hline(")) {
      const args = namedArgs(line.slice(6, line.lastIndexOf(")")));
      const price = Number(args.positional[0]);
      if (Number.isFinite(price)) hlines.push({ price, title: args.title || String(price), color: COLORS[args.color] || args.color || "#94a3b8", lineWidth: Number(args.linewidth) || 1 });
      continue;
    }
    const assignment = line.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+)$/);
    if (assignment) context.variables[assignment[1]] = expression(assignment[2], context);
  }
  return { title: indicatorTitle(source), requestedSymbols: extractRequestedSymbols(source), plots, hlines, barsLoaded: dates.length };
}
