<script lang="ts">
  import Icon from "../components/Icon.svelte";
  import Titlebar from "../components/Titlebar.svelte";
  import { api } from "../lib/api";
  import { lib, type Sort } from "../lib/store.svelte";
  import CoverGrid from "./CoverGrid.svelte";
  import Details from "./Details.svelte";
  import LaunchPanel from "./LaunchPanel.svelte";
  import Settings from "./Settings.svelte";
  import Sidebar from "./Sidebar.svelte";

  let { onbigpicture }: { onbigpicture?: () => void } = $props();

  let settingsOpen = $state(false);
  let search: HTMLInputElement | undefined = $state();

  const titles: Record<string, string> = {
    all: "All games",
    installed: "Installed",
    notinstalled: "Not installed",
    favorites: "Favorites",
    recent: "Recently played",
    found: "New on this PC",
    hidden: "Hidden",
  };
  const heading = $derived(
    lib.filter.kind === "source"
      ? (lib.sources.find((s) => "source" in lib.filter && s.id === lib.filter.source)?.label ?? "Games")
      : lib.filter.kind === "collection"
        ? lib.filter.name
        : titles[lib.filter.kind],
  );

  // A collection's heading can rename or delete it.
  let renaming = $state(false);
  let newName = $state("");
  async function renameCollection() {
    if (!renaming) return; // Escape or Enter already ended it; the input's blur follows
    const f = lib.filter;
    if (f.kind !== "collection") return;
    const name = newName.trim();
    renaming = false;
    if (!name || name === f.name) return;
    const ok = await lib.run(() => api.renameCollection(f.name, name).then(() => true));
    if (ok) lib.filter = { kind: "collection", name };
  }
  async function deleteCollection() {
    const f = lib.filter;
    if (f.kind !== "collection") return;
    if (await lib.run(() => api.renameCollection(f.name, "").then(() => true))) {
      lib.toast(`Collection "${f.name}" deleted. Its games are still in your library.`);
      lib.filter = { kind: "all" };
    }
  }

  const sorts: { id: Sort; label: string }[] = [
    { id: "title", label: "Title" },
    { id: "recent", label: "Recently played" },
    { id: "added", label: "Recently added" },
    { id: "playtime", label: "Playtime" },
  ];

  function play(id: number) {
    const g = lib.games.find((x) => x.id === id);
    if (g?.installed) lib.play(g);
  }

  function onkeydown(e: KeyboardEvent) {
    if ((e.ctrlKey && e.key.toLowerCase() === "f") || (e.key === "/" && document.activeElement?.tagName !== "INPUT")) {
      e.preventDefault();
      search?.focus();
    } else if (e.key === "F5") {
      e.preventDefault();
      api.rescan();
    } else if (e.ctrlKey && e.key === ",") {
      e.preventDefault();
      settingsOpen = true;
    }
  }
</script>

<svelte:window {onkeydown} />

