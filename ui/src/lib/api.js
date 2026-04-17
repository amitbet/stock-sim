let apiBaseCache;
let apiBaseReady;
let apiHealthReady;
const REQUEST_TIMEOUT_MS = 15_000;
const STARTUP_REQUEST_TIMEOUT_MS = 60_000;
const RECOVERY_TIMEOUT_MS = 45_000;
const RECOVERY_POLL_MS = 750;
const STOCK_DETAILS_TIMEOUT_MS = 120_000;
const CSV_PARSE_TIMEOUT_MS = 120_000;
const API_READY_TIMEOUT_MS = 60_000;
const API_READY_POLL_MS = 500;

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function clearApiBaseCache() {
  apiBaseCache = undefined;
  apiBaseReady = undefined;
  apiHealthReady = undefined;
}

async function waitForWailsBindings() {
  for (let i = 0; i < 80; i++) {
    if (typeof window !== "undefined" && window.go?.main?.App) {
      return true;
    }
    await new Promise((r) => setTimeout(r, 25));
  }
  return typeof window !== "undefined" && !!window.go?.main?.App;
}

async function resolveApiBase(options = {}) {
  const forceRefresh = options.forceRefresh === true;
  if (forceRefresh) {
    clearApiBaseCache();
  }
  if (apiBaseCache !== undefined) {
    return apiBaseCache;
  }
  if (!apiBaseReady) {
    apiBaseReady = (async () => {
      let hasWails = typeof window !== "undefined" && !!window.go?.main?.App;
      // In Wails dev the webview loads Vite (same as browser dev). If window.go is not ready yet, wait
      // only when Wails injected window.runtime — otherwise we'd cache "" and miss GetAPIBaseURL() for
      // SIM_ADDR / 127.0.0.1:0 (dynamic port). Pure browser dev has no window.runtime, so no delay.
      const wailsChrome =
        typeof window !== "undefined" && typeof window.runtime !== "undefined";
      if (import.meta.env.DEV && !hasWails && wailsChrome) {
        hasWails = await waitForWailsBindings();
      }
      // Browser-only dev (npm run dev + cmd/server): Vite proxies /api → localhost:3002 (vite.config.js).
      // Makefile sets SIM_ADDR=127.0.0.1:3002 for dev-api; no dynamic port here.
      if (import.meta.env.DEV && !hasWails) {
        apiBaseCache = "";
        return;
      }
      if (!hasWails) {
        hasWails = await waitForWailsBindings();
      }
      if (!hasWails) {
        apiBaseCache = "";
        return;
      }
      const { GetAPIBaseURL } = await import("../../wailsjs/go/main/App.js");
      let u = await GetAPIBaseURL();
      for (let i = 0; i < 80 && !u; i++) {
        await new Promise((r) => setTimeout(r, 25));
        u = await GetAPIBaseURL();
      }
      apiBaseCache = u ? String(u).replace(/\/+$/, "") : "";
    })();
  }
  await apiBaseReady;
  return apiBaseCache;
}

function joinApi(path) {
  const p = path.startsWith("/") ? path : `/${path}`;
  const base = apiBaseCache || "";
  if (!base) {
    return p;
  }
  return `${base}${p}`;
}

function timeoutForPath(path, explicitTimeoutMs) {
  if (explicitTimeoutMs != null) {
    return explicitTimeoutMs;
  }
  if (typeof path === "string" && path.startsWith("/api/stock-details/")) {
    return STOCK_DETAILS_TIMEOUT_MS;
  }
  if (
    path === "/api/data-sources" ||
    path === "/api/default-plan" ||
    (typeof path === "string" && path.startsWith("/api/symbols?")) ||
    (typeof path === "string" && path.startsWith("/api/symbol-info?")) ||
    (typeof path === "string" && path.startsWith("/api/bars?"))
  ) {
    return STARTUP_REQUEST_TIMEOUT_MS;
  }
  return REQUEST_TIMEOUT_MS;
}

function shouldWaitForApiReady() {
  return typeof window !== "undefined" && !!window.go?.main?.App;
}

async function ensureApiReady(options = {}) {
  if (!shouldWaitForApiReady()) {
    return;
  }
  const forceRefresh = options.forceRefresh === true;
  if (forceRefresh) {
    apiHealthReady = undefined;
  }
  if (apiHealthReady) {
    return apiHealthReady;
  }
  apiHealthReady = (async () => {
    const deadline = Date.now() + API_READY_TIMEOUT_MS;
    let lastError;

    while (Date.now() < deadline) {
      try {
        const base = await resolveApiBase({ forceRefresh });
        const healthURL = base ? `${base}/api/health` : "/api/health";
        const healthResponse = await fetchWithTimeout(healthURL, {
          method: "GET",
          timeoutMs: Math.min(5_000, REQUEST_TIMEOUT_MS)
        });
        if (!healthResponse.ok) {
          throw new Error(`Health check failed (${healthResponse.status})`);
        }
        return;
      } catch (error) {
        lastError = error;
        await sleep(API_READY_POLL_MS);
      }
    }

    throw lastError || new Error("Server did not become ready in time");
  })();
  try {
    await apiHealthReady;
  } catch (error) {
    apiHealthReady = undefined;
    throw error;
  }
}

