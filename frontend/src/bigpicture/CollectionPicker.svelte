<script lang="ts">
  // Putting a game in collections from the couch: the collections there
  // are, plus a few common ones to start with, each switched on or off
  // with ✕. New names are typed in desktop mode.
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { feedback, useInput } from "../lib/input.svelte";
  import { collectionKey, lib } from "../lib/store.svelte";
  import { title, type Game } from "../lib/types";
  import Hints from "./Hints.svelte";

  let { game, onclose }: { game: Game; onclose: () => void } = $props();

  const starters = ["Playing now", "Backlog", "Finished", "Co-op", "Couch"];
  // The list as it was when the picker opened, so it doesn't reorder under
  // the cursor as games go in and out.
  const names = (() => {
    const have = lib.collections.map((c) => c.name);
    const keys = new Set(have.map(collectionKey));
    return [...have, ...starters.filter((s) => !keys.has(collectionKey(s)))];
  })();
  const isIn = (n: string) => !!game.collections?.some((c) => collectionKey(c) === collectionKey(n));
  const counts = $derived(new Map(lib.collections.map((c) => [collectionKey(c.name), c.count])));

  let i = $state(0);
  let list: HTMLUListElement | undefined = $state();
  $effect(() => {
    list?.querySelector<HTMLElement>(`[data-i="${i}"]`)?.scrollIntoView({ block: "nearest" });
  });

  function toggle(n: string) {
    feedback.confirm();
    const cur = game.collections ?? [];
    const next = isIn(n) ? cur.filter((c) => collectionKey(c) !== collectionKey(n)) : [...cur, n];
    lib.run(() => api.setCollections(game.id, next));
  }

  $effect(() =>
    useInput((intent) => {
      switch (intent) {
        case "up":
        case "down": {
          const j = i + (intent === "up" ? -1 : 1);
          if (j >= 0 && j < names.length) ((i = j), feedback.move());
          else feedback.edge();
          return;
        }
        case "confirm":
          if (names[i]) toggle(names[i]);
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

<div class="picker" role="dialog" aria-label="Collections">
  <h2>Collections for {title(game)}</h2>
  <p class="sub">Collections show as tabs in Library (L2 / R2) and in desktop mode's sidebar. New ones can be named in desktop mode.</p>
  <ul bind:this={list}>
    {#each names as n, k (n)}
      <li>
        <button type="button" class:on={k === i} class:yes={isIn(n)} data-i={k} onclick={() => ((i = k), toggle(n))} onmouseenter={() => (i = k)}>
          <span class="box">{#if isIn(n)}<Icon name="check" size={22} stroke={2.6} />{/if}</span>
          <span class="n">{n}</span>
          <span class="c">{counts.get(collectionKey(n)) ?? 0} {(counts.get(collectionKey(n)) ?? 0) === 1 ? "game" : "games"}</span>
        </button>
      </li>
    {/each}
  </ul>
  <div class="hints"><Hints hints={[{ button: "confirm", label: "In / out" }, { button: "back", label: "Done" }]} /></div>
</div>

<style>
  .picker {
    position: absolute;
    inset: 0;
    z-index: 30;
    background: rgba(6, 8, 11, 0.96);
    padding: 70px 110px 140px;
    animation: in 0.25s ease both;
  }
  @keyframes in {
    from {
      opacity: 0;
    }
  }
  h2 {
    margin: 0 0 10px;
    font-family: var(--font-display);
    font-size: 44px;
  }
  .sub {
    margin: 0 0 30px;
    font-size: 21px;
    color: rgba(232, 237, 242, 0.65);
    max-width: 1100px;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 6px;
    display: flex;
    flex-direction: column;
    gap: 10px;
    max-width: 900px;
    max-height: calc(100% - 150px);
    overflow: hidden;
  }
  li button {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 20px;
    padding: 18px 24px;
    border-radius: 18px;
    border: 0;
    background: rgba(255, 255, 255, 0.05);
    color: #e8edf2;
    font: inherit;
    font-size: 26px;
    font-weight: 700;
    text-align: left;
  }
  li button.on {
    box-shadow: 0 0 0 4px #f3f5f7;
    background: rgba(255, 255, 255, 0.09);
  }
  .box {
    width: 38px;
    height: 38px;
    border-radius: 10px;
    border: 2px solid rgba(255, 255, 255, 0.35);
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .yes .box {
    background: oklch(0.82 0.13 190);
    border-color: transparent;
    color: #06080b;
  }
  .n {
    flex: 1;
  }
  .c {
    font-size: 19px;
    font-weight: 600;
    color: rgba(232, 237, 242, 0.55);
  }
  .hints {
    position: absolute;
    right: 110px;
    bottom: 48px;
  }
</style>
