import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
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

describe("App", () => {
  beforeEach(() => {
    fetchMock.mockReset();
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ symbols: ["QQQ", "AAPL"] }))
      .mockResolvedValueOnce(jsonResponse({ plan: "metadata:\n  name: Test\nreference_price: sell_price\nentry_rules: []\nconstraints:\n  max_actions_per_day: 1\n  prevent_duplicate_level_buys: true\nexit:\n  hold_days_after_full_invest: 10\n" }))
      .mockResolvedValueOnce(jsonResponse({ bars: sampleBars() }));
  });

  it("loads symbols and renders the batch modal after a batch run", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({
      runs: [
        {
          summary: {
            reference_sell_date: "2024-01-02",
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
    }));

    render(<App />);

    await screen.findByText("Stock Simulator");

    fireEvent.click(screen.getByLabelText("Multi-select mode"));
    fireEvent.click(screen.getByText("Select 2024-01-02"));

    fireEvent.click(screen.getByText("Batch Simulate"));

    await screen.findByText("Batch Simulation Report");
  });
});

function jsonResponse(payload) {
  return Promise.resolve({
    ok: true,
    headers: { get: () => "application/json" },
    json: () => Promise.resolve(payload)
  });
}

function sampleBars() {
  return [
    { date: "2024-01-02T00:00:00Z", open: 100, high: 102, low: 98, close: 101 },
    { date: "2024-01-03T00:00:00Z", open: 101, high: 103, low: 99, close: 100 }
  ];
}
