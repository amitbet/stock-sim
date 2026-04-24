import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import StockDetailsPanel from "./StockDetailsPanel.jsx";
import { fetchIndustryMA50, fetchStockDetails, parseStockDetailsCsvFile } from "../lib/api.js";

vi.mock("../lib/api.js", () => ({
  fetchIndustryMA50: vi.fn(),
  fetchStockDetails: vi.fn(),
  parseStockDetailsCsvFile: vi.fn()
}));

describe("StockDetailsPanel", () => {
  beforeEach(() => {
    parseStockDetailsCsvFile.mockResolvedValue({
      tickerColumnName: "Symbol",
      tickerColumnIndex: 0,
      tickers: ["SPY", "AAPL"],
      criteriaByTicker: {
        SPY: "Breakout",
        AAPL: "Base"
      }
    });
    fetchStockDetails.mockResolvedValue({
      records: [
        { symbol: "SPY", name: "SPDR S&P 500 ETF Trust", type: "ETF", SCTR: 98.4, industry: "Exchange Traded Fund" },
        { symbol: "AAPL", name: "Apple Inc.", type: "Large", SCTR: 91.2, industry: "Consumer Electronics" }
      ],
      missingTickers: []
    });
    fetchIndustryMA50.mockResolvedValue({ ma50: null });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("shows CSV criteria and StockCharts types in the detail table", async () => {
    const { container } = render(<StockDetailsPanel />);
    const input = container.querySelector('input[type="file"]');
    const file = new File(["Symbol,Criteria\nSPY,Breakout\nAAPL,Base\n"], "tickers.csv", { type: "text/csv" });

    fireEvent.change(input, { target: { files: [file] } });

    await screen.findByText("Breakout");
    expect(screen.getByText("Base")).toBeInTheDocument();
    expect(screen.getByText("Criteria")).toBeInTheDocument();
    expect(screen.getByText("Type")).toBeInTheDocument();
    expect(screen.getByText("ETF")).toBeInTheDocument();
    expect(screen.getByText("Large")).toBeInTheDocument();

    await waitFor(() => {
      expect(fetchStockDetails).toHaveBeenCalledWith({
        tickers: ["SPY", "AAPL"],
        industrySource: "finviz",
        includeIndustryStrength: false,
        forceRefresh: false
      });
    });
    expect(screen.queryByText("Ind RS")).not.toBeInTheDocument();
    expect(screen.queryByText("Sec RS")).not.toBeInTheDocument();
    expect(screen.queryByText("Ind vs MA50")).not.toBeInTheDocument();
    expect(fetchIndustryMA50).not.toHaveBeenCalled();
  });

  it("requests and displays industry strength only when toggled on", async () => {
    const { container } = render(<StockDetailsPanel />);
    const input = container.querySelector('input[type="file"]');
    const file = new File(["Symbol,Criteria\nSPY,Breakout\nAAPL,Base\n"], "tickers.csv", { type: "text/csv" });

    fireEvent.change(input, { target: { files: [file] } });
    await screen.findByText("Breakout");

    fireEvent.click(screen.getByLabelText("Industry Strength"));

    await waitFor(() => {
      expect(fetchStockDetails).toHaveBeenLastCalledWith({
        tickers: ["SPY", "AAPL"],
        industrySource: "finviz",
        includeIndustryStrength: true,
        forceRefresh: false
      });
    });
    expect(await screen.findByText("Ind RS")).toBeInTheDocument();
  });

  it("uses the per-source detail cache when the industry source changes", async () => {
    const { container } = render(<StockDetailsPanel />);
    const input = container.querySelector('input[type="file"]');
    const file = new File(["Symbol,Criteria\nSPY,Breakout\nAAPL,Base\n"], "tickers.csv", { type: "text/csv" });

    fireEvent.change(input, { target: { files: [file] } });
    await screen.findByText("Breakout");

    fireEvent.change(screen.getByLabelText("Industry Source"), {
      target: { value: "yahoo" }
    });

    await waitFor(() => {
      expect(fetchStockDetails).toHaveBeenLastCalledWith({
        tickers: ["SPY", "AAPL"],
        industrySource: "yahoo",
        includeIndustryStrength: false,
        forceRefresh: false
      });
    });
  });

  it("appends new manual tickers to the existing table", async () => {
    fetchStockDetails
      .mockResolvedValueOnce({
        records: [
          { symbol: "SPY", name: "SPDR S&P 500 ETF Trust", type: "ETF", SCTR: 98.4, industry: "Exchange Traded Fund" },
          { symbol: "AAPL", name: "Apple Inc.", type: "Large", SCTR: 91.2, industry: "Consumer Electronics" }
        ],
        missingTickers: []
      })
      .mockResolvedValueOnce({
        records: [
          { symbol: "MSFT", name: "Microsoft Corp.", type: "Large", SCTR: 89.5, industry: "Software" }
        ],
        missingTickers: []
      });

    const { container } = render(<StockDetailsPanel />);
    const input = container.querySelector('input[type="file"]');
    const file = new File(["Symbol,Criteria\nSPY,Breakout\nAAPL,Base\n"], "tickers.csv", { type: "text/csv" });

    fireEvent.change(input, { target: { files: [file] } });
    await screen.findByText("Breakout");

    fireEvent.change(screen.getByPlaceholderText(/Paste tickers/), {
      target: { value: "AAPL MSFT" }
    });
    fireEvent.click(screen.getByText("Append"));

    await screen.findByText("Microsoft Corp.");
    expect(screen.getByText("SPDR S&P 500 ETF Trust")).toBeInTheDocument();
    expect(screen.getByText("Apple Inc.")).toBeInTheDocument();
    await waitFor(() => {
      expect(fetchStockDetails).toHaveBeenLastCalledWith({
        tickers: ["MSFT"],
        industrySource: "finviz",
        includeIndustryStrength: false,
        forceRefresh: false
      });
    });
  });
});
