import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.jsx";

vi.mock("./components/CandleChart.jsx", () => ({
  default: ({ onSelectDate }) => (
    <div>
      <button type="button" onClick={() => onSelectDate("2024-01-02")}>
        Select 2024-01-02
      </button>
    </div>
  )
}));

const fetchMock = vi.fn();
global.fetch = fetchMock;
let queuedResponses;

describe("App", () => {
  beforeEach(() => {
    queuedResponses = new Map();
    fetchMock.mockReset();
    fetchMock.mockImplementation((url) => {
      const path = String(url);
      const queued = queuedResponses.get(path);
      if (queued?.length) {
        return jsonResponse(queued.shift());
      }
      if (path.startsWith("/api/data-sources")) {
        return jsonResponse({ default_source: "sqlite", sources: ["sqlite", "yahoo"] });
      }
      if (path.startsWith("/api/default-plan")) {
        return jsonResponse({ plan: "metadata:\n  name: Test\nreference_price: sell_price\nentry_rules: []\nconstraints:\n  max_actions_per_day: 1\n  prevent_duplicate_level_buys: true\nexit:\n  hold_days_after_full_invest: 10\n" });
      }
      if (path.startsWith("/api/symbol-info?symbol=SPY")) {
        return jsonResponse({ info: { symbol: "SPY", name: "SPDR S&P 500 ETF Trust", description: "SPDR S&P 500 ETF Trust." } });
      }
      if (path.startsWith("/api/symbol-info?symbol=QQQ")) {
        return jsonResponse({ info: { symbol: "QQQ", name: "Invesco QQQ Trust", description: "Invesco QQQ Trust, Nasdaq-100 ETF." } });
      }
      if (path.startsWith("/api/symbol-info?symbol=AAPL")) {
        return jsonResponse({ info: { symbol: "AAPL", name: "Apple Inc.", description: "Apple Inc." } });
      }
      if (path.startsWith("/api/symbols?source=yahoo")) {
        return jsonResponse({ symbols: ["SPY", "IWM"] });
      }
      if (path.startsWith("/api/symbols")) {
        return jsonResponse({ symbols: ["QQQ", "AAPL"] });
      }
      if (path.startsWith("/api/bars?symbol=SPY")) {
        return jsonResponse({ bars: sampleYahooBars() });
      }
      if (path.startsWith("/api/bars")) {
        return jsonResponse({ bars: sampleBars() });
      }
      if (path.startsWith("/api/simulations/run")) {
        return jsonResponse(singleRunResponse());
      }
      if (path.startsWith("/api/simulations/batch")) {
        return jsonResponse({ runs: [singleRunResponse()] });
      }
      throw new Error(`Unhandled fetch in test: ${url}`);
    });
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("loads symbols and renders the batch modal after a batch run", async () => {
    enqueueResponse("/api/simulations/batch", {
      runs: [
        {
          summary: {
            reference_sell_date: "2024-01-02",
            reference_price: 101,
            full_invest_date: "2024-01-10",
            end_date: "2024-01-30",
            gain_pct: 5.4,
            total_invested_pct: 100,
            execution_mode: "next_day_open"
          },
          actions: [],
          stats: { max_drawdown_pct: -4.5, bars_to_full_invest: 4, bars_to_end: 15 }
        }
      ]
    });

    render(<App />);

    await screen.findByText("Stock Simulator");
    await waitForBarsLoaded();

    fireEvent.click(screen.getByLabelText("Multi-select mode"));
    fireEvent.click(screen.getByText("Select 2024-01-02"));

    fireEvent.click(screen.getByText("Batch Simulate"));

    await screen.findByText("Batch Simulation Report");
  });

  it("switches data source and reloads the symbol universe", async () => {
    render(<App />);

    await screen.findByText("Stock Simulator");
    await waitForBarsLoaded();

    fireEvent.change(screen.getByLabelText("Data source"), {
      target: { value: "yahoo" }
    });

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([url]) => url === "/api/symbols?source=yahoo")
      ).toBe(true);
    });

    await waitFor(() => {
      expect(screen.getByDisplayValue("SPY")).toBeInTheDocument();
    });
    expect(screen.getByText("SPDR S&P 500 ETF Trust.")).toBeInTheDocument();
  });

  it("allows entering a custom ticker that is not in the preset list", async () => {
    render(<App />);

    await screen.findByText("Stock Simulator");
    await waitForBarsLoaded();

    fireEvent.change(screen.getByLabelText("Data source"), {
      target: { value: "yahoo" }
    });

    const symbolInput = screen.getByPlaceholderText("Search ticker");
    fireEvent.focus(symbolInput);
    fireEvent.change(symbolInput, { target: { value: "AAPL" } });
    fireEvent.keyDown(symbolInput, { key: "Enter", code: "Enter" });

    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([url]) => url.includes("/api/bars?symbol=AAPL"))).toBe(true);
    });
  });

  it("selects the first matching symbol when pressing enter in the symbol picker", async () => {
    render(<App />);

    await screen.findByText("Stock Simulator");
    await waitForBarsLoaded();

    const symbolInput = screen.getByPlaceholderText("Search ticker");
    fireEvent.focus(symbolInput);
    fireEvent.change(symbolInput, { target: { value: "AA" } });
    fireEvent.keyDown(symbolInput, { key: "Enter", code: "Enter" });

    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([url]) => url.includes("/api/bars?symbol=AAPL"))).toBe(true);
    });
  });

  it("reruns the selected date when run settings change", async () => {
    enqueueResponse("/api/simulations/run", singleRunResponse());
    enqueueResponse("/api/simulations/run", singleRunResponse());

    render(<App />);

    await screen.findByText("Stock Simulator");
    await waitForBarsLoaded();

    fireEvent.click(screen.getByText("Select 2024-01-02"));

    await waitFor(() => {
      const firstRunCall = fetchMock.mock.calls.find(([url]) => url === "/api/simulations/run");
      expect(firstRunCall).toBeTruthy();
      expect(JSON.parse(firstRunCall[1].body)).toMatchObject({
        data_source: "sqlite",
        symbol: "QQQ",
        reference_sell_date: "2024-01-02",
        execution_price_mode: "exact",
        reference_price_mode: "close",
        reference_price: 101
      });
    });

    fireEvent.change(screen.getByLabelText("Execution price"), {
      target: { value: "next_day_open" }
    });

    await waitFor(() => {
      const runCalls = fetchMock.mock.calls.filter(([url]) => url === "/api/simulations/run");
      expect(runCalls).toHaveLength(2);
      expect(JSON.parse(runCalls[1][1].body)).toMatchObject({
        data_source: "sqlite",
        symbol: "QQQ",
        reference_sell_date: "2024-01-02",
        execution_price_mode: "next_day_open",
        reference_price_mode: "close",
        reference_price: 101
      });
    }, { timeout: 1500 });
  });

  it("resets S price from the selected source when the date changes", async () => {
    enqueueResponse("/api/simulations/run", singleRunResponse());
    enqueueResponse("/api/simulations/run", singleRunResponse());
    enqueueResponse("/api/simulations/run", singleRunResponse());

    render(<App />);

    await screen.findByText("Stock Simulator");
    await waitForBarsLoaded();

    fireEvent.click(screen.getByText("Select 2024-01-02"));

    const sPriceInput = screen.getByLabelText(/S price override/i);
    expect(sPriceInput).toHaveValue(101);

    fireEvent.change(screen.getByLabelText("Default S price"), {
      target: { value: "high" }
    });

    await waitFor(() => {
      expect(screen.getByLabelText(/S price override/i)).toHaveValue(102);
    });

    fireEvent.change(screen.getByLabelText(/S price override/i), {
      target: { value: "150.25" }
    });
    expect(screen.getByLabelText(/S price override/i)).toHaveValue(150.25);

    fireEvent.click(screen.getByText("Select 2024-01-02"));

    await waitFor(() => {
      expect(screen.getByLabelText(/S price override/i)).toHaveValue(102);
    });
  });

  it("validates S price override against the selected candle range", async () => {
    enqueueResponse("/api/simulations/run", singleRunResponse());

    render(<App />);

    await screen.findByText("Stock Simulator");
    await waitForBarsLoaded();

    fireEvent.click(screen.getByText("Select 2024-01-02"));

    await waitFor(() => {
      const runCalls = fetchMock.mock.calls.filter(([url]) => url === "/api/simulations/run");
      expect(runCalls).toHaveLength(1);
    });

    fireEvent.change(screen.getByLabelText(/S price override/i), {
      target: { value: "150.25" }
    });

    expect(screen.getByText(/S price override must stay within the selected candle range/)).toBeInTheDocument();
    expect(screen.getByText(/Day range: 98 to 102/)).toBeInTheDocument();
    expect(screen.getByText("Run Selected Date")).toBeDisabled();

    await new Promise((resolve) => setTimeout(resolve, 500));

    const runCalls = fetchMock.mock.calls.filter(([url]) => url === "/api/simulations/run");
    expect(runCalls).toHaveLength(1);
  });

  it("handles runs that return null actions", async () => {
    enqueueResponse("/api/simulations/run", {
      summary: {
        reference_sell_date: "2026-03-30",
        reference_price: 101,
        full_invest_date: "",
        end_date: "2026-04-03",
        gain_pct: 0,
        total_invested_pct: 0,
        execution_mode: "same_day_close"
      },
      actions: null,
      stats: {
        max_drawdown_pct: 0,
        bars_to_full_invest: 0,
        bars_to_end: 4
      }
    });

    render(<App />);

    await screen.findByText("Stock Simulator");
    await waitForBarsLoaded();

    fireEvent.click(screen.getByText("Select 2024-01-02"));

    await screen.findByText("Run Results");

    expect(screen.getByText("Not reached")).toBeInTheDocument();
  });
});

