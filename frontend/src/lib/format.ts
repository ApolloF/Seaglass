export function playtime(sec?: number): string {
  if (!sec) return "Not played";
  const h = sec / 3600;
  if (h < 1) return `${Math.max(1, Math.round(sec / 60))} min`;
  return h < 10 ? `${h.toFixed(1).replace(/\.0$/, "")} h` : `${Math.round(h)} h`;
}

/** "12 h in total." ("Less than a minute in total." under a minute). */
export function totalLine(sec: number): string {
  return sec < 60 ? "Less than a minute in total." : `${playtime(sec)} in total.`;
}

export function ago(unix?: number, now = Date.now() / 1000): string {
  if (!unix) return "Never";
  const days = Math.floor((startOfDay(now) - startOfDay(unix)) / 86400);
  if (days <= 0) return "Today";
  if (days === 1) return "Yesterday";
  if (days < 7) return `${days} days ago`;
  if (days < 14) return "Last week";
  if (days < 31) return `${Math.floor(days / 7)} weeks ago`;
  if (days < 60) return "Last month";
  if (days < 365) return `${Math.floor(days / 30)} months ago`;
  if (days < 730) return "Last year";
  return `${Math.floor(days / 365)} years ago`;
}

function startOfDay(unix: number): number {
  const d = new Date(unix * 1000);
  d.setHours(0, 0, 0, 0);
  return d.getTime() / 1000;
}

export function bytes(n?: number): string {
  if (!n) return "";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1000 && i < units.length - 1) {
    n /= 1000;
    i++;
  }
  return `${n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1)} ${units[i]}`;
}

export function scanned(unix: number, now = Date.now() / 1000): string {
  if (!unix) return "not scanned yet";
  const s = Math.max(0, Math.floor(now - unix));
  if (s < 60) return "scanned just now";
  if (s < 3600) return `scanned ${Math.floor(s / 60)} min ago`;
  return `scanned ${Math.floor(s / 3600)} h ago`;
}

/** A running time as 12:05 or 1:02:03. */
export function clock(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}
