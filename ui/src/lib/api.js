async function request(path, options = {}) {
  const response = await fetch(path, {
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
