import { describe, expect, it } from "vitest";
import { DEFAULT_BREADTH_SCRIPT, evaluateIndicatorScript, extractRequestedSymbols } from "./indicatorScripts.js";

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
});
