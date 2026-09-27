// Game lists and helpers shared by the big picture layouts.
import { artFor } from "./art";
import { isFresh } from "./store.svelte";
import { lastPlayed, played, title, type Game } from "./types";
import { playtime, ago } from "./format";
import { pad } from "./input.svelte";
import { padExplain } from "./route";

export const byRecent = (a: Game, b: Game) => lastPlayed(b) - lastPlayed(a) || a.sortTitle.localeCompare(b.sortTitle);
export const byTitle = (a: Game, b: Game) => a.sortTitle.localeCompare(b.sortTitle);

/** Installed games, most recently played first (then A–Z). */
export function recentFirst(games: Game[]): Game[] {
  return games.filter((g) => g.installed).sort(byRecent);
}

const RECENT = 60 * 86400;

/**
 * Orbit's order, from the middle out: the games played lately (at most
 * `recent`, most recent first), then favorites, then the rest of the
 * installed games, most recently played first. The first two rings hold
 * 19 games.
 */
export function orbitOrder(games: Game[], now = Date.now() / 1000, recent = 10): Game[] {
  const installed = recentFirst(games);
  const core = installed.filter((g) => lastPlayed(g) > 0 && now - lastPlayed(g) < RECENT).slice(0, recent);
  const inCore = new Set(core);
  const favs = installed.filter((g) => g.favorite && !inCore.has(g));
  const rest = installed.filter((g) => !g.favorite && !inCore.has(g));
  return [...core, ...favs, ...rest];
}

/** Recently played games; falls back to the whole library when nothing was played. */
export function continuePlaying(games: Game[], n = 10): Game[] {
  const played = games.filter((g) => g.installed && lastPlayed(g) > 0).sort(byRecent);
  return (played.length ? played : games.filter((g) => g.installed).sort(byTitle)).slice(0, n);
}

/** Games that need a check (first), then new finds, newest first. */
export function newFinds(games: Game[], n = 10): Game[] {
  return games
    .filter((g) => g.installed && (isFresh(g) || g.needsReview))
    .sort((a, b) => Number(b.needsReview) - Number(a.needsReview) || b.addedAt - a.addedAt)
    .slice(0, n);
}

export const accentOf = (g: Game | null | undefined) => (g ? (g.meta?.accent ?? artFor(g.key).accent) : undefined);

/** "18 h · Played yesterday" style line. */
export function metaLine(g: Game): string {
  const p = played(g);
  const lp = lastPlayed(g);
  // Steam can know when a game was last played but count under a minute.
  if (!p && !lp) return `Not played yet · Added ${ago(g.addedAt).toLowerCase()}`;
  return [p ? playtime(p) : "", lp ? `Played ${ago(lp).toLowerCase()}` : `Added ${ago(g.addedAt).toLowerCase()}`].filter(Boolean).join(" · ");
}

export { title };

/** What the controller mode will do, in words, for the big picture cards. */
export function padSummary(g: Game): { short: string; long: string } {
  return padExplain(g, pad);
}
