<script lang="ts">
  // Choosing a game's art from the couch: its cover, its background (the
  // backdrop behind big picture), its banner (the hero) and its logo, from
  // what the stores, PCGamingWiki and SteamGridDB (with a key) have. L1 / R1
  // switch between them, the D-pad picks, ✕ uses the picture.
  import { untrack } from "svelte";
  import { api } from "../lib/api";
  import { feedback, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import { title, type ArtChoice, type ArtKind, type Game } from "../lib/types";
  import Glyph from "./Glyph.svelte";
  import Hints from "./Hints.svelte";

  let { game, onclose }: { game: Game; onclose: () => void } = $props();

  const kinds: { id: ArtKind; label: string; cols: number }[] = [
    { id: "cover", label: "Cover", cols: 6 },
    { id: "backdrop", label: "Background", cols: 3 },
    { id: "hero", label: "Banner", cols: 2 },
    { id: "logo", label: "Logo", cols: 3 },
  ];
  let kind = $state(0);
  let i = $state(0);
  // Every kind's pictures, fetched once each while the picker is open.
  let found = $state<Record<string, ArtChoice[] | "error" | undefined>>({});
  const list = $derived(found[kinds[kind].id]);
  const choices = $derived(Array.isArray(list) ? list : []);
  const cols = $derived(kinds[kind].cols);
  const current = $derived(game.meta?.[kinds[kind].id]);

  // A request keeps going when the kind changes (they take seconds) or
  // the game object is swapped for a refreshed one; each is asked once.
  const asked = new Set<ArtKind>();
  let alive = true;
  $effect(() => () => (alive = false));
  $effect(() => {
    const k = kinds[kind].id;
    if (asked.has(k)) return;
    asked.add(k);
    untrack(() =>
      api
        .artChoices(game.id, k)
        .then((c) => alive && (found = { ...found, [k]: c }))
        .catch(() => alive && (found = { ...found, [k]: "error" })),
    );
  });
  $effect(() => {
    kinds[kind];
    i = 0;
  });

  // Without a SteamGridDB key there are only the stores' few pictures.
  let sgdb = $state(true);
  $effect(() => {
    api.hasSteamGridDBKey().then((k) => (sgdb = k)).catch(() => {});
  });

  let busy = $state(false);
  async function use(c: ArtChoice) {
    if (busy) return;
    if (c.art === current) {
      feedback.edge();
      return;
    }
    busy = true;
    feedback.confirm();
    try {
      await api.setArt(game.id, kinds[kind].id, c.art);
      lib.toast(`${kinds[kind].label} changed`);
    } catch (e) {
      lib.toast(String((e as Error)?.message ?? e));
    } finally {
      busy = false;
    }
  }

  let grid: HTMLDivElement | undefined = $state();
  $effect(() => {
    grid?.querySelector<HTMLElement>(`[data-i="${i}"]`)?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  });

  $effect(() =>
    useInput((intent) => {
      const n = choices.length;
      const move = (j: number) => {
        if (j >= 0 && j < n) ((i = j), feedback.move());
        else feedback.edge();
      };
      switch (intent) {
        case "left":
          if (i % cols > 0) move(i - 1);
          else feedback.edge();
          return;
        case "right":
          if (i % cols < cols - 1) move(i + 1);
          else feedback.edge();
          return;
        case "up":
          move(i - cols);
          return;
        case "down":
          move(i + cols >= n && Math.floor(i / cols) < Math.floor((n - 1) / cols) ? n - 1 : i + cols);
          return;
        case "lb":
        case "rb": {
          const k = kind + (intent === "lb" ? -1 : 1);
          if (k >= 0 && k < kinds.length) ((kind = k), feedback.move());
          else feedback.edge();
          return;
        }
        case "confirm":
          if (choices[i]) use(choices[i]);
          return;
        case "back":
          onclose();
          return;
        case "menu":
        case "home":
          return false;
      }
    }),
  );
</script>

<div class="picker" role="dialog" aria-label="Choose art">
  <div class="head">
    <h2>Art for {title(game)}</h2>
    <nav class="tabs" aria-label="Kind of picture">
      <Glyph button="lb" size={30} />
      {#each kinds as k, n (k.id)}
        <button type="button" class="tab" class:on={n === kind} tabindex="-1" onclick={() => (kind = n)}>{k.label}</button>
      {/each}
      <Glyph button="rb" size={30} />
    </nav>
  </div>
  {#if list === undefined}
    <p class="msg">Looking for pictures…</p>
  {:else if list === "error" || choices.length === 0}
    <p class="msg">
      {list === "error" ? "Couldn't reach the stores. Try again in a moment." : "Nothing found for this game."}
      {#if !sgdb}A SteamGridDB key (in desktop mode's Settings) adds many more pictures.{/if}
    </p>
  {:else}
    <div class="grid {kinds[kind].id}" style:--cols={cols} bind:this={grid}>
      {#each choices as c, k (c.art)}
        <button type="button" class="cell" class:on={k === i} class:current={c.art === current} data-i={k} onclick={() => ((i = k), use(c))}>
          <img src={c.art} alt="" decoding="async" draggable="false" />
          <span class="src">{c.art === current ? "In use" : c.source === "Current" ? "Before" : c.source}{c.width ? ` · ${c.width}×${c.height}` : ""}</span>
        </button>
      {/each}
    </div>
  {/if}
  {#if !sgdb && choices.length > 0}
    <p class="more">More pictures come with a free SteamGridDB key, added in desktop mode's Settings.</p>
  {/if}
  <div class="hints">
    <Hints hints={[{ button: "confirm", label: "Use" }, { button: "lb", also: "rb", label: "Cover, background, banner, logo" }, { button: "back", label: "Back" }]} />
  </div>
</div>

<style>
  .picker {
    position: absolute;
    inset: 0;
    z-index: 30;
    background: rgba(6, 8, 11, 0.97);
    display: flex;
    flex-direction: column;
    padding: 56px 110px 130px;
    animation: in 0.25s ease both;
  }
  @keyframes in {
    from {
      opacity: 0;
    }
  }
  .head {
    display: flex;
    align-items: center;
    gap: 40px;
    margin-bottom: 34px;
  }
  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 44px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 50%;
  }
  .tabs {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .tabs > :global(:first-child) {
    margin-right: 8px;
  }
  .tabs > :global(:last-child) {
    margin-left: 8px;
  }
  .tab {
    padding: 10px 22px;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: rgba(232, 237, 242, 0.7);
    font-size: 22px;
    font-weight: 700;
  }
  .tab.on {
    background: rgba(255, 255, 255, 0.14);
    color: #fff;
  }
  .msg {
    font-size: 24px;
    color: rgba(232, 237, 242, 0.75);
    max-width: 900px;
  }
  .grid {
    flex: 1;
    min-height: 0;
    overflow: hidden;
    display: grid;
    grid-template-columns: repeat(var(--cols), minmax(0, 1fr));
    /* Rows as tall as their pictures: in a container shorter than the
       list, auto rows would shrink and the pictures overlap. */
    grid-auto-rows: max-content;
    gap: 26px;
    align-content: start;
    padding: 8px;
  }
  .cell {
    position: relative;
    padding: 0;
    border: 0;
    border-radius: 18px;
    overflow: hidden;
    background: #141a21;
    aspect-ratio: 16 / 9;
    box-shadow: 0 10px 26px rgba(0, 0, 0, 0.4);
    transition: transform 0.2s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .cover .cell {
    aspect-ratio: 2 / 3;
  }
  .hero .cell {
    aspect-ratio: 31 / 10;
  }
  .logo .cell {
    aspect-ratio: 2 / 1;
    background: linear-gradient(160deg, #26303b, #10151b);
  }
  .cell img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .logo .cell img {
    inset: 14%;
    width: 72%;
    height: 72%;
    object-fit: contain;
  }
  .cell.on {
    box-shadow:
      0 0 0 4px #f3f5f7,
      0 18px 40px rgba(0, 0, 0, 0.5);
    transform: scale(1.03);
  }
  .src {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    padding: 26px 14px 10px;
    background: linear-gradient(0deg, rgba(0, 0, 0, 0.8), transparent);
    font-size: 17px;
    font-weight: 600;
    text-align: left;
    color: #fff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cell.current .src {
    color: oklch(0.85 0.13 190);
  }
  .hints {
    position: absolute;
    right: 110px;
    bottom: 48px;
  }
  .more {
    position: absolute;
    left: 110px;
    bottom: 52px;
    margin: 0;
    font-size: 19px;
    color: rgba(232, 237, 242, 0.6);
  }
</style>
