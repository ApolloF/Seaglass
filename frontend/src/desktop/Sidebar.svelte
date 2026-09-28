<script lang="ts">
  import Icon, { type IconName } from "../components/Icon.svelte";
  import WhoPlays from "../components/WhoPlays.svelte";
  import { api } from "../lib/api";
  import { scanned } from "../lib/format";
  import { lib, type FilterKind } from "../lib/store.svelte";

  let { onbigpicture }: { onbigpicture?: () => void } = $props();

  const views = $derived.by(() => {
    const c = lib.counts;
    const list: { kind: FilterKind; label: string; icon: IconName; count: number; accent?: boolean }[] = [
      { kind: "all", label: "All games", icon: "grid", count: c.all },
      { kind: "favorites", label: "Favorites", icon: "star", count: c.favorites },
      { kind: "recent", label: "Recently played", icon: "clock", count: c.recent },
      { kind: "found", label: "New on this PC", icon: "scan", count: c.found, accent: c.found > 0 },
    ];
    if (lib.settings?.showNotInstalled) list.splice(1, 0, { kind: "notinstalled", label: "Not installed", icon: "cloudDown", count: c.notinstalled });
    if (c.hidden) list.push({ kind: "hidden", label: "Hidden", icon: "eyeOff", count: c.hidden });
    return list;
  });

  const dots: Record<string, string> = {
    steam: "oklch(0.72 0.1 240)",
    epic: "oklch(0.85 0.01 240)",
    gog: "oklch(0.65 0.16 305)",
    ea: "oklch(0.68 0.19 25)",
    ubisoft: "oklch(0.7 0.14 250)",
    battlenet: "oklch(0.72 0.13 230)",
    xbox: "oklch(0.72 0.17 145)",
    external: "oklch(0.78 0.14 60)",
    standalone: "oklch(0.72 0.05 200)",
    folder: "oklch(0.7 0.02 240)",
  };

  const isActive = (kind: string, source?: string) =>
    lib.filter.kind === kind && (kind !== "source" || ("source" in lib.filter && lib.filter.source === source));

  // Keep the "scanned … ago" text current.
  let now = $state(Date.now() / 1000);
  $effect(() => {
    const t = setInterval(() => (now = Date.now() / 1000), 30_000);
    return () => clearInterval(t);
  });
</script>

<aside class="side">
  <nav aria-label="Library">
    <span class="label">Library</span>
    {#each views as v (v.kind)}
      <button type="button" class="nav" class:active={isActive(v.kind)} onclick={() => (lib.filter = { kind: v.kind })}>
        <Icon name={v.icon} size={18} stroke={2} />
        <span class="grow">{v.label}</span>
        <span class="count" class:accent={v.accent}>{v.count}</span>
      </button>
    {/each}

    {#if lib.collections.length}
      <span class="label">Collections</span>
      {#each lib.collections as c (c.name.toLowerCase())}
        <button
          type="button"
          class="nav small"
          class:active={lib.filter.kind === "collection" && lib.filter.name.toLowerCase() === c.name.toLowerCase()}
          onclick={() => (lib.filter = { kind: "collection", name: c.name })}
        >
          <span class="dot-wrap"><Icon name="layers" size={16} stroke={2} /></span>
          <span class="grow">{c.name}</span>
          <span class="count">{c.count}</span>
        </button>
      {/each}
    {/if}

    {#if lib.sources.length}
      <span class="label">Sources</span>
      {#each lib.sources as s (s.id)}
        <button type="button" class="nav small" class:active={isActive("source", s.id)} onclick={() => (lib.filter = { kind: "source", source: s.id })}>
          <span class="dot-wrap"><span class="dot" style:background={dots[s.id]}></span></span>
          <span class="grow">{s.label}</span>
          <span class="count">{s.count}</span>
        </button>
      {/each}
    {/if}
  </nav>

  <div class="foot">
    <WhoPlays />
    {#if lib.update?.status === "ready" && !lib.update.failed}
      <button type="button" class="update" onclick={() => lib.installUpdate()} title="Installs {lib.update.latest} and restarts Seaglass">
        <Icon name="sparkle" size={16} stroke={2} />
        <span class="grow">{lib.update.latest} is ready</span>
        <span class="go">Restart</span>
      </button>
    {/if}
    <button type="button" class="scan" onclick={() => api.rescan()} disabled={lib.scan.running} title="Look for games again (F5)">
      <span class="spin" class:on={lib.scan.running || lib.meta.running}><Icon name="refresh" size={16} stroke={2} /></span>
      <span
        >{lib.scan.running
          ? "Looking for games…"
          : lib.meta.running && lib.meta.total > 0
            ? `Fetching art · ${Math.min(lib.meta.done + 1, lib.meta.total)} of ${lib.meta.total}`
            : `${lib.counts.all} games · ${scanned(lib.scan.lastScan, now)}`}</span
      >
    </button>
    {#if onbigpicture}
      <button type="button" class="bp" onclick={onbigpicture}>
        <Icon name="pad" size={20} stroke={1.8} />
        <span>Big picture</span>
      </button>
    {/if}
  </div>
</aside>

<style>
  .side {
    width: 248px;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    background: var(--surface);
    border-right: 1px solid var(--line);
    padding: 14px 12px;
    overflow-y: auto;
  }
  nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .label {
    padding: 14px 12px 6px;
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--muted);
  }
  .label:first-child {
    padding-top: 4px;
  }
  .nav {
    height: 38px;
    border: 0;
    border-radius: 10px;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 12px;
    background: transparent;
    color: var(--text-2);
    font-size: 15px;
    font-weight: 600;
    text-align: left;
  }
  .nav.small {
    height: 34px;
  }
  .nav:hover {
    background: var(--surface-2);
  }
  .nav.active {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .grow {
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .count {
    font-size: 13px;
    color: var(--muted);
    font-variant-numeric: tabular-nums;
  }
  .count.accent {
    color: var(--accent-text);
    font-weight: 700;
  }
  .dot-wrap {
    width: 18px;
    display: flex;
    justify-content: center;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }
  .foot {
    margin-top: auto;
    padding-top: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .scan {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    border: 0;
    border-radius: 10px;
    background: transparent;
    color: var(--muted);
    font-size: 13.5px;
    font-weight: 600;
    text-align: left;
  }
  .scan:hover:not(:disabled) {
    background: var(--surface-2);
    color: var(--text-2);
  }
  .scan:disabled {
    opacity: 1;
  }
  .update {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 8px 9px 12px;
    border: 0;
    border-radius: 10px;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 13.5px;
    font-weight: 700;
    text-align: left;
  }
  .update:hover {
    filter: brightness(1.12);
  }
  .update .go {
    padding: 3px 9px;
    border-radius: 99px;
    background: var(--accent);
    color: var(--accent-ink);
    font-size: 12px;
  }
  .spin {
    display: flex;
  }
  .spin.on {
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .bp {
    height: 46px;
    border-radius: 12px;
    border: 1px solid color-mix(in oklab, var(--accent) 55%, transparent);
    background: var(--accent-soft);
    color: var(--accent-text);
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    font-size: 16px;
    font-weight: 700;
  }
  .bp:hover {
    background: color-mix(in oklab, var(--accent) 24%, transparent);
  }
</style>
