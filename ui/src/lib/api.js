let apiBaseCache;
let apiBaseReady;

async function waitForWailsBindings() {
  for (let i = 0; i < 80; i++) {
    if (typeof window !== "undefined" && window.go?.main?.App) {
      return true;
    }
    await new Promise((r) => setTimeout(r, 25));
  }
  return typeof window !== "undefined" && !!window.go?.main?.App;
}

async function resolveApiBase() {
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

async function request(path, options = {}) {
  await resolveApiBase();
  const url = joinApi(path);
  const response = await fetch(url, {
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

export function fetchAppVersion() {
  return request("/api/app/version");
}

export function fetchUpdateStatus() {
  return request("/api/app/update-status");
}

export function applyBrowserUpdate() {
  return request("/api/app/apply-update", {
    method: "POST"
  });
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
