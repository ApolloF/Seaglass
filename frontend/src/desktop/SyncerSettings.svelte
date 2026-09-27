<script lang="ts">
  // Settings → Saves: how Seaglass and Syncer get on, and what it does
  // with a game's saves around playing.
  import Icon from "../components/Icon.svelte";
  import Toggle from "../components/Toggle.svelte";
  import { api } from "../lib/api";
  import { syncerSummary } from "../lib/saves";
  import { lib } from "../lib/store.svelte";
  import type { Settings, SyncerStatus } from "../lib/types";

  const s = $derived(lib.settings);
  const set = (patch: Partial<Settings>) => s && lib.saveSettings({ ...s, ...patch });

  let status = $state<SyncerStatus | null>(null);
  let busy = $state(false);
  async function check(start = false) {
    busy = true;
    try {
      status = await api.saves.syncer(start);
    } catch {
      /* the card keeps the last answer */
    } finally {
      busy = false;
    }
  }
  // Looked at when shown and every little while after, without starting it.
  $effect(() => {
    check();
    const t = setInterval(() => !busy && check(), 15000);
    return () => clearInterval(t);
  });
  let installing = $state(false);
  async function install() {
    installing = true;
    await lib.run(() => api.saves.installSyncer());
    installing = false;
    check();
  }
  const sum = $derived(syncerSummary(status, s?.startSyncer ?? true));
  const waits: [number, string][] = [
    [30, "30 s"],
    [60, "1 min"],
    [150, "2½ min"],
    [300, "5 min"],
  ];
</script>

{#if s}
  <div class="card" class:ok={sum?.tone === "ok"} class:warn={sum?.tone === "warn"}>
    <span class="dot" aria-hidden="true"></span>
    <div class="text">
      <span class="t">Syncer: {sum ? sum.title : "Checking…"}</span>
      {#if sum}<span class="d">{sum.detail}</span>{/if}
    </div>
  </div>
  <div class="row">
    {#if sum?.action === "get" || sum?.action === "update"}
      <button type="button" class="btn primary" disabled={installing} onclick={install}
        ><Icon name="download" size={16} />{installing ? (sum.action === "get" ? "Installing Syncer…" : "Updating Syncer…") : sum.action === "get" ? "Install Syncer" : "Update Syncer"}</button
      >
    {:else if sum?.action === "start"}
      <button type="button" class="btn primary" disabled={busy} onclick={() => check(true)}><Icon name="play" size={16} />Start Syncer</button>
    {/if}
    {#if status?.installed}
      <button type="button" class="btn" onclick={() => lib.run(() => api.saves.openSyncer())}><Icon name="link" size={16} />Open Syncer</button>
    {/if}
    <button type="button" class="btn" disabled={busy} onclick={() => check(false)}><Icon name="refresh" size={16} />{busy ? "Checking…" : "Check again"}</button>
    <button type="button" class="btn" onclick={() => lib.run(() => api.saves.syncerProject())}><Icon name="link" size={16} />About Syncer</button>
  </div>

  <div class="group">
    <span class="glabel">Around playing</span>
    <Toggle checked={s.syncSavesBefore} title="Sync saves before playing" detail="A game's saves are brought up to date from your other PCs first. Two versions of a save, or a newer one still on its way, are asked about before the game starts." onchange={(v) => set({ syncSavesBefore: v })} />
    <div class="wait" class:off={!s.syncSavesBefore}>
      <span class="wl">Wait for a sync at most</span>
      <div class="seg" role="group" aria-label="Wait for a sync at most">
        {#each waits as [secs, label] (secs)}
          <button type="button" class:on={s.syncWait === secs} aria-pressed={s.syncWait === secs} disabled={!s.syncSavesBefore} onclick={() => set({ syncWait: secs })}>{label}</button>
        {/each}
      </div>
    </div>
    <Toggle checked={s.backupSavesAfter} title="Back up saves after playing" detail="A backup runs as soon as the game exits, also for games started outside Seaglass." onchange={(v) => set({ backupSavesAfter: v })} />
    <Toggle checked={s.startSyncer} title="Start Syncer when it isn't running" detail="In the background, without its window. Off: games whose saves need Syncer start without syncing until you open it." onchange={(v) => set({ startSyncer: v })} />
  </div>
  <p class="hint">Which games and folders Syncer looks after, and your other PCs, are set up in Syncer.</p>
{/if}

<style>
  .card {
    display: flex;
    gap: 14px;
    align-items: flex-start;
    padding: 16px 18px;
    border-radius: 12px;
    background: var(--surface-2);
    border: 1px solid var(--line);
  }
  .dot {
    width: 12px;
    height: 12px;
    margin-top: 5px;
    border-radius: 50%;
    flex-shrink: 0;
    background: var(--muted);
  }
  .card.ok .dot {
    background: oklch(0.75 0.15 150);
    box-shadow: 0 0 0 4px color-mix(in oklab, oklch(0.75 0.15 150) 25%, transparent);
  }
  .card.warn .dot {
    background: oklch(0.8 0.14 75);
    box-shadow: 0 0 0 4px color-mix(in oklab, oklch(0.8 0.14 75) 25%, transparent);
  }
  .text {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .t {
    font-weight: 700;
  }
  .d {
    color: var(--muted);
    font-size: 13px;
    line-height: 1.45;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin: 12px 0 18px;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    height: 32px;
    padding: 0 14px;
    border-radius: 8px;
    border: 1px solid var(--line);
    background: var(--surface);
    color: var(--text);
    font-size: 13px;
    font-weight: 600;
  }
  .btn.primary {
    background: var(--accent);
    color: var(--accent-ink);
    border-color: transparent;
  }
  .group {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .glabel {
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--muted);
    margin-bottom: 4px;
  }
  .wait {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 4px 0 10px 12px;
    font-size: 13px;
  }
  .wait.off {
    opacity: 0.5;
  }
  .wl {
    color: var(--muted);
  }
  .seg {
    display: inline-flex;
    padding: 3px;
    border-radius: 9px;
    background: var(--surface-3);
    gap: 2px;
  }
  .seg button {
    height: 26px;
    padding: 0 12px;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: var(--text);
    font-size: 12px;
    font-weight: 600;
  }
  .seg button.on {
    background: var(--surface);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.15);
  }
  .hint {
    margin: 14px 0 0;
    font-size: 12px;
    color: var(--muted);
  }
</style>
