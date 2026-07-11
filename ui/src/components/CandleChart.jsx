import { useEffect, useRef } from "react";
import {
  CandlestickSeries,
  HistogramSeries,
  LineSeries,
  createChart,
  createSeriesMarkers
} from "lightweight-charts";
import { toChartData, toSMAData } from "../lib/chart.js";

const MOVING_AVERAGES = [
  { period: 20, color: "#38bdf8", title: "SMA 20" },
  { period: 50, color: "#f59e0b", title: "SMA 50" },
  { period: 150, color: "#a78bfa", title: "SMA 150" }
];

const LIGHT_THEME_PLOT_COLORS = {
  // Pine's bright yellow is clear on the dark chart but nearly disappears on white.
  "#facc15": "#a16207"
};

export function indicatorPlotColor(color, theme) {
  return theme === "light" ? LIGHT_THEME_PLOT_COLORS[color?.toLowerCase()] || color : color;
}

function compareMarkerTimes(left, right) {
  return String(left.time).localeCompare(String(right.time));
}

function markerActionText(actions, date) {
  const matches = actions.filter((item) => item.date === date);
  if (matches.length === 0) {
    return "";
  }
  return matches.map((item) => `${item.trigger_id} ${item.allocation_pct}%`).join(" | ");
}

export default function CandleChart({
  bars,
  selectedDate,
  multiSelectedDates,
  multiSelectEnabled,
  onSelectDate,
  actions,
  endDate,
  indicatorResults,
  theme = "dark"
}) {
  const hostRef = useRef(null);
  const chartRef = useRef(null);
  const seriesRef = useRef(null);
  const maSeriesRef = useRef([]);
  const indicatorSeriesRef = useRef([]);
  const markersRef = useRef(null);
  const onSelectDateRef = useRef(onSelectDate);

  useEffect(() => {
    onSelectDateRef.current = onSelectDate;
  }, [onSelectDate]);

  useEffect(() => {
    if (!hostRef.current || chartRef.current) {
      return undefined;
    }

    const chart = createChart(hostRef.current, {
      autoSize: true,
      layout: {
        background: { color: "#0d1b2a" },
        textColor: "#dfe7ef",
        fontFamily: "IBM Plex Sans, sans-serif"
      },
      grid: {
        vertLines: { color: "rgba(255,255,255,0.07)" },
        horzLines: { color: "rgba(255,255,255,0.07)" }
      },
      crosshair: {
        mode: 0
      },
      rightPriceScale: {
        scaleMargins: { top: 0.1, bottom: 0.15 }
      },
      timeScale: {
        timeVisible: true,
        secondsVisible: false
      }
    });

    const series = chart.addSeries(CandlestickSeries, {
      upColor: "#21c55d",
      downColor: "#ef4444",
      borderVisible: false,
      wickUpColor: "#7dd3a8",
      wickDownColor: "#fca5a5"
    });

    const maSeries = MOVING_AVERAGES.map((movingAverage) =>
      chart.addSeries(LineSeries, {
        color: movingAverage.color,
        lineWidth: 2,
        lastValueVisible: false,
        priceLineVisible: false,
        crosshairMarkerVisible: false
      })
    );

    chart.subscribeClick((param) => {
      const time = param?.time;
      if (!time) {
        return;
      }
      const isoDate = typeof time === "string"
        ? time
        : `${time.year}-${String(time.month).padStart(2, "0")}-${String(time.day).padStart(2, "0")}`;
      onSelectDateRef.current?.(isoDate);
    });

    const markers = createSeriesMarkers(series, []);

    chartRef.current = chart;
    seriesRef.current = series;
    maSeriesRef.current = maSeries;
    markersRef.current = markers;

    return () => {
      markers.detach();
      chart.remove();
      chartRef.current = null;
      seriesRef.current = null;
      maSeriesRef.current = [];
      indicatorSeriesRef.current = [];
      markersRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!chartRef.current) {
      return;
    }
    const isLight = theme === "light";
    chartRef.current.applyOptions({
      layout: {
        background: { color: isLight ? "#ffffff" : "#0d1b2a" },
        textColor: isLight ? "#1e293b" : "#dfe7ef",
        fontFamily: "IBM Plex Sans, sans-serif"
      },
      grid: {
        vertLines: { color: isLight ? "rgba(15, 23, 42, 0.09)" : "rgba(255, 255, 255, 0.07)" },
        horzLines: { color: isLight ? "rgba(15, 23, 42, 0.09)" : "rgba(255, 255, 255, 0.07)" }
      }
    });
  }, [theme]);

  useEffect(() => {
    if (!seriesRef.current || !chartRef.current) {
      return;
    }

    const chartData = toChartData(bars);
    seriesRef.current.setData(chartData);
    maSeriesRef.current.forEach((lineSeries, index) => {
      const movingAverage = MOVING_AVERAGES[index];
      lineSeries.setData(toSMAData(bars, movingAverage.period));
    });

    if (bars.length > 0) {
      chartRef.current.timeScale().fitContent();
    }
  }, [bars]);

  useEffect(() => {
    if (!markersRef.current) {
      return;
    }

    const markers = [];
    for (const action of actions || []) {
      markers.push({
        time: action.date,
        position: "belowBar",
        color: "#00c389",
        shape: "arrowUp",
        text: `${action.allocation_pct}%`
      });
    }
    if (endDate) {
      markers.push({
        time: endDate,
        position: "aboveBar",
        color: "#ffb703",
        shape: "circle",
        text: "End"
      });
    }
    if (selectedDate) {
      markers.push({
        time: selectedDate,
        position: "aboveBar",
        color: multiSelectEnabled ? "#7c3aed" : "#4cc9f0",
        shape: "square",
        text: multiSelectEnabled ? `Pick ${multiSelectedDates.length}` : "S"
      });
    }
    for (const date of multiSelectedDates || []) {
      if (date === selectedDate) {
        continue;
      }
      markers.push({
        time: date,
        position: "aboveBar",
        color: "#7c3aed",
        shape: "square",
        text: markerActionText(actions || [], date) || "Batch"
      });
    }

    markers.sort(compareMarkerTimes);
    markersRef.current.setMarkers(markers);
  }, [selectedDate, multiSelectedDates, multiSelectEnabled, actions, endDate]);

  useEffect(() => {
    if (!chartRef.current) return;
    for (const item of indicatorSeriesRef.current) {
      try {
        chartRef.current.removeSeries(item.series);
      } catch {
        /* ignore stale chart series */
      }
    }
    indicatorSeriesRef.current = [];

    while (chartRef.current.panes().length > 1) {
      chartRef.current.removePane(chartRef.current.panes().length - 1);
    }

    const active = (indicatorResults || []).filter((result) => !result.error && result.plots?.length > 0);
    if (active.length === 0) {
      return;
    }

    active.forEach((result, resultIndex) => {
      const paneIndex = resultIndex + 1;
      let firstSeries = null;
      for (const plot of result.plots) {
        const definition = plot.type === "histogram" ? HistogramSeries : LineSeries;
        const series = chartRef.current.addSeries(definition, {
          color: indicatorPlotColor(plot.color, theme),
          lineWidth: plot.lineWidth,
          priceScaleId: result.title,
          priceLineVisible: false,
          lastValueVisible: true,
          crosshairMarkerVisible: plot.type !== "histogram"
        }, paneIndex);
        series.setData(plot.data);
        indicatorSeriesRef.current.push({ series });
        if (!firstSeries) firstSeries = series;
      }

      if (!firstSeries) return;
      const created = new Set();
      for (const line of result.hlines || []) {
        const key = `${line.price}-${line.title}`;
        if (created.has(key)) continue;
        created.add(key);
        firstSeries.createPriceLine({
          price: line.price,
          color: line.color,
          lineWidth: line.lineWidth,
          lineStyle: 0,
          axisLabelVisible: true,
          title: line.title
        });
      }
    });

    const panes = chartRef.current.panes();
    if (panes[0]) panes[0].setStretchFactor(3);
    for (let paneIndex = 1; paneIndex < panes.length; paneIndex += 1) {
      panes[paneIndex].setStretchFactor(1);
    }
  }, [indicatorResults, theme]);

  return (
    <div className="chart-shell">
      <div className="chart-legend">
        {MOVING_AVERAGES.map((movingAverage) => (
          <div className="chart-legend-item" key={movingAverage.period}>
            <span className="chart-legend-swatch" style={{ backgroundColor: movingAverage.color }} />
            <span>{movingAverage.title}</span>
          </div>
        ))}
        {(indicatorResults || []).filter((result) => !result.error).flatMap((result) => result.plots || []).map((plot) => (
          <div className="chart-legend-item" key={plot.id}>
            <span className="chart-legend-swatch" style={{ backgroundColor: indicatorPlotColor(plot.color, theme) }} />
            <span>{plot.title}</span>
          </div>
        ))}
      </div>
      <div className="chart-host" ref={hostRef} data-testid="candle-chart" />
    </div>
  );
}
