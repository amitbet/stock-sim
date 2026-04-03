import { useEffect, useRef } from "react";
import {
  CandlestickSeries,
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
  endDate
}) {
  const hostRef = useRef(null);
  const chartRef = useRef(null);
  const seriesRef = useRef(null);
  const maSeriesRef = useRef([]);
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
      markersRef.current = null;
    };
  }, []);

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

    markersRef.current.setMarkers(markers);
  }, [selectedDate, multiSelectedDates, multiSelectEnabled, actions, endDate]);

  return (
    <div className="chart-shell">
      <div className="chart-legend">
        {MOVING_AVERAGES.map((movingAverage) => (
          <div className="chart-legend-item" key={movingAverage.period}>
            <span className="chart-legend-swatch" style={{ backgroundColor: movingAverage.color }} />
            <span>{movingAverage.title}</span>
          </div>
        ))}
      </div>
      <div className="chart-host" ref={hostRef} data-testid="candle-chart" />
    </div>
  );
}
