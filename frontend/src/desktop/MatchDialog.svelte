<script lang="ts">
  // "Change game…": search the Steam store and say which game this is.
  import { untrack } from "svelte";
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { errText, lib } from "../lib/store.svelte";
  import { title, type Game, type StoreHit } from "../lib/types";

  let { game, onclose }: { game: Game; onclose: () => void } = $props();

  let query = $state(untrack(() => title(game)));
  let hits = $state<StoreHit[]>([]);
  let busy = $state(false);
  let error = $state("");
  let input: HTMLInputElement | undefined = $state();
  let seq = 0;
  let searched = "";

  async function search() {
    const q = query.trim();
    if (!q) return;
    searched = q;
    const n = ++seq;
    busy = true;
    error = "";
    try {
      const r = await api.searchSteam(q);
      if (n === seq) hits = r;
    } catch (e) {
      if (n === seq) error = errText(e);
    } finally {
      if (n === seq) busy = false;
    }
  }

  async function pick(h: StoreHit) {
    const g = await lib.run(() => api.setMatch(game.id, h.appId, h.name));
    if (g) {
      lib.toast(`${title(game)} is now ${h.name}. Fetching its details…`);
      onclose();
    }
  }

  // Once, when it opens: search() reads the query, which would make this
  // run (and select the whole title) again on every key typed.
  $effect(() => {
    if (!input) return;
    input.focus();
    input.select();
    untrack(search);
  });

  // Results follow the title as it's typed.
  $effect(() => {
    const q = query;
    const t = setTimeout(() => q.trim() && q.trim() !== searched && search(), 400);
    return () => clearTimeout(t);
  });
</script>

<div class="scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <div class="dialog" role="dialog" aria-modal="true" aria-label="Change game" tabindex="-1" onkeydown={(e) => e.key === "Escape" && (e.preventDefault(), onclose())}>
    <div class="head">
      <h2>Which game is this?</h2>
      <button type="button" class="close" aria-label="Close" onclick={onclose}><Icon name="close" size={18} stroke={2.2} /></button>
    </div>
    <p class="hint">Search the Steam store and pick the right game. Seaglass uses it for the title, details and art, and remembers your choice.</p>
    <form
      class="search"
      onsubmit={(e) => {
        e.preventDefault();
        search();
      }}
    >
      <Icon name="search" size={18} stroke={2} />
      <input bind:this={input} bind:value={query} aria-label="Game title" placeholder="Game title" />
      <button type="submit" class="go" disabled={busy}>{busy ? "Searching…" : "Search"}</button>
    </form>
    <div class="results" aria-live="polite">
      {#if error}
        <p class="msg error">{error}</p>
      {:else if !busy && hits.length === 0}
        <p class="msg">No games found. Try a shorter title.</p>
      {:else}
        {#each hits as h (h.appId)}
          <button type="button" class="hit" onclick={() => pick(h)}>
            <span class="name">{h.name}</span>
            <span class="id">Steam {h.appId}</span>
          </button>
        {/each}
      {/if}
    </div>
  </div>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 60;
    background: var(--scrim);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .dialog {
    width: min(600px, calc(100vw - 64px));
    max-height: min(640px, calc(100vh - 96px));
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 22px 24px 24px;
    border-radius: var(--radius-l);
    background: var(--surface);
    border: 1px solid var(--line-strong);
    box-shadow: var(--shadow);
    outline: none;
  }
  .head {
    display: flex;
    align-items: center;
  }
  h2 {
    flex: 1;
    margin: 0;
    font-family: var(--font-display);
    font-size: 26px;
  }
  .close {
    width: 36px;
    height: 36px;
    border: 0;
    border-radius: 10px;
    background: var(--surface-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .hint {
    margin: 0;
    color: var(--muted);
    font-size: 14px;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 44px;
    padding: 0 6px 0 12px;
    border-radius: 10px;
    background: var(--surface-2);
    border: 1px solid var(--line);
    color: var(--muted);
  }
  .search:focus-within {
    border-color: var(--accent);
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--text);
    font-size: 15px;
  }
  .go {
    height: 32px;
    padding: 0 12px;
    border: 0;
    border-radius: 8px;
    background: var(--accent);
    color: var(--accent-ink);
    font-weight: 700;
    font-size: 14px;
  }
  .results {
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-height: 120px;
  }
  .hit {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    border: 0;
    border-radius: 10px;
    background: transparent;
    text-align: left;
  }
  .hit:hover,
  .hit:focus-visible {
    background: var(--surface-2);
  }
  .name {
    flex: 1;
    font-weight: 600;
  }
  .id {
    font-size: 13px;
    color: var(--muted);
    font-variant-numeric: tabular-nums;
  }
  .msg {
    margin: 12px 0;
    text-align: center;
    color: var(--muted);
  }
  .msg.error {
    color: var(--danger);
  }
</style>
