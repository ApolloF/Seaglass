<script lang="ts">
  // Search from the couch: an on-screen keyboard on the left, results on the
  // right. A real keyboard works too.
  import GameArt from "../components/GameArt.svelte";
  import Icon from "../components/Icon.svelte";
  import { feedback, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import { title, type Game } from "../lib/types";
  import Hints from "./Hints.svelte";
  import { gridStep } from "./nav";
  import Sections, { type Section } from "./Sections.svelte";

  let {
    width,
    onplay,
    oninfo,
    onfocus,
    onback,
    onsection,
    active = true,
  }: { width: number; active?: boolean; onplay: (g: Game) => void; oninfo: (g: Game) => void; onfocus: (g: Game | null) => void; onback: () => void; onsection: (s: Section) => void } = $props();

  const KEYS = [..."1234567890", ..."qwertyuiop", ..."asdfghjkl'", ..."zxcvbnm-:.", "space", "del", "clear", "done"];
  const COLS = 10;
  let query = $state("");
  let zone = $state<"keys" | "results">("keys");
  let k = $state(11);
  let r = $state(0);
  // Typing on a real keyboard: Enter then goes to the results instead of
  // pressing the on-screen key under the highlight.
  let typing = $state(false);

  const norm = (s: string) => s.toLowerCase().normalize("NFKD").replace(/[^\p{L}\p{N}]+/gu, "");
  const results = $derived.by(() => {
    const q = norm(query);
    const base = lib.base.filter((g) => g.installed);
    if (!q) return [];
    // Each title is normalised once, not again in every comparison.
    return base
      .map((g) => ({ g, t: norm(title(g)) }))
      .filter((x) => x.t.includes(q))
      .sort((a, b) => Number(b.t.startsWith(q)) - Number(a.t.startsWith(q)) || a.g.sortTitle.localeCompare(b.g.sortTitle))
      .slice(0, RCOLS * 2)
      .map((x) => x.g);
  });
  $effect(() => onfocus(zone === "results" ? (results[r] ?? null) : (results[0] ?? null)));
  $effect(() => {
    results;
    r = Math.min(r, Math.max(0, results.length - 1));
  });

  function key(id: string) {
    feedback.move();
    if (id === "space") query += " ";
    else if (id === "del") query = query.slice(0, -1);
    else if (id === "clear") query = "";
    else if (id === "done") {
      if (results.length) zone = "results";
    } else query += id;
  }

  const RCOLS = $derived(Math.max(4, Math.floor((width - 960 - 110 + 22) / 214)));
  // The bottom row has 4 wide keys, laid out under columns 0-1, 2-4, 5-7, 8-9.
  const wideCols = [0, 2, 5, 8];
  const isWide = (idx: number) => idx >= 40;
  const colOf = (idx: number) => (isWide(idx) ? wideCols[idx - 40] : idx % COLS);

  function moveKeys(intent: string) {
    typing = false;
    if (isWide(k)) {
      if (intent === "left") k = k > 40 ? k - 1 : k;
      else if (intent === "right") {
        if (k < 43) k++;
        else if (results.length) ((zone = "results"), (r = 0));
      } else if (intent === "up") k = 30 + colOf(k);
      return;
    }
    if (intent === "down" && k >= 30) {
      const c = k % COLS;
      k = 40 + (c < 2 ? 0 : c < 5 ? 1 : c < 8 ? 2 : 3);
      return;
    }
    if (intent === "right" && k % COLS === COLS - 1) {
      if (results.length) ((zone = "results"), (r = 0));
      return;
    }
    const j = gridStep(k, 40, COLS, intent);
    if (j !== null) k = j;
  }

  $effect(() =>
    useInput((intent) => {
      if (intent === "back") {
        if (zone === "results") zone = "keys";
        else if (query) query = query.slice(0, -1);
        else onback();
        feedback.move();
        return;
      }
      if (zone === "keys") {
        if (intent === "confirm" && typing) {
          if (results.length) ((zone = "results"), (r = 0), feedback.move());
          else feedback.edge();
        } else if (intent === "confirm") key(KEYS[k]);
        else if (intent === "action") key("del");
        else if (intent === "info") key("space");
        else if (["up", "down", "left", "right"].includes(intent)) {
          moveKeys(intent);
          feedback.move();
        } else return false;
        return;
      }
      const g = results[r];
      if (intent === "confirm" && g) onplay(g);
      else if (intent === "info" && g) oninfo(g);
      else if (["up", "down", "left", "right"].includes(intent)) {
        if (intent === "left" && r % RCOLS === 0) zone = "keys";
        else {
          const j = gridStep(r, results.length, RCOLS, intent);
          if (j === null) return feedback.edge();
          r = j;
        }
        feedback.move();
      } else return false;
    }),
  );

  function onkeydown(e: KeyboardEvent) {
    // A sheet, quick access or the launch sequence on top gets its keys.
    if (!active || e.ctrlKey || e.altKey || e.metaKey) return;
    const t = e.target as HTMLElement | null;
    if (t?.tagName === "INPUT" || t?.tagName === "TEXTAREA") return;
    // Typed characters go into the search, not to the shortcuts (X, I, Q, …).
    const space = e.key === " " && typing && query !== "";
    if (space || (e.key.length === 1 && e.key !== " " && /[\p{L}\p{N}'\-:.&!]/u.test(e.key))) {
      query += e.key;
      zone = "keys";
      typing = true;
      e.stopPropagation();
      e.preventDefault();
    }
  }
</script>

<svelte:window onkeydowncapture={onkeydown} />

<div class="search">
  <div class="top"><Sections current="search" onpick={onsection} /></div>
  <div class="left">
    <div class="field" class:focus={zone === "keys"}>
      <Icon name="search" size={28} stroke={2} />
      <span class="q">{query}<span class="caret"></span></span>
    </div>
    <div class="keys">
      {#each KEYS as id, idx (id)}
        <button
          type="button"
          class="key"
          class:wide={isWide(idx)}
          class:on={zone === "keys" && idx === k}
          style:grid-column={isWide(idx) ? ["1 / span 2", "3 / span 3", "6 / span 3", "9 / span 2"][idx - 40] : undefined}
          onclick={() => ((k = idx), key(id))}
        >
          {id === "space" ? "Space" : id === "del" ? "⌫" : id === "clear" ? "Clear" : id === "done" ? "Done" : id}
        </button>
      {/each}
    </div>
  </div>
  <div class="right">
    {#if !query}
      <p class="hint">Type a title. Results appear here as you type.</p>
    {:else if results.length === 0}
      <p class="hint">Nothing matches “{query}”.</p>
    {:else}
      <div class="results" style:--cols={RCOLS}>
        {#each results as g, idx (g.id)}
          <button type="button" class="res" class:on={zone === "results" && idx === r} onclick={() => onplay(g)}>
            <span class="cover"><GameArt game={g} kind="cover" /></span>
            <span class="rt">{title(g)}</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>
  <div class="hints">
    <Hints
      hints={zone === "keys"
        ? [
            { button: "confirm", label: "Type" },
            { button: "action", label: "Delete" },
            { button: "info", label: "Space" },
            { button: "back", label: "Back" },
          ]
        : [
            { button: "confirm", label: "Play" },
            { button: "info", label: "Details" },
            { button: "back", label: "Keyboard" },
          ]}
    />
  </div>
</div>

<style>
  .search {
    position: absolute;
    inset: 0;
    background: #0a0e13;
    color: #e8edf2;
  }
  .top {
    position: absolute;
    left: 110px;
    top: 44px;
  }
  .left {
    position: absolute;
    left: 110px;
    top: 130px;
    width: 780px;
    display: flex;
    flex-direction: column;
    gap: 24px;
  }
  .field {
    height: 76px;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 0 24px;
    border-radius: 18px;
    background: #121a23;
    border: 2px solid transparent;
    font-size: 30px;
    font-weight: 600;
    color: #9ba8b5;
  }
  .field.focus {
    border-color: oklch(0.8 0.12 205);
  }
  .q {
    color: #fff;
  }
  .caret {
    display: inline-block;
    width: 3px;
    height: 34px;
    margin-left: 3px;
    vertical-align: middle;
    background: oklch(0.8 0.12 205);
    animation: blink 1s steps(1) infinite;
  }
  @keyframes blink {
    50% {
      opacity: 0;
    }
  }
  .keys {
    display: grid;
    grid-template-columns: repeat(10, minmax(0, 1fr));
    gap: 10px;
  }
  .key {
    height: 70px;
    border: 0;
    border-radius: 14px;
    background: #121a23;
    color: #e8edf2;
    font-size: 28px;
    font-weight: 600;
    text-transform: uppercase;
  }
  .key.wide {
    font-size: 22px;
    text-transform: none;
  }
  .key.on {
    background: #e8edf2;
    color: #0a0e13;
  }
  .right {
    position: absolute;
    left: 960px;
    right: 110px;
    top: 150px;
  }
  .hint {
    font-size: 24px;
    color: #9ba8b5;
  }
  .results {
    display: grid;
    grid-template-columns: repeat(var(--cols), minmax(0, 1fr));
    gap: 22px;
  }
  .res {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 0;
    border: 0;
    background: none;
    color: #9ba8b5;
    text-align: left;
  }
  .cover {
    position: relative;
    width: 100%;
    aspect-ratio: 2 / 3;
    border-radius: 12px;
    overflow: hidden;
    background: #121a23;
    transition: transform 0.2s;
  }
  .res.on .cover {
    transform: scale(1.05);
    box-shadow: 0 0 0 4px #fff;
  }
  .res.on .rt {
    color: #fff;
  }
  .rt {
    font-size: 19px;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 40px;
    left: 110px;
  }
</style>
