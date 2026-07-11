import { describe, expect, it } from "vitest";
import { indicatorPlotColor } from "./CandleChart.jsx";

describe("indicatorPlotColor", () => {
  it("uses a darker yellow-series color on the light chart", () => {
    expect(indicatorPlotColor("#facc15", "light")).toBe("#a16207");
    expect(indicatorPlotColor("#FACC15", "light")).toBe("#a16207");
    expect(indicatorPlotColor("#facc15", "dark")).toBe("#facc15");
  });

  it("does not alter other plot colors", () => {
    expect(indicatorPlotColor("#a78bfa", "light")).toBe("#a78bfa");
  });
});
