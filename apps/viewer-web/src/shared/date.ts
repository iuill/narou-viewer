export function formatDate(value: string | null): string {
  if (!value) {
    return "未取得";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat("ja-JP", {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(date);
}

export function formatElapsedMs(value: number): string {
  if (value < 1000) {
    return `${value}ms`;
  }

  const seconds = value / 1000;
  return `${seconds.toFixed(seconds >= 10 ? 1 : 2)}s`;
}
