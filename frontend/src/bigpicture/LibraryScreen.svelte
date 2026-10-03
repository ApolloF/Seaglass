<script lang="ts">
  // Every game as a cover grid, with filters switched by L2 / R2. Rows
  // outside the view aren't rendered. With `review` it lists what's new on
  // this PC instead: games matched by folder name only come first, and
  // open their page to be checked.
  import GameArt from "../components/GameArt.svelte";
  import { byRecent, byTitle, newFinds } from "../lib/bp";
  import { feedback, useInput } from "../lib/input.svelte";
  import { inCollection, lib } from "../lib/store.svelte";
  import { title, type Game } from "../lib/types";
  import Glyph from "./Glyph.svelte";
  import Hints from "./Hints.svelte";
  import { gridStep } from "./nav";
  import Sections, { type Section } from "./Sections.svelte";

  let {
    width,
    height,
    games: only,
    review = false,
    onplay,
    oninfo,
    onfocus,
    onback,
    onsection,
  }: {
    width: number;
    height: number;
    games?: Game[];
    review?: boolean;
    onplay: (g: Game) => void;
    oninfo: (g: Game) => void;
    onfocus: (g: Game | null) => void;
    onback: () => void;
    onsection: (s: Section) => void;
  } = $props();

  // The filters, then the user's collections (L2 / R2 go through them all).
  const tabs = $derived([
    { id: "all", label: "All" },
    { id: "fav", label: "Favorites" },
    { id: "recent", label: "Recently played" },
    { id: "new", label: "New" },
    ...lib.collections.map((c) => ({ id: `c:${c.name}`, label: c.name })),
  ]);
  // The tab is kept by id: when a collection goes, the tab doesn't turn
  // into the one after it, it goes back to All.
  let tabId = $state("all");
  const tab = $derived(Math.max(0, tabs.findIndex((t) => t.id === tabId)));
  // With many collections only the tabs around the current one fit.
  const TABS_SHOWN = 7;
  const tabStart = $derived(Math.max(0, Math.min(tab - 3, tabs.length - TABS_SHOWN)));
  const games = $derived.by(() => {
    if (only) return only;
    const base = lib.base.filter((g) => g.installed);
    const id = tabs[tab]?.id ?? "all";
    if (id.startsWith("c:")) return base.filter((g) => inCollection(g, id.slice(2))).sort(byTitle);
    switch (id) {
      case "fav":
        return base.filter((g) => g.favorite).sort(byTitle);
      case "recent":
        return base.filter((g) => (g.lastPlayed ?? 0) > 0 || (g.storeLastPlayed ?? 0) > 0).sort(byRecent);
      case "new":
        return newFinds(base, 100);
      default:
        return [...base].sort(byTitle);
    }
  });
  const checks = $derived(games.filter((g) => g.needsReview).length);

  // Covers keep their size; a wider screen gets more of them per row.
  const SIDE = 110;
  const GAP = 26;
  const CW = 220;
  const COLS = $derived(Math.max(4, Math.floor((width - 2 * SIDE + GAP) / (CW + GAP))));
  const W = $derived((width - 2 * SIDE - GAP * (COLS - 1)) / COLS);
  const H = $derived(W * 1.5);
  const ROW = $derived(H + (review ? 84 : 64));
  const TOP = $derived(review ? 200 : 170);
  let i = $state(0);
  $effect(() => {
    games;
    i = Math.min(i, Math.max(0, games.length - 1));
  });
  const row = $derived(Math.floor(i / COLS));
  // The grid shows between the heading and the prompts along the bottom.
  const BOTTOM = 110;
  const visibleRows = $derived(Math.max(1, Math.floor((height - TOP - BOTTOM + 30) / ROW)));
  const firstRow = $derived(Math.max(0, Math.min(row - (visibleRows > 1 ? 1 : 0), Math.ceil(games.length / COLS) - visibleRows)));
  const shown = $derived(games.slice(Math.max(0, firstRow - 1) * COLS, (firstRow + visibleRows + 1) * COLS).map((g, k) => ({ g, k: Math.max(0, firstRow - 1) * COLS + k })));
  $effect(() => onfocus(games[i] ?? null));

  function open(g: Game) {
    // A game that needs a check opens its page, where it's checked.
    if (g.needsReview) oninfo(g);
    else onplay(g);
  }

  $effect(() =>
    useInput((intent) => {
      const g = games[i];
      switch (intent) {
        case "lt":
        case "rt":
          if (only) return;
          tabId = tabs[(tab + (intent === "lt" ? tabs.length - 1 : 1)) % tabs.length].id;
          i = 0;
          feedback.move();
          return;
        case "confirm":
          if (g) open(g);
          return;
        case "info":
          if (g) oninfo(g);
          return;
        case "back":
          onback();
          return;
        case "up":
        case "down":
        case "left":
        case "right": {
          const j = gridStep(i, games.length, COLS, intent);
          if (j !== null) {
            i = j;
            feedback.move();
          } else feedback.edge();
          return;
        }
      }
      return false;
    }),
  );