async function fetchWithTimeout(url, options = {}) {
  const controller = new AbortController();
  const timeoutMs = options.timeoutMs ?? REQUEST_TIMEOUT_MS;
  let timedOut = false;
  const timeoutId = window.setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);
  try {
    return await fetch(url, {
      ...options,
      signal: controller.signal
    });
  } catch (error) {
    if (timedOut && (error?.name === "AbortError" || error instanceof TypeError)) {
      const requestLabel = (() => {
        try {
          return new URL(String(url), typeof window !== "undefined" ? window.location.origin : "http://localhost").pathname;
        } catch {
          return String(url);
        }
      })();
      throw new Error(`Request to ${requestLabel} timed out after ${Math.round(timeoutMs / 1000)}s`);
    }
    if (error?.name === "AbortError" && (!error?.message || /aborted without reason/i.test(String(error.message)))) {
      throw new Error("Request was interrupted before the server responded");
    }
    throw error;
  } finally {
    window.clearTimeout(timeoutId);
  }
}

function isRecoverableRequestError(error) {
  if (!error) {
    return false;
  }
  return error.name === "AbortError" || error instanceof TypeError;
}

async function waitForApiRecovery(path, options = {}) {
  const deadline = Date.now() + RECOVERY_TIMEOUT_MS;
  let lastError;

  while (Date.now() < deadline) {
    try {
      await ensureApiReady({ forceRefresh: true });
      return await rawRequest(path, options, { refreshBase: true });
    } catch (error) {
      lastError = error;
      await sleep(RECOVERY_POLL_MS);
    }
  }

  throw lastError || new Error("Server did not come back after update");
}

async function rawRequest(path, options = {}, requestOptions = {}) {
  await ensureApiReady({ forceRefresh: requestOptions.refreshBase === true });
  await resolveApiBase({ forceRefresh: requestOptions.refreshBase === true });
  const url = joinApi(path);
  const response = await fetchWithTimeout(url, {
    timeoutMs: timeoutForPath(path, options.timeoutMs),
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {})
    },
    ...options
  });

  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json")
    ? await response.json()
    : await response.text();

  if (!response.ok) {
    throw new Error(payload?.error || payload || "Request failed");
  }

  return payload;
}

async function request(path, options = {}) {
  try {
    return await rawRequest(path, options);
  } catch (error) {
    if (
      !options.disableRecovery &&
      typeof window !== "undefined" &&
      window.go?.main?.App &&
      isRecoverableRequestError(error)
    ) {
      return waitForApiRecovery(path, { ...options, disableRecovery: true });
    }
    throw error;
  }
}

async function requestForm(path, formData, options = {}) {
  await ensureApiReady({ forceRefresh: options.refreshBase === true });
  await resolveApiBase({ forceRefresh: options.refreshBase === true });
  const response = await fetchWithTimeout(joinApi(path), {
    method: "POST",
    body: formData,
    timeoutMs: timeoutForPath(path, options.timeoutMs),
    ...options
  });

  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json")
    ? await response.json()
    : await response.text();

  if (!response.ok) {
    throw new Error(payload?.error || payload || "Request failed");
  }
  return payload;
}

export function fetchDataSources() {
  return request("/api/data-sources");
}

export function fetchSymbols(source) {
  const params = new URLSearchParams();
  if (source) {
    params.set("source", source);
  }
  return request(`/api/symbols?${params.toString()}`);
}

export function fetchBars(source, symbol, from, to) {
  const params = new URLSearchParams({ symbol, from, to });
  if (source) {
    params.set("source", source);
  }
  return request(`/api/bars?${params.toString()}`);
}

export function fetchSymbolInfo(source, symbol) {
  const params = new URLSearchParams({ symbol });
  if (source) {
    params.set("source", source);
  }
  return request(`/api/symbol-info?${params.toString()}`);
}

export function fetchDefaultPlan() {
  return request("/api/default-plan");
}

export function validatePlan(source, symbol, plan) {
  return request("/api/plans/validate", {
    method: "POST",
    body: JSON.stringify({ data_source: source, symbol, plan })
  });
}

export function runSimulation(payload) {
  return request("/api/simulations/run", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export function runBatchSimulation(payload) {
  return request("/api/simulations/batch", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export function parseStockDetailsCsvFile(file) {
  const form = new FormData();
  form.append("file", file, file?.name || "tickers.csv");
  return requestForm("/api/stock-details/parse-csv", form, {
    timeoutMs: CSV_PARSE_TIMEOUT_MS
  });
}

export function fetchStockDetails(payload) {
  return request("/api/stock-details/fetch-sctr", {
    method: "POST",
    body: JSON.stringify(payload),
    timeoutMs: STOCK_DETAILS_TIMEOUT_MS,
    disableRecovery: true
  });
}

export function fetchIndustryMA50(payload) {
  return request("/api/stock-details/industry-ma50", {
    method: "POST",
    body: JSON.stringify(payload),
    timeoutMs: STOCK_DETAILS_TIMEOUT_MS,
    disableRecovery: true
  });
}
