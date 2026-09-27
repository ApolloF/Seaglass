// A game's achievements in words and order, for the details card, the
// full list and the big picture screen.
import type { Achievement, Achievements } from "./types";

export type AchievementsTone = "ok" | "warn" | "muted";

export interface AchievementsSummary {
  /** "12 / 40 · 30%", or why there's nothing to count. */
  text: string;
  /** What's missing and how to get it; "" when nothing is. */
  detail: string;
  tone: AchievementsTone;
  /** 0–100, for the progress bar. */
  pct: number;
}

/** Share of the achievements unlocked, rounded down (a percent is only 100 when all are). */
export const unlockedPct = (a: Achievements) => (a.total > 0 ? Math.floor((a.unlocked / a.total) * 100) : 0);

export function achievementsSummary(a: Achievements | null): AchievementsSummary | null {
  if (!a) return null;
  if (a.total === 0) {
    if (a.hint) return { text: a.source ? "No achievements yet" : "Not available", detail: a.hint, tone: a.source ? "warn" : "muted", pct: 0 };
    return { text: "No achievements", detail: a.source ? "This game has no achievements." : "", tone: "muted", pct: 0 };
  }
  const pct = unlockedPct(a);
  return {
    text: `${a.unlocked} / ${a.total} · ${pct}%`,
    detail: a.hint ?? "",
    tone: a.hint ? "warn" : a.unlocked === a.total ? "ok" : "muted",
    pct,
  };
}

/**
 * The order the lists show: unlocked first, newest first (unknown times
 * after the dated ones); then locked ones, the most common first (the ones
 * most players get are the likely next ones), unknown rarity last. Ties
 * keep the game's own order.
 */
export function sortAchievements(items: Achievement[]): Achievement[] {
  return items
    .map((a, i) => ({ a, i }))
    .sort((x, y) => {
      const a = x.a, b = y.a;
      if (a.unlocked !== b.unlocked) return a.unlocked ? -1 : 1;
      if (a.unlocked) {
        const d = (b.unlockedAt ?? 0) - (a.unlockedAt ?? 0);
        if (d) return d;
      } else {
        const pa = a.percent ?? -1, pb = b.percent ?? -1;
        if (pa !== pb) return pb - pa;
      }
      return x.i - y.i;
    })
    .map((x) => x.a);
}

/** The latest unlocks, newest first. */
export const recentUnlocks = (items: Achievement[], n = 5) => sortAchievements(items.filter((a) => a.unlocked)).slice(0, n);

/** Whether an achievement's name and description are kept back (a hidden one, still locked). */
export const masked = (a: Achievement, showHidden: boolean) => !!a.hidden && !a.unlocked && !showHidden;

/** An achievement as the lists show it: hidden ones don't give away their name until unlocked. */
export function shown(a: Achievement, showHidden: boolean): Achievement {
  if (!masked(a, showHidden)) return a;
  return { ...a, name: "Hidden achievement", desc: "Keep playing to find out." };
}

/** "12.5% of players", "Rare · 1.2% of players", "" when unknown. */
export function rarityText(a: Achievement): string {
  if (a.percent == null) return "";
  const p = a.percent < 10 ? a.percent.toFixed(1) : Math.round(a.percent).toString();
  return a.percent < 5 ? `Rare · ${p}% of players` : `${p}% of players`;
}

/** "Unlocked 3 May 2024", "Unlocked", "3 / 10" (progress), "" for a locked one. */
export function statusText(a: Achievement): string {
  if (a.unlocked) {
    if (!a.unlockedAt) return "Unlocked";
    return "Unlocked " + new Date(a.unlockedAt * 1000).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
  }
  if (a.max && a.max > 0) return `${Math.floor(a.progress ?? 0)} / ${Math.floor(a.max)}`;
  return "";
}

/** "3 achievements unlocked" for the note after playing. */
export const unlockedText = (n: number) => (n === 1 ? "1 achievement unlocked" : `${n} achievements unlocked`);