</script>

<div class="lib">
  <div class="head">
    {#if review}
      <h1>New on this PC</h1>
      <span class="count">{games.length} {games.length === 1 ? "game" : "games"}</span>
    {:else}
      <Sections current="library" onpick={onsection} />
      <span class="count">{games.length} {games.length === 1 ? "game" : "games"}</span>
      <div class="tabs">
        <Glyph button="lt" size={28} />
        {#if tabStart > 0}<span class="more">…</span>{/if}
        {#each tabs.slice(tabStart, tabStart + TABS_SHOWN) as t, n (t.id)}
          {@const k = tabStart + n}
          <button type="button" tabindex="-1" class:on={k === tab} onclick={() => ((tabId = t.id), (i = 0))}>{t.label}</button>
        {/each}
        {#if tabStart + TABS_SHOWN < tabs.length}<span class="more">…</span>{/if}
        <Glyph button="rt" size={28} />
      </div>
    {/if}
  </div>
  {#if review}
    <p class="intro">
      {#if checks}
        {checks === 1 ? "One game was" : `${checks} games were`} matched by folder name only. Open one to say whether it's the right game, pick another, or hide it if it isn't a game.
      {:else}
        Games found in the last week. Seaglass finds new games on its own, from stores, installers and your game folders.
      {/if}
    </p>
  {/if}
  {#if games.length === 0}
    <p class="empty" style:top="{TOP + 20}px">{review ? "Nothing new. Games you install show up here for a week." : tabs[tab]?.id === "fav" ? "No favorites yet. Add one from a game's page." : "Nothing here yet."}</p>
  {/if}
  <div class="view" style:top="{TOP - 24}px" style:bottom="{BOTTOM}px">
    <div class="grid" style:transform="translateY({-firstRow * ROW}px)" style:height="{Math.ceil(games.length / COLS) * ROW}px">
      {#each shown as { g, k } (g.id)}
        <button
          type="button"
          class="cell"
          class:on={k === i}
          style:width="{W}px"
          style:height="{H}px"
          style:left="{SIDE + (k % COLS) * (W + GAP)}px"
          style:top="{Math.floor(k / COLS) * ROW}px"
          onclick={() => (k === i ? open(g) : (i = k))}
          aria-label={title(g)}
        >
          <GameArt game={g} kind="cover" />
          {#if !g.meta?.cover}<span class="ct">{title(g)}</span>{/if}
          {#if review}<span class="badge" class:check={g.needsReview}>{g.needsReview ? "Check" : "New"}</span>{/if}
        </button>
        <span class="name" class:on={k === i} style:width="{W}px" style:left="{SIDE + (k % COLS) * (W + GAP)}px" style:top="{Math.floor(k / COLS) * ROW + H + 12}px">
          {title(g)}
          {#if review}<span class="how">{g.needsReview ? "Found by its folder name" : g.how}</span>{/if}
        </span>
      {/each}
    </div>
  </div>
  <div class="hints">
    <Hints
      hints={[
        { button: "confirm", label: games[i]?.needsReview ? "Check" : "Play" },
        { button: "info", label: "Details" },
        ...(only ? [] : [{ button: "lt" as const, also: "rt" as const, label: "Filter" }]),
        { button: "back", label: "Back" },
      ]}
    />
  </div>
</div>

<style>
  .lib {
    position: absolute;
    inset: 0;
    background: #0a0e13;
    color: #e8edf2;
    overflow: hidden;
  }
  .head {
    position: absolute;
    left: 110px;
    right: 110px;
    top: 44px;
    height: 60px;
    display: flex;
    align-items: center;
    gap: 22px;
    z-index: 2;
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 52px;
  }
  .count {
    font-size: 21px;
    color: #9ba8b5;
  }
  .tabs {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .tabs > :global(:first-child) {
    margin-right: 8px;
  }
  .tabs > :global(:last-child) {
    margin-left: 8px;
  }
  .tabs .more {
    color: rgba(232, 237, 242, 0.5);
    font-size: 20px;
    padding: 0 4px;
  }
  .tabs button {
    white-space: nowrap;
    height: 44px;
    padding: 0 18px;
    border: 0;
    border-radius: 22px;
    background: transparent;
    color: #9ba8b5;
    font-size: 19px;
    font-weight: 700;
  }
  .tabs button.on {
    background: rgba(79, 209, 232, 0.16);
    color: oklch(0.88 0.09 205);
  }
  .intro {
    position: absolute;
    left: 110px;
    top: 112px;
    max-width: 1300px;
    margin: 0;
    font-size: 21px;
    line-height: 1.45;
    color: #9ba8b5;
  }
  /* Room above the first row for the selected cover to grow into; the
     rows fade out before the prompts. */
  .view {
    position: absolute;
    left: 0;
    right: 0;
    overflow: hidden;
    mask-image: linear-gradient(180deg, #000 calc(100% - 50px), transparent 100%);
  }
  .grid {
    position: absolute;
    left: 0;
    right: 0;
    top: 24px;
    transition: transform 0.4s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .cell {
    position: absolute;
    padding: 0;
    border: 0;
    border-radius: 14px;
    overflow: hidden;
    background: #121a23;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
    transition:
      transform 0.22s cubic-bezier(0.2, 0.8, 0.2, 1),
      box-shadow 0.22s;
  }
  .cell.on {
    transform: scale(1.06);
    box-shadow:
      0 0 0 4px #fff,
      0 18px 44px rgba(0, 0, 0, 0.55);
    z-index: 1;
  }
  .ct {
    position: absolute;
    left: 12px;
    right: 12px;
    top: 18px;
    text-align: center;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 30px;
    line-height: 1;
    color: #fff;
    text-shadow: 0 2px 12px rgba(0, 0, 0, 0.5);
  }
  .badge {
    position: absolute;
    top: 12px;
    left: 12px;
    padding: 5px 11px;
    border-radius: 8px;
    background: rgba(8, 12, 16, 0.82);
    color: #fff;
    font-size: 15px;
    font-weight: 800;
    letter-spacing: 0.03em;
  }
  .badge.check {
    background: #ffd28a;
    color: #1d1405;
  }
  .name {
    position: absolute;
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 19px;
    font-weight: 600;
    color: #9ba8b5;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .name.on {
    color: #fff;
  }
  .how {
    font-size: 15px;
    font-weight: 500;
    color: #7f8c99;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .empty {
    position: absolute;
    left: 110px;
    font-size: 24px;
    color: #9ba8b5;
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 40px;
    left: 110px;
    z-index: 2;
  }
</style>
