import type { Profile, SyncerAccount } from "./types";

/** An account's colour, when Syncer gave one CSS can use as is. */
export function accountColor(a: SyncerAccount | undefined): string | undefined {
  return a?.color && /^#[0-9a-fA-F]{6}$/.test(a.color) ? a.color : undefined;
}

/** The first letter of a name, for an avatar. */
export function initial(name: string): string {
  return [...name.trim()][0]?.toUpperCase() ?? "?";
}

/** Whether there's anyone to choose between: Syncer has accounts on. */
export function hasAccounts(p: Profile | null): p is Profile & { enabled: true } {
  return !!p && p.enabled && p.accounts.length > 0;
}

/** The account playing on this PC. */
export function playing(p: Profile | null): SyncerAccount | undefined {
  return p?.accounts.find((a) => a.id === (p.active || p.owner));
}

/** One line on how playtime, achievements and settings get to the other PCs. */
export function profileSummary(p: Profile | null, on: boolean): { text: string; tone: "ok" | "warn" | "muted" } {
  if (!on) return { text: "Kept on this PC only.", tone: "muted" };
  if (!p || !p.installed) return { text: "Syncer isn't installed: kept on this PC only for now.", tone: "muted" };
  if (p.dataError) return { text: p.dataError, tone: "warn" };
  if (p.dismissed) return { text: "Syncing was turned off for it in Syncer. Turn it on again there to use it.", tone: "warn" };
  if (p.synced) return { text: `Synced with your other PCs${p.backup ? " and backed up" : ""} by Syncer.`, tone: "ok" };
  if (!p.reachable) return { text: "Syncer isn't running; it's synced once Syncer runs.", tone: "muted" };
  return { text: "Waiting for Syncer…", tone: "muted" };
}
