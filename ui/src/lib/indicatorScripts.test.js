import { describe, expect, it } from "vitest";
import {
  DEFAULT_BREADTH_SCRIPT,
  DEFAULT_MACD_SCRIPT,
  DEFAULT_NYAD_SCRIPT,
  DEFAULT_OBV_SCRIPT,
  DEFAULT_OBV_SMA14_SCRIPT,
  DEFAULT_RSI_SCRIPT,
  evaluateIndicatorScript,
  extractRequestedSymbols,
  upgradeStoredIndicatorScript
} from "./indicatorScripts.js";

describe("indicator scripts", () => {
  it("extracts and evaluates the default ADR/DAR script", () => {
    expect(extractRequestedSymbols(DEFAULT_BREADTH_SCRIPT)).toEqual(["USI:ADVN.NY", "USI:DECL.NY"]);
    const result = evaluateIndicatorScript(DEFAULT_BREADTH_SCRIPT, {
      "USI:ADVN.NY": [
        { date: "2026-07-01", close: 4000 },
        { date: "2026-07-02", close: 1000 }
      ],
      "USI:DECL.NY": [
        { date: "2026-07-01", close: 1000 },
        { date: "2026-07-02", close: 4000 }
      ]
    });
    expect(result.barsLoaded).toBe(2);
    expect(result.plots).toHaveLength(2);
    expect(result.plots[0].data).toEqual([{ time: "2026-07-01", value: 4 }]);
    expect(result.plots[1].data).toEqual([{ time: "2026-07-02", value: -4 }]);
    expect(result.hlines.map((line) => line.price)).toEqual([0, 3, -3]);
  });

  it("builds a cumulative NYAD line with 10, 20, and 50-period EMAs", () => {
    expect(extractRequestedSymbols(DEFAULT_NYAD_SCRIPT)).toEqual(["USI:NYAD.NY"]);
    const result = evaluateIndicatorScript(DEFAULT_NYAD_SCRIPT, {
      "USI:NYAD.NY": [
        { date: "2026-07-01", close: 300 },
        { date: "2026-07-02", close: -100 },
        { date: "2026-07-03", close: 250 }
      ]
    });
    expect(result.plots[0].data).toEqual([
      { time: "2026-07-01", value: 300 },
      { time: "2026-07-02", value: 200 },
      { time: "2026-07-03", value: 450 }
    ]);
    expect(result.plots.map((plot) => plot.title)).toEqual(["NYAD", "EMA 10", "EMA 20", "EMA 50"]);
    expect(result.plots.slice(1).every((plot) => plot.data.length === 3)).toBe(true);
  });

  it("upgrades only the untouched legacy NYAD script", () => {
    const legacySource = DEFAULT_NYAD_SCRIPT
      .replace(/ema(?:10|20|50) = ta\.ema\(nyadLine, (?:10|20|50)\)\n/g, "")
      .replace(/plot\(ema(?:10|20|50),[^\n]+\n/g, "");
    expect(upgradeStoredIndicatorScript({ id: "nyse-advance-decline-line", source: legacySource }).source)
      .toBe(DEFAULT_NYAD_SCRIPT);

    const customized = `${legacySource}\n// customized`;
    expect(upgradeStoredIndicatorScript({ id: "nyse-advance-decline-line", source: customized }).source)
      .toBe(customized);
  });

  it("upgrades only the untouched legacy RSI script", () => {
    const legacySource = DEFAULT_RSI_SCRIPT
      .replace("ma14 = ta.sma(rsi, 14)\n", "")
      .replace(/plot\(ma14,[^\n]+\n/, "");
    expect(upgradeStoredIndicatorScript({ id: "relative-strength-index", source: legacySource }).source)
      .toBe(DEFAULT_RSI_SCRIPT);

    const customized = `${legacySource}\n// customized`;
    expect(upgradeStoredIndicatorScript({ id: "relative-strength-index", source: customized }).source)
      .toBe(customized);
  });

  it("evaluates RSI, MACD, and OBV from local chart bars", () => {
    const bars = Array.from({ length: 40 }, (_, index) => ({
      date: `2026-01-${String(index + 1).padStart(2, "0")}`,
      close: 100 + index + (index % 3 === 0 ? -2 : 1),
      volume: 1000 + index * 10
    }));
    const rsi = evaluateIndicatorScript(DEFAULT_RSI_SCRIPT, {}, bars);
    const macd = evaluateIndicatorScript(DEFAULT_MACD_SCRIPT, {}, bars);
    const obv = evaluateIndicatorScript(DEFAULT_OBV_SCRIPT, {}, bars);
    expect(rsi.plots.map((plot) => plot.title)).toEqual(["RSI 14", "MA 14"]);
    expect(rsi.plots[0].data.length).toBe(26);
    expect(rsi.plots[1].data.length).toBe(13);
    expect(macd.plots).toHaveLength(3);
    expect(macd.plots.every((plot) => plot.data.length === 40)).toBe(true);
    expect(obv.plots[0].data).toHaveLength(40);
    expect(obv.plots[0].data.at(-1).value).not.toBe(0);
  });

  it("evaluates the supplied OBV SMA14 script", () => {
    const bars = Array.from({ length: 20 }, (_, index) => ({
      date: `2026-02-${String(index + 1).padStart(2, "0")}`,
      close: 100 + (index % 4),
      volume: 100 + index
    }));
    const result = evaluateIndicatorScript(DEFAULT_OBV_SMA14_SCRIPT, {}, bars);
    expect(result.plots).toHaveLength(2);
    expect(result.plots[0].title).toBe("On Balance Volume");
    expect(result.plots[0].color).toBe("#2962FF");
    expect(result.plots[0].data).toHaveLength(19);
    expect(result.plots[1].title).toBe("OBV SMA 14");
    expect(result.plots[1].data).toHaveLength(6);
    expect(() => evaluateIndicatorScript(DEFAULT_OBV_SMA14_SCRIPT, {}, bars.map((bar) => ({ ...bar, volume: 0 }))))
      .toThrow("No volume is provided by the data vendor.");
  });
});
