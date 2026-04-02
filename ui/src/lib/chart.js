export function toChartData(bars) {
  return bars.map((bar) => ({
    time: bar.date.slice(0, 10),
    open: bar.open,
    high: bar.high,
    low: bar.low,
    close: bar.close
  }));
}

export function toSMAData(bars, period) {
  if (!Array.isArray(bars) || bars.length < period) {
    return [];
  }

  const result = [];
  let rollingSum = 0;

  for (let index = 0; index < bars.length; index += 1) {
    rollingSum += bars[index].close;

    if (index >= period) {
      rollingSum -= bars[index - period].close;
    }

    if (index >= period - 1) {
      result.push({
        time: bars[index].date.slice(0, 10),
        value: Number((rollingSum / period).toFixed(4))
      });
    }
  }

  return result;
}
