<script lang="ts">
  // Deck layout (the default): a side rail and rows of games: continue
  // playing, new finds, and the library.
  import GameArt from "../components/GameArt.svelte";
  import Icon, { type IconName } from "../components/Icon.svelte";
  import Logo from "../components/Logo.svelte";
  import { byTitle, continuePlaying, metaLine, newFinds } from "../lib/bp";
  import { feedback, pad, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import { title, type Game } from "../lib/types";
  import Hints from "./Hints.svelte";
  import Sections, { type Section } from "./Sections.svelte";

  type Props = {
    width: number;
    height: number;
    onplay: (g: Game) => void;
    oninfo: (g: Game) => void;
    onfocus: (g: Game | null) => void;
    onlibrary: () => void;
    onsearch: () => void;
    onsettings: () => void;
    onfound: () => void;
    onmenu: () => void;
    ondesktop: () => void;
    onsection: (s: Section) => void;
    foundCount: number;
    checkCount: number;
  };
  let p: Props = $props();

  // Rows: the last item of the library row is "All games".
  const cont = $derived(continuePlaying(lib.base, 10));
  const fresh = $derived(newFinds(lib.base, 10));
  const all = $derived(lib.base.filter((g) => g.installed).sort(byTitle));
  const libRow = $derived(all.slice(0, 24));
  type Row = { id: "cont" | "fresh" | "lib"; label: string; sub?: string; games: Game[]; extra: boolean };
  const rows = $derived(
    [
      { id: "cont", label: cont.some((g) => (g.lastPlayed ?? 0) || (g.storeLastPlayed ?? 0)) ? "Continue playing" : "Your games", games: cont, extra: false },
      { id: "fresh", label: "New on this PC", sub: p.checkCount ? `${p.checkCount} matched by folder name only: open one to check it` : "Found in the last week", games: fresh, extra: false },
      { id: "lib", label: "Library", sub: `A–Z · ${all.length} games`, games: libRow, extra: true },
    ].filter((r) => r.games.length > 0 || r.extra) as Row[],
  );

  const rail: { id: string; label: string; icon: IconName; run: () => void; badge?: number }[] = $derived([
    { id: "home", label: "Home", icon: "grid", run: () => (zone = 0) },
    { id: "library", label: "Library", icon: "folder", run: p.onlibrary },
    { id: "search", label: "Search", icon: "search", run: p.onsearch },
    { id: "found", label: "New on this PC", icon: "scan", run: p.onfound, badge: p.checkCount },
    { id: "settings", label: "Settings", icon: "gear", run: p.onsettings },
    { id: "desktop", label: "Desktop mode", icon: "tv", run: p.ondesktop },
  ]);

  let zone = $state<number | "rail">(0); // row index or the rail
  let lastRow = $state(0);
  let idx = $state<number[]>([0, 0, 0]);
  let ri = $state(0);

  const rowLen = (r: Row) => r.games.length + (r.extra ? 1 : 0);
  const focusGame = $derived.by(() => {
    if (zone === "rail") return rows[lastRow]?.games[idx[lastRow]] ?? null;
    const r = rows[zone];
    return r?.games[idx[zone]] ?? null;
  });
  $effect(() => p.onfocus(focusGame));
  $effect(() => {
    if (typeof zone === "number" && zone >= rows.length) zone = Math.max(0, rows.length - 1);
  });

  function setIdx(row: number, v: number) {
    const next = [...idx];
    next[row] = v;
    idx = next;
  }

  $effect(() =>
    useInput((intent) => {
      if (zone === "rail") {
        if (intent === "up" || intent === "down") {
          const j = ri + (intent === "up" ? -1 : 1);
          if (j >= 0 && j < rail.length) ((ri = j), feedback.move());
          else feedback.edge();
        } else if (intent === "right" || intent === "back") {
          zone = lastRow;
          feedback.move();
        } else if (intent === "confirm") {
          feedback.confirm();
          const item = rail[ri];
          if (item.id === "home") zone = lastRow;
          else item.run();
        } else return false;
        return;
      }
      const r = rows[zone];
      if (!r) return false;
      const n = rowLen(r);
      const i = idx[zone];
      switch (intent) {
        case "left":
          if (i === 0) {
            lastRow = zone;
            zone = "rail";
          } else setIdx(zone, i - 1);
          feedback.move();
          return;
        case "right":
          if (i < n - 1) (setIdx(zone, i + 1), feedback.move());
          else feedback.edge();
          return;
        case "up":
          if (zone > 0) {
            zone = zone - 1;
            setIdx(zone, Math.min(idx[zone], rowLen(rows[zone]) - 1));
            feedback.move();
          } else feedback.edge();
          return;
        case "down":
          if (zone < rows.length - 1) {
            zone = zone + 1;
            setIdx(zone, Math.min(idx[zone], rowLen(rows[zone]) - 1));
            feedback.move();
          } else feedback.edge();
          return;
        case "confirm": {
          const g = r.games[i];
          // A game matched by folder name only opens its page, to check it.
          if (g?.needsReview) p.oninfo(g);
          else if (g) p.onplay(g);
          else if (r.extra) (feedback.confirm(), p.onlibrary());
          return;
        }
        case "info": {
          const g = r.games[i];
          if (g) p.oninfo(g);
          return;
        }
      }
      return false;
    }),
  );

  // The blurred glow behind the rows follows the selection once it rests:
  // a new blur over half the screen for every game flown past (a held
  // direction) costs more than it shows.
  let ambient = $state<Game | null>(null);
  $effect(() => {
    const cur = focusGame;
    if (!ambient) {
      ambient = cur;
      return;
    }
    const t = setTimeout(() => (ambient = cur), 180);
    return () => clearTimeout(t);
  });

  const railOpen = $derived(zone === "rail");
  const slide = $derived(typeof zone === "number" ? Math.max(0, zone - 1) * -340 : 0);
  const clock = $state({ v: "" });
  $effect(() => {
    const tick = () => {
      const d = new Date();
      clock.v = `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
    };
    tick();
    const t = setInterval(tick, 15000);
    return () => clearInterval(t);
  });

  const size = { cont: [520, 292, 24], fresh: [420, 244, 24], lib: [200, 300, 20] } as const;
  const offset = (row: Row, rIdx: number) => {
    const [w, , gap] = size[row.id];
    const lead = row.id === "lib" ? 4 : 1;
    return -Math.max(0, idx[rIdx] - lead) * (w + gap);
  };
</script>

<div class="deck">
  <div class="ambient">
    {#key ambient?.id}
      {#if ambient}<div class="amb"><GameArt game={ambient} kind="hero" /></div>{/if}
    {/key}
  </div>
  <div class="ambient-shade"></div>

  <div class="content" style:transform="translateY({slide}px)">
    <div class="header">
      <Sections current="home" onpick={p.onsection} />
      <div class="grow"></div>
      {#if pad.connected}
        <span class="status"><Icon name="pad" size={26} stroke={1.8} />{pad.battery >= 0 ? `${pad.battery}%` : pad.wireless ? "Bluetooth" : "USB"}</span>
      {/if}
      <span class="clock">{clock.v}</span>
    </div>

    {#each rows as row, r (row.id)}
      <section class="row" class:active={zone === r}>
        <div class="label">
          <h2>{row.label}</h2>
          {#if row.sub}<span>{row.sub}</span>{/if}
        </div>
        <div class="strip" style:transform="translateX({offset(row, r)}px)" style:gap="{size[row.id][2]}px">
          {#each row.games as g, k (g.id)}
            {@const on = zone === r && idx[r] === k}
            <button
              type="button"
              class="card {row.id}"
              class:on
              style:width="{size[row.id][0]}px"
              style:height="{size[row.id][1]}px"
              onclick={() => (on ? p.onplay(g) : ((zone = r), setIdx(r, k)))}
              aria-label={title(g)}
            >
              <GameArt game={g} kind={row.id === "lib" ? "cover" : "hero"} />
              {#if row.id === "lib"}
                {#if !g.meta?.cover}<span class="cover-title">{title(g)}</span>{/if}
              {:else}
                <span class="shade"></span>
                {#if row.id === "fresh"}<span class="badge" class:check={g.needsReview}>{g.needsReview ? "Check" : g.sourceLabel}</span>{/if}
                <span class="info">
                  {#if g.meta?.logo && row.id === "cont"}
                    <img class="logo" src={g.meta.logo} alt="" />
                  {:else}
                    <span class="name">{title(g)}</span>
                  {/if}
                  <span class="meta">{row.id === "fresh" ? (g.needsReview ? "Check: found by its folder name" : g.how) : metaLine(g)}</span>
                </span>
              {/if}
            </button>
          {/each}
          {#if row.extra}
            {@const on = zone === r && idx[r] === row.games.length}
            <button type="button" class="card more" class:on style:width="{size.lib[0]}px" style:height="{size.lib[1]}px" onclick={p.onlibrary}>
              <Icon name="grid" size={40} stroke={1.7} />
              <span>All games</span>
              <span class="muted">{all.length}</span>
            </button>
          {/if}
        </div>
      </section>
    {/each}
  </div>

  <nav class="rail" class:open={railOpen} aria-label="Big picture">
    <div class="brand"><Logo size={32} /><span class="fade">Seaglass</span></div>
    {#each rail as item, k (item.id)}
      <button type="button" class="rail-item" class:on={railOpen && ri === k} onclick={() => ((ri = k), item.run())}>
        <span class="ico"><Icon name={item.icon} size={28} stroke={1.8} />{#if item.badge}<span class="dot"></span>{/if}</span>
        <span class="fade">{item.label}</span>
        {#if item.badge}<span class="count fade">{item.badge}</span>{/if}
      </button>
    {/each}
  </nav>

  <div class="bar">
    <Hints
      left={focusGame ? `${title(focusGame)} · ${focusGame.sourceLabel}` : pad.connected ? pad.name : ""}
      hints={railOpen
        ? [
            { button: "confirm", label: "Open" },
            { button: "menu", label: "Quick access" },
          ]
        : [
            { button: "confirm", label: focusGame?.needsReview ? "Check" : "Play" },
            { button: "info", label: "Details" },
            { button: "menu", label: "Quick access" },
          ]}
    />
  </div>
</div>

<style>
  .deck {
    position: absolute;
    inset: 0;
    overflow: hidden;
    isolation: isolate;
  }
  .ambient {
    position: absolute;
    left: 0;
    right: 0;
    top: 0;
    height: 700px;
    overflow: hidden;
  }
  .amb {
    position: absolute;
    inset: -80px;
    opacity: 0.4;
    filter: blur(70px) saturate(1.2);
    animation: fade 0.6s ease both;
  }
  @keyframes fade {
    from {
      opacity: 0;
    }
  }
  .ambient-shade {
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, rgba(10, 14, 19, 0.35) 0%, rgba(10, 14, 19, 0.8) 38%, #0a0e13 62%);
  }
  .content {
    position: absolute;
    left: 160px;
    right: 0;
    top: 0;
    transition: transform 0.5s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .header {
    height: 56px;
    margin: 40px 64px 30px 0;
    display: flex;
    align-items: center;
    gap: 26px;
  }
  .grow {
    flex: 1;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 19px;
    font-weight: 600;
    color: #c9d3dc;
  }
  .clock {
    font-size: 24px;
    font-weight: 700;
  }
  .row {
    margin-bottom: 34px;
  }
  .label {
    display: flex;
    align-items: baseline;
    gap: 18px;
    margin-bottom: 16px;
  }
  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 30px;
    font-weight: 700;
  }
  .label span {
    font-size: 18px;
    color: #9ba8b5;
  }
  .strip {
    display: flex;
    transition: transform 0.45s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .card {
    position: relative;
    flex-shrink: 0;
    padding: 0;
    border: 0;
    border-radius: 16px;
    overflow: hidden;
    background: #121a23;
    color: #fff;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
    transition:
      transform 0.25s cubic-bezier(0.2, 0.8, 0.2, 1),
      box-shadow 0.25s;
  }
  .card.lib,
  .card.more {
    border-radius: 12px;
  }
  .card.on {
    transform: scale(1.05);
    box-shadow:
      0 0 0 3px #fff,
      0 18px 44px rgba(0, 0, 0, 0.55),
      0 0 50px color-mix(in oklab, var(--accent-game) 45%, transparent);
    z-index: 1;
  }
  .shade {
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, rgba(8, 12, 16, 0) 38%, rgba(8, 12, 16, 0.9) 100%);
  }
  .badge {
    position: absolute;
    top: 16px;
    left: 16px;
    padding: 6px 12px;
    border-radius: 8px;
    background: rgba(8, 12, 16, 0.78);
    font-size: 15px;
    font-weight: 700;
    letter-spacing: 0.03em;
  }
  .badge.check {
    background: #ffd28a;
    color: #1d1405;
  }
  .info {
    position: absolute;
    left: 22px;
    right: 22px;
    bottom: 18px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    text-align: left;
  }
  .logo {
    max-width: 62%;
    max-height: 96px;
    object-fit: contain;
    object-position: left bottom;
    filter: drop-shadow(0 3px 12px rgba(0, 0, 0, 0.6));
  }
  .name {
    font-family: var(--font-display);
    font-size: 34px;
    font-weight: 700;
    line-height: 1;
  }
  .meta {
    font-size: 16px;
    font-weight: 600;
    color: #c9d3dc;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cover-title {
    position: absolute;
    left: 10px;
    right: 10px;
    top: 16px;
    text-align: center;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 26px;
    line-height: 1;
    text-shadow: 0 2px 12px rgba(0, 0, 0, 0.5);
  }
  .card.more {
    border: 1px dashed rgba(255, 255, 255, 0.22);
    background: rgba(18, 26, 35, 0.7);
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    color: #c9d3dc;
    font-size: 20px;
    font-weight: 700;
  }
  .muted {
    color: #9ba8b5;
    font-weight: 600;
  }
  .rail {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 104px;
    z-index: 6;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 32px 0 110px;
    background: #0d1319;
    border-right: 1px solid rgba(255, 255, 255, 0.07);
    overflow: hidden;
    transition:
      width 0.32s cubic-bezier(0.2, 0.8, 0.2, 1),
      box-shadow 0.32s;
  }
  .rail.open {
    width: 320px;
    box-shadow: 24px 0 60px rgba(0, 0, 0, 0.55);
  }
  .brand {
    height: 64px;
    display: flex;
    align-items: center;
    gap: 16px;
    padding-left: 36px;
    margin-bottom: 18px;
    white-space: nowrap;
    font-family: var(--font-display);
    font-size: 28px;
    font-weight: 700;
  }
  .fade {
    opacity: 0;
    transition: opacity 0.25s;
    white-space: nowrap;
  }
  .rail.open .fade {
    opacity: 1;
  }
  .rail-item {
    position: relative;
    margin: 0 18px;
    height: 60px;
    border: 0;
    border-radius: 14px;
    display: flex;
    align-items: center;
    gap: 18px;
    padding: 0 16px;
    background: transparent;
    color: #c9d3dc;
    font-size: 21px;
    font-weight: 600;
    text-align: left;
  }
  .rail-item.on {
    box-shadow: 0 0 0 3px #fff;
    color: #fff;
  }
  .ico {
    position: relative;
    width: 36px;
    display: flex;
    justify-content: center;
    flex-shrink: 0;
  }
  .dot {
    position: absolute;
    top: -2px;
    right: 0;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: oklch(0.8 0.12 205);
  }
  .count {
    margin-left: auto;
    min-width: 30px;
    height: 30px;
    padding: 0 8px;
    border-radius: 15px;
    background: oklch(0.8 0.12 205);
    color: #071016;
    font-size: 16px;
    font-weight: 800;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .bar {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 76px;
    z-index: 7;
    display: flex;
    align-items: center;
    padding: 0 64px 0 140px;
    background: rgba(8, 12, 16, 0.94);
    border-top: 1px solid rgba(255, 255, 255, 0.07);
  }
  .bar > :global(*) {
    flex: 1;
  }
</style>
