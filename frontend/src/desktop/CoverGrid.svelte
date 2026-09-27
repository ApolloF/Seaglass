<script lang="ts">
  // The cover grid. Only the rows in view (plus a few) are rendered, so a
  // library of thousands of games scrolls as smoothly as one of ten.
  import GameArt from "../components/GameArt.svelte";
  import Icon from "../components/Icon.svelte";
  import { playtime } from "../lib/format";
  import { isFresh } from "../lib/store.svelte";
  import { played, title, type Game } from "../lib/types";

  let {
    games,
    resetKey,
    selectedId,
    onselect,
    onplay,
  }: {
    games: Game[];
    /** Names the view shown (filter, search, sort); a new one scrolls to the top. */
    resetKey: string;
    selectedId: number | null;
    onselect: (id: number) => void;
    onplay: (id: number) => void;
  } = $props();

  const PAD = 28;
  const GAP_X = 18;
  const GAP_Y = 22;
  const MIN_W = 148;
  const TEXT_H = 42;

  let viewport: HTMLDivElement | undefined = $state();
  let width = $state(0);
  let height = $state(0);
  let scrollTop = $state(0);

  const cols = $derived(Math.max(2, Math.floor((width - 2 * PAD + GAP_X) / (MIN_W + GAP_X))));
  const coverW = $derived(Math.max(60, (width - 2 * PAD - GAP_X * (cols - 1)) / cols));
  const rowH = $derived(coverW * 1.5 + TEXT_H + GAP_Y);
  const rows = $derived(Math.ceil(games.length / cols));
  const first = $derived(Math.max(0, Math.floor(scrollTop / rowH) - 2));
  const last = $derived(Math.min(rows, Math.ceil((scrollTop + height) / rowH) + 2));
  const items = $derived(games.slice(first * cols, last * cols).map((g, j) => ({ g, i: first * cols + j })));

  const selIndex = $derived(games.findIndex((g) => g.id === selectedId));

  function ensureVisible(i: number) {
    if (!viewport || i < 0) return;
    const top = Math.floor(i / cols) * rowH;
    if (top < viewport.scrollTop) viewport.scrollTop = top;
    else if (top + rowH > viewport.scrollTop + height) viewport.scrollTop = top + rowH - height + 8;
  }

  function onkeydown(e: KeyboardEvent) {
    if (!games.length) return;
    const i = selIndex < 0 ? 0 : selIndex;
    let next = i;
    // At the grid's edge a key isn't used, so focus can move on from it
    // (a controller moves to the sidebar or the details that way).
    switch (e.key) {
      case "ArrowRight":
        if (i % cols === cols - 1 || i === games.length - 1) return;
        next = i + 1;
        break;
      case "ArrowLeft":
        if (i % cols === 0) return;
        next = i - 1;
        break;
      case "ArrowDown":
        if (Math.floor(i / cols) === rows - 1) return;
        next = Math.min(games.length - 1, i + cols);
        break;
      case "ArrowUp":
        if (i < cols) return;
        next = i - cols;
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = games.length - 1;
        break;
      case "Enter":
        e.preventDefault();
        onplay(games[i].id);
        return;
      default:
        return;
    }
    e.preventDefault();
    onselect(games[next].id);
    ensureVisible(next);
  }

  // A new view (filter, search or sort) scrolls back to the top; the same
  // view's list changing (a game unstarred, a scan adding one) doesn't.
  let lastKey = "";
  $effect(() => {
    if (resetKey !== lastKey && viewport) viewport.scrollTop = 0;
    lastKey = resetKey;
  });
</script>

<div
  class="viewport"
  bind:this={viewport}
  bind:clientWidth={width}
  bind:clientHeight={height}
  onscroll={() => (scrollTop = viewport?.scrollTop ?? 0)}
  role="listbox"
  aria-label="Games"
  tabindex="0"
  {onkeydown}
>
  <div class="spacer" style:height="{rows * rowH + PAD}px">
    {#each items as { g, i } (g.id)}
      <div
        class="cell"
        role="option"
        aria-selected={g.id === selectedId}
        style:width="{coverW}px"
        style:transform="translate({PAD + (i % cols) * (coverW + GAP_X)}px, {Math.floor(i / cols) * rowH + 6}px)"
      >
        <button
          type="button"
          class="cover"
          class:selected={g.id === selectedId}
          class:dim={!g.installed}
          style:height="{coverW * 1.5}px"
          tabindex="-1"
          aria-label={title(g)}
          onclick={() => onselect(g.id)}
          ondblclick={() => onplay(g.id)}
        >
          <GameArt game={g} kind="cover" />
          {#if !g.meta?.cover}
            <span class="shade"></span>
            <span class="ctitle">{title(g)}</span>
          {/if}
          {#if g.needsReview}
            <span class="badge review">Check</span>
          {:else if isFresh(g)}
            <span class="badge">New</span>
          {/if}
          {#if !g.installed}
            <span class="corner"><Icon name="cloudDown" size={15} stroke={2} /></span>
          {/if}
        </button>
        <span class="name">{title(g)}</span>
        <span class="sub">{g.installed ? (played(g) ? playtime(played(g)) : g.sourceLabel) : g.owned ? "Owned · " + g.sourceLabel : "Not installed"}</span>
      </div>
    {/each}
  </div>
</div>

<style>
  .viewport {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    outline: none;
  }
  .spacer {
    position: relative;
  }
  .cell {
    position: absolute;
    top: 0;
    left: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
    contain: layout style;
  }
  .cover {
    position: relative;
    width: 100%;
    padding: 0;
    border: 0;
    border-radius: var(--radius-s);
    overflow: hidden;
    background: var(--surface-2);
    box-shadow: 0 6px 18px rgba(0, 0, 0, 0.3);
    transition:
      transform 0.18s var(--ease),
      box-shadow 0.18s var(--ease);
  }
  .cover:hover {
    transform: translateY(-3px);
  }
  .cover.selected {
    box-shadow:
      0 0 0 3px var(--accent),
      var(--shadow);
  }
  .cover.dim {
    filter: saturate(0.35);
    opacity: 0.62;
  }
  .shade {
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, rgba(8, 12, 16, 0.6) 0%, rgba(8, 12, 16, 0) 45%);
  }
  .ctitle {
    position: absolute;
    left: 10px;
    right: 10px;
    top: 12px;
    text-align: center;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 21px;
    line-height: 1.02;
    color: #fff;
    text-wrap: balance;
    text-shadow: 0 2px 10px rgba(0, 0, 0, 0.55);
  }
  .badge {
    position: absolute;
    left: 8px;
    bottom: 8px;
    padding: 3px 8px;
    border-radius: 6px;
    background: var(--accent);
    color: var(--accent-ink);
    font-size: 11.5px;
    font-weight: 800;
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }
  .badge.review {
    background: #ffd28a;
    color: #2a1a00;
  }
  .corner {
    position: absolute;
    right: 8px;
    bottom: 8px;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: rgba(8, 12, 16, 0.8);
    color: #e8edf2;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .name {
    margin-top: 6px;
    font-size: 14.5px;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .sub {
    font-size: 12.5px;
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
