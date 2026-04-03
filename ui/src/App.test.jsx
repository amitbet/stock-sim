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
      if (path.startsWith("/api/default-plan")) {
        return jsonResponse({ plan: "metadata:\n  name: Test\nreference_price: sell_price\nentry_rules: []\nconstraints:\n  max_actions_per_day: 1\n  prevent_duplicate_level_buys: true\nexit:\n  hold_days_after_full_invest: 10\n" });
      }
      if (path.startsWith("/api/symbols")) {
        return jsonResponse({ symbols: ["QQQ", "AAPL"] });
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
        symbol: "QQQ",
        reference_sell_date: "2024-01-02",
        execution_price_mode: "same_day_close",
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

    const sPriceInput = screen.getByLabelText("S price override");
    expect(sPriceInput).toHaveValue(101);

    fireEvent.change(screen.getByLabelText("Default S price"), {
      target: { value: "high" }
    });

    await waitFor(() => {
      expect(screen.getByLabelText("S price override")).toHaveValue(102);
    });

    fireEvent.change(screen.getByLabelText("S price override"), {
      target: { value: "150.25" }
    });
    expect(screen.getByLabelText("S price override")).toHaveValue(150.25);

    fireEvent.click(screen.getByText("Select 2024-01-02"));

    await waitFor(() => {
      expect(screen.getByLabelText("S price override")).toHaveValue(102);
    });
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