<div class="shell">
  <Titlebar />
  <div class="body">
    <Sidebar {onbigpicture} />

    <main>
      <div class="toolbar">
        <label class="search">
          <Icon name="search" size={18} stroke={2} />
          <span class="sr-only">Search games</span>
          <input bind:this={search} type="search" placeholder={`Search ${lib.counts.all} games`} bind:value={lib.query} onkeydown={(e) => e.key === "Escape" && (lib.query = "")} />
        </label>
        <label class="sort">
          <span class="sr-only">Sort by</span>
          <select bind:value={lib.sort} disabled={lib.filter.kind === "recent"}>
            {#each sorts as s (s.id)}<option value={s.id}>{s.label}</option>{/each}
          </select>
          <Icon name="chevronDown" size={14} stroke={2.2} />
        </label>
        <div class="grow"></div>
        <button type="button" class="tool" aria-label="Settings" title="Settings (Ctrl+,)" onclick={() => (settingsOpen = true)}><Icon name="gear" size={20} /></button>
      </div>

      <div class="heading">
        {#if renaming}
          <!-- svelte-ignore a11y_autofocus -->
          <input class="rename" bind:value={newName} autofocus onkeydown={(e) => (e.key === "Enter" ? renameCollection() : e.key === "Escape" && (renaming = false))} onblur={renameCollection} maxlength="40" aria-label="Collection name" />
        {:else}
          <h1>{heading}</h1>
        {/if}
        <span>{lib.visible.length} {lib.visible.length === 1 ? "game" : "games"}</span>
        {#if lib.filter.kind === "collection" && !renaming}
          <button type="button" class="coll-act" onclick={() => ((newName = heading), (renaming = true))}><Icon name="pencil" size={16} />Rename</button>
          <button type="button" class="coll-act" onclick={deleteCollection}><Icon name="trash" size={16} />Delete</button>
        {/if}
      </div>

      {#if !lib.loaded || (lib.scan.running && lib.games.length === 0)}
        <div class="empty">
          <span class="pulse"><Icon name="scan" size={40} stroke={1.6} /></span>
          <h2>Looking for games on this PC…</h2>
          <p>Steam, Epic, GOG, EA, Ubisoft, Xbox, other installers and game folders.</p>
        </div>
      {:else if lib.counts.all === 0 && lib.filter.kind === "all"}
        <div class="empty">
          <Icon name="folder" size={40} stroke={1.6} />
          <h2>No games found yet</h2>
          <p>If your games live in a folder of their own, add it and Seaglass will look inside.</p>
          <button type="button" class="primary" onclick={() => (settingsOpen = true)}>Add a game folder</button>
        </div>
      {:else if lib.visible.length === 0}
        <div class="empty">
          <Icon name={lib.query ? "search" : "grid"} size={40} stroke={1.6} />
          <h2>{lib.query ? `Nothing matches “${lib.query}”` : "Nothing here yet"}</h2>
          {#if lib.filter.kind === "found"}<p>New games and games that need a check show up here.</p>{/if}
          {#if lib.filter.kind === "favorites"}<p>Use the star on a game to add it here.</p>{/if}
        </div>
      {:else}
        <CoverGrid games={lib.visible} resetKey={`${JSON.stringify(lib.filter)}|${lib.query}|${lib.sort}`} selectedId={lib.selected?.id ?? null} onselect={(id) => (lib.selectedId = id)} onplay={play} />
      {/if}
    </main>

    {#if lib.selected}
      <Details game={lib.selected} />
    {/if}
  </div>
</div>

<LaunchPanel />
{#if settingsOpen}
  <Settings onclose={() => (settingsOpen = false)} />
{/if}

<style>
  .shell {
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  .body {
    flex: 1;
    min-height: 0;
    display: flex;
  }
  main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .toolbar {
    height: 68px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 28px;
  }
  .search {
    width: min(360px, 40%);
    height: 40px;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 12px;
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
    font-size: 15px;
  }
  .search input::placeholder {
    color: var(--muted);
  }
  .sort {
    position: relative;
    display: flex;
    align-items: center;
    color: var(--text-2);
  }
  .sort select {
    appearance: none;
    height: 40px;
    padding: 0 34px 0 14px;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: var(--surface-2);
    font-size: 14.5px;
    font-weight: 600;
  }
  .sort :global(svg) {
    position: absolute;
    right: 12px;
    pointer-events: none;
  }
  .grow {
    flex: 1;
  }
  .tool {
    width: 40px;
    height: 40px;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: var(--surface-2);
    color: var(--text-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .tool:hover {
    background: var(--surface-3);
  }
  .heading {
    display: flex;
    align-items: baseline;
    gap: 12px;
    padding: 0 28px 10px;
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 30px;
    font-weight: 700;
  }
  .rename {
    font-family: var(--font-display);
    font-size: 26px;
    font-weight: 700;
    padding: 2px 8px;
    border-radius: 8px;
    border: 1px solid var(--line-strong);
    background: var(--surface);
    color: var(--text);
  }
  .coll-act {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 8px;
    border: 1px solid var(--line);
    background: none;
    color: var(--muted);
    font: inherit;
    font-size: 13px;
    align-self: center;
  }
  .coll-act:hover {
    color: var(--text);
  }
  .heading span {
    color: var(--muted);
    font-weight: 500;
  }
  .empty {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 40px;
    text-align: center;
    color: var(--muted);
  }
  .empty h2 {
    margin: 8px 0 0;
    color: var(--text);
    font-family: var(--font-display);
    font-size: 26px;
  }
  .empty p {
    margin: 0;
    max-width: 440px;
  }
  .primary {
    margin-top: 12px;
    height: 44px;
    padding: 0 20px;
    border: 0;
    border-radius: 10px;
    background: var(--accent);
    color: var(--accent-ink);
    font-weight: 700;
    font-size: 15px;
  }
  .pulse {
    animation: pulse 1.6s ease-in-out infinite;
    color: var(--accent);
  }
  @keyframes pulse {
    50% {
      opacity: 0.45;
    }
  }
</style>