function jsonResponse(payload) {
  return Promise.resolve({
    ok: true,
    headers: { get: () => "application/json" },
    json: () => Promise.resolve(payload)
  });
}

async function waitForBarsLoaded() {
  await waitFor(() => {
    expect(screen.getByText("Bars loaded").closest(".hero-metric")).toHaveTextContent("2");
  });
}

function enqueueResponse(path, payload) {
  const existing = queuedResponses.get(path) || [];
  existing.push(payload);
  queuedResponses.set(path, existing);
}

function singleRunResponse() {
  return {
    summary: {
      reference_sell_date: "2024-01-02",
      reference_price: 101,
      full_invest_date: "2024-01-10",
      end_date: "2024-01-30",
      gain_pct: 5.4,
      total_invested_pct: 100,
      execution_mode: "next_day_open"
    },
    actions: [],
    stats: {
      max_drawdown_pct: -4.5,
      bars_to_full_invest: 4,
      bars_to_end: 15
    }
  };
}

function sampleBars() {
  return [
    { date: "2024-01-02T00:00:00Z", open: 100, high: 102, low: 98, close: 101 },
    { date: "2024-01-03T00:00:00Z", open: 101, high: 103, low: 99, close: 100 }
  ];
}

function sampleYahooBars() {
  return [
    { date: "2024-01-02T00:00:00Z", open: 470, high: 472, low: 468, close: 471 },
    { date: "2024-01-03T00:00:00Z", open: 471, high: 473, low: 469, close: 472 }
  ];
}
