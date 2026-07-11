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

export const DEFAULT_INDICATOR_SCRIPTS = [{
  id: "breadth-adr-dar-3",
  name: "ADR + DAR ±3",
  visible: true,
  source: DEFAULT_BREADTH_SCRIPT
}];

const COLORS = {
  "color.green": "#22c55e",
  "color.red": "#ef4444",
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

function alignSeries(externalSeries) {
  const entries = Object.entries(externalSeries || {});
  let dates = null;
  const bySymbol = {};
  for (const [symbol, rows] of entries) {
    const normalized = rows
      .map((row) => ({ date: String(row.date).slice(0, 10), value: Number(row.close) }))
      .filter((row) => row.date && Number.isFinite(row.value));
    bySymbol[normalizeSymbol(symbol)] = normalized;
    const available = new Set(normalized.map((row) => row.date));
    dates = dates ? dates.filter((date) => available.has(date)) : normalized.map((row) => row.date);
  }
  return { dates: [...(dates || [])].sort(), bySymbol };
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

export function evaluateIndicatorScript(source, externalSeries) {
  const { dates, bySymbol } = alignSeries(externalSeries);
  const context = { dates, bySymbol, variables: {} };
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
