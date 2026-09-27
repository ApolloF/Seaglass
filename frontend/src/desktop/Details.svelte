<script lang="ts">
  import GameArt from "../components/GameArt.svelte";
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { ago, bytes, clock, playtime } from "../lib/format";
  import { lib } from "../lib/store.svelte";
  import { lastPlayed, played, title, type Game } from "../lib/types";
  import { pad } from "../lib/input.svelte";
  import { padExplain } from "../lib/route";
  import { savesSummary } from "../lib/saves";
  import { sessionActive, storeName, type Saves } from "../lib/types";
  import MatchDialog from "./MatchDialog.svelte";

  let { game }: { game: Game } = $props();

  let matching = $state(false);
  let logoFailed = $state(false);
  $effect(() => {
    game.meta?.logo;
    logoFailed = false;
  });
  const m = $derived(game.meta);
  const facts = $derived(
    [
      m?.developers?.length ? m.developers[0] + (m.developers.length > 1 ? ` +${m.developers.length - 1}` : "") : "",
      m?.releaseYear ? String(m.releaseYear) : "",
      m?.genres?.slice(0, 3).join(" · "),
    ].filter(Boolean) as string[],
  );

  let menuOpen = $state(false);
  let renaming = $state(false);
  let draft = $state("");
  let input: HTMLInputElement | undefined = $state();

  $effect(() => {
    game.id;
    menuOpen = false;
    renaming = false;
  });

  const padMode = $derived(game.padMode || "auto");

  // Collections: chips to take the game out, a field (with the others as
  // suggestions) to put it in one, new or not.
  let collDraft = $state("");
  const setColl = (names: string[]) => lib.run(() => api.setCollections(game.id, names));
  function addColl() {
    const name = collDraft.trim();
    collDraft = "";
    if (!name) return;
    const known = lib.collections.find((c) => c.name.toLowerCase() === name.toLowerCase())?.name ?? name;
    setColl([...(game.collections ?? []), known]);
  }
  const padNote = $derived(padExplain(game, pad).long);

  // Saves, from Syncer: fetched when the game is shown and after it was played.
  let saves = $state<Saves | null>(null);
  const savesInfo = $derived(savesSummary(saves));
  $effect(() => {
    const id = game.id;
    const installed = game.installed;
    lib.session?.phase; // ask again once a game session ends
    saves = null;
    if (!installed) return;
    let live = true;
    const t = setTimeout(() => {
      api.saves
        .get(id, lib.session?.gameId === id && lib.session.phase === "ended")
        .then((s) => live && (saves = s))
        .catch(() => {});
    }, 250);
    return () => {
      live = false;
      clearTimeout(t);
    };
  });

  let installingSyncer = $state(false);
  async function installSyncer() {
    installingSyncer = true;
    await lib.run(() => api.saves.installSyncer());
    installingSyncer = false;
    saves = (await api.saves.get(game.id, true).catch(() => null)) ?? saves;
  }

  function startRename() {
    menuOpen = false;
    draft = title(game);
    renaming = true;
    queueMicrotask(() => input?.select());
  }

  async function saveRename() {
    renaming = false;
    const t = draft.trim();
    if (t && t !== title(game)) await lib.run(() => api.rename(game.id, t === game.title ? "" : t));
  }

  // The game's session, when it is being launched or played.
  const mine = $derived(sessionActive(lib.session) && lib.session?.gameId === game.id ? lib.session : null);
  const other = $derived(sessionActive(lib.session) && !mine ? lib.session : null);
  let quitting = $state(false);

  async function install() {
    const ok = await lib.run(() => api.install(game.id).then(() => true));
    if (ok) lib.toast(storeName(game) + " will install " + title(game));
  }

  async function quit() {
    if (!quitting) {
      quitting = true;
      setTimeout(() => (quitting = false), 4000);
      return;
    }
    quitting = false;
    await lib.run(() => api.launch.quitGame());
  }

  function onmenukey(e: KeyboardEvent) {
    if (e.key === "Escape") menuOpen = false;
  }
</script>

<aside class="details" aria-label="Game details">
  <div class="hero">
    {#key game.id}
      <div class="art-wrap"><GameArt {game} kind="hero" /></div>
    {/key}
    <div class="shade"></div>
    {#if renaming}
      <input
        class="rename"
        bind:this={input}
        bind:value={draft}
        aria-label="Game title"
        onkeydown={(e) => {
          if (e.key === "Enter") saveRename();
          if (e.key === "Escape") renaming = false;
        }}
        onblur={saveRename}
      />
    {:else if m?.logo && !logoFailed}
      <img class="logo" src={m.logo} alt={title(game)} draggable="false" onerror={() => (logoFailed = true)} />
    {:else}
      <h2 class="title">{title(game)}</h2>
    {/if}
  </div>

  <div class="body">
    <div class="actions">
      {#if mine?.phase === "running"}
        <div class="play playing" style:--glow={m?.accent ?? "transparent"} role="status">
          <span class="dot"></span>
          <span>Playing · {clock(mine.seconds)}</span>
        </div>
        <button type="button" class="square quit" class:armed={quitting} aria-label={quitting ? "Click again to quit the game" : "Quit game"} title={quitting ? "Click again to quit. Unsaved progress is lost." : "Quit game"} onclick={quit}>
          <Icon name="stop" size={18} />
        </button>
      {:else if mine}
        <button type="button" class="play" disabled>
          <span class="spinner"></span>
          <span>{mine.phase === "finishing" ? "Finishing…" : "Starting…"}</span>
        </button>
      {:else if game.installed}
        <button type="button" class="play" onclick={() => lib.play(game)} disabled={!!other} title={other ? `${other.title} is running` : undefined} style:--glow={m?.accent ?? "transparent"}>
          <Icon name="play" size={18} />
          <span>Play</span>
        </button>
      {:else if game.installUri}
        <button type="button" class="play install" onclick={install}>
          <Icon name="download" size={20} stroke={2} />
          <span>Install with {storeName(game)}</span>
        </button>
      {:else}
        <button type="button" class="play" disabled>
          <Icon name="cloudDown" size={20} stroke={2} />
          <span>Not installed</span>
        </button>
      {/if}
      <button
        type="button"
        class="square"
        class:fav={game.favorite}
        aria-label={game.favorite ? "Remove from favorites" : "Add to favorites"}
        aria-pressed={!!game.favorite}
        onclick={() => lib.run(() => api.setFavorite(game.id, !game.favorite))}
      >
        <svg width="22" height="22" viewBox="0 0 24 24" fill={game.favorite ? "currentColor" : "none"} stroke="currentColor" stroke-width="1.9" stroke-linejoin="round" aria-hidden="true"
          ><path d="M12 4l2.4 5 5.4.7-4 3.7 1 5.4L12 16.2 7.2 18.8l1-5.4-4-3.7 5.4-.7z" /></svg
        >
      </button>
      <div class="menu-wrap">
        <button type="button" class="square" aria-label="More actions" aria-haspopup="menu" aria-expanded={menuOpen} onclick={() => (menuOpen = !menuOpen)}>
          <Icon name="dots" size={22} />
        </button>
        {#if menuOpen}
          <div class="menu" role="menu" tabindex="-1" onkeydown={onmenukey}>
            {#if game.installed}
              <button type="button" role="menuitem" onclick={() => ((menuOpen = false), lib.run(() => api.openFolder(game.id)))}><Icon name="folder" size={18} />Open folder</button>
              <button type="button" role="menuitem" onclick={() => ((menuOpen = false), lib.run(() => api.chooseExe(game.id)))}><Icon name="file" size={18} />Choose program…</button>
            {/if}
            <button type="button" role="menuitem" onclick={startRename}><Icon name="pencil" size={18} />Rename</button>
            <button type="button" role="menuitem" onclick={() => ((menuOpen = false), (matching = true))}><Icon name="link" size={18} />Change game…</button>
            <button
              type="button"
              role="menuitem"
              onclick={async () => {
                menuOpen = false;
                if ((await lib.run(() => api.refreshMetadata(game.id).then(() => true))) === true) lib.toast("Fetching details and art…");
              }}><Icon name="refresh" size={18} />Refresh details and art</button
            >
            {#if game.needsReview}
              <button type="button" role="menuitem" onclick={() => ((menuOpen = false), lib.run(() => api.confirmMatch(game.id)))}><Icon name="check" size={18} />This is the right game</button>
            {/if}
            <button type="button" role="menuitem" onclick={() => ((menuOpen = false), lib.run(() => api.setHidden(game.id, !game.hidden)))}>
              <Icon name={game.hidden ? "eye" : "eyeOff"} size={18} />{game.hidden ? "Show in library" : "Hide from library"}
            </button>
          </div>
        {/if}
      </div>
    </div>

    <div class="stats">
      <div><span class="k">Playtime</span><span class="v">{playtime(played(game))}</span></div>
      <div><span class="k">Last played</span><span class="v">{ago(lastPlayed(game))}</span></div>
      <div><span class="k">Source</span><span class="v ellipsis" title={game.sourceLabel}>{game.sourceLabel}</span></div>
    </div>

    {#if m?.description || facts.length}
      <div class="about-game">
        {#if facts.length}<div class="facts">{facts.join("  ·  ")}</div>{/if}
        {#if m?.description}<p class="desc">{m.description}</p>{/if}
      </div>
    {/if}

    {#if game.needsReview}
      <div class="card review">
        <div class="card-head"><Icon name="warn" size={20} /><span>Is this {title(game)}?</span></div>
        <p>Seaglass found this game by its folder name and couldn't match it to a known game. Pick the right one, or keep it as it is.</p>
        <div class="row">
          <button type="button" class="btn primary" onclick={() => (matching = true)}>Find the game…</button>
          <button type="button" class="btn" onclick={() => lib.run(() => api.confirmMatch(game.id))}>Keep as is</button>
          <button type="button" class="btn" onclick={() => lib.run(() => api.setHidden(game.id, true))}>Not a game</button>
        </div>
      </div>
    {/if}

    <div class="card">
      <div class="card-head">
        <Icon name="layers" size={22} stroke={1.8} />
        <span class="grow">Collections</span>
      </div>
      <div class="chips">
        {#each game.collections ?? [] as c (c)}
          <span class="chip">
            <button type="button" class="chip-name" onclick={() => (lib.filter = { kind: "collection", name: c })} title="Show this collection">{c}</button>
            <button type="button" class="chip-x" aria-label="Take it out of {c}" onclick={() => setColl((game.collections ?? []).filter((x) => x !== c))}><Icon name="close" size={12} stroke={2.4} /></button>
          </span>
        {/each}
        <input class="chip-add" list="wl-collections" placeholder={game.collections?.length ? "Add to another…" : "Add to a collection…"} bind:value={collDraft} onkeydown={(e) => e.key === "Enter" && addColl()} onchange={addColl} maxlength="40" />
        <datalist id="wl-collections">
          {#each lib.collections.filter((c) => !game.collections?.some((x) => x.toLowerCase() === c.name.toLowerCase())) as c (c.name)}<option value={c.name}></option>{/each}
        </datalist>
      </div>
    </div>

    {#if game.installed}
      <div class="card">
        <div class="card-head">
          <Icon name="pad" size={22} stroke={1.8} />
          <span class="grow">Controller</span>
          <div class="seg" role="group" aria-label="Controller mode">
            {#each [["auto", "Auto"], ["native", "Native"], ["steam", "Steam Input"]] as [id, label] (id)}
              <button type="button" class:on={padMode === id} aria-pressed={padMode === id} onclick={() => lib.run(() => api.setPadMode(game.id, id))}>{label}</button>
            {/each}
          </div>
        </div>
        <p>{padNote}</p>
      </div>
      <div class="card saves" class:warn={savesInfo?.tone === "warn"}>
        <div class="card-head">
          <Icon name="cloudCheck" size={22} stroke={1.8} />
          <span class="grow">Saves</span>
          {#if savesInfo?.action === "get"}
            <button type="button" class="btn small" disabled={installingSyncer} onclick={installSyncer}
              >{installingSyncer ? "Installing…" : saves?.outdated ? "Update Syncer" : "Install Syncer"}</button
            >
          {:else if savesInfo?.action === "open"}
            <button type="button" class="btn small" onclick={() => lib.run(() => api.saves.openSyncer())}>Open Syncer</button>
          {/if}
        </div>
        {#if savesInfo}
          <strong class="saves-line">{savesInfo.text}</strong>
          <p>{savesInfo.detail}</p>
        {:else}
          <p>Asking Syncer…</p>
        {/if}
      </div>
    {/if}

    <dl class="about">
      <dt>Found</dt>
      <dd>{game.how}</dd>
      <dt>Identified</dt>
      <dd>{game.matchHow}{game.confidence < 100 ? ` (${game.confidence}% sure)` : ""}</dd>
      {#if game.emulator}<dt>Emulator</dt><dd>{game.emulator}</dd>{/if}
      {#if game.repacker}<dt>Repack</dt><dd>{game.repacker}</dd>{/if}
      {#if game.steamAppId}<dt>Steam app</dt><dd>{game.steamAppId}</dd>{/if}
      {#if game.installed}
        <dt>Folder</dt>
        <dd><button type="button" class="link ellipsis" title={game.dir} onclick={() => lib.run(() => api.openFolder(game.id))}>{game.dir}</button></dd>
        <dt>Starts</dt>
        <dd class="ellipsis" title={game.launchUri || game.exe}>{game.launchUri ? game.launchUri.split("?")[0] : game.exe ? game.exe.split("\\").pop() : "Nothing found yet"}</dd>
      {/if}
      {#if game.sizeBytes}<dt>Size</dt><dd>{bytes(game.sizeBytes)}</dd>{/if}
    </dl>
  </div>
</aside>

{#if matching}
  <MatchDialog {game} onclose={() => (matching = false)} />
{/if}

<style>
  .details {
    width: 420px;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    background: var(--surface);
    border-left: 1px solid var(--line);
    overflow: hidden;
  }
  .hero {
    position: relative;
    height: 226px;
    flex-shrink: 0;
    overflow: hidden;
  }
  .art-wrap {
    position: absolute;
    inset: 0;
    animation: fade 0.35s ease both;
  }
  @keyframes fade {
    from {
      opacity: 0.35;
    }
  }
  .shade {
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, rgba(13, 19, 25, 0) 35%, color-mix(in oklab, var(--surface) 96%, transparent) 100%);
  }
  .title,
  .rename {
    position: absolute;
    left: 22px;
    right: 22px;
    bottom: 14px;
    margin: 0;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 36px;
    line-height: 1;
    color: var(--text);
    text-wrap: balance;
  }
  .logo {
    position: absolute;
    left: 22px;
    bottom: 14px;
    max-width: calc(100% - 44px);
    max-height: 110px;
    object-fit: contain;
    object-position: left bottom;
    filter: drop-shadow(0 4px 18px rgba(0, 0, 0, 0.55));
  }
  .about-game {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .facts {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-2);
  }
  .desc {
    margin: 0;
    font-size: 14px;
    line-height: 1.5;
    color: var(--muted);
    display: -webkit-box;
    -webkit-line-clamp: 5;
    line-clamp: 5;
    -webkit-box-orient: vertical;
    overflow: hidden;
    user-select: text;
    -webkit-user-select: text;
  }
  .rename {
    padding: 4px 8px;
    border: 1px solid var(--accent);
    border-radius: var(--radius-s);
    background: var(--surface-2);
    outline: none;
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding: 14px 22px 22px;
  }
  .actions {
    display: flex;
    gap: 10px;
  }
  .play {
    flex: 1;
    height: 52px;
    border: 0;
    border-radius: var(--radius);
    background: var(--accent);
    color: var(--accent-ink);
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    font-size: 19px;
    font-weight: 700;
  }
  .play {
    box-shadow: 0 10px 34px -8px color-mix(in oklab, var(--glow) 70%, transparent);
  }
  .play:hover:not(:disabled) {
    filter: brightness(1.08);
  }
  .play:disabled {
    opacity: 0.75;
  }
  .playing {
    background: color-mix(in oklab, var(--accent) 16%, var(--surface-2));
    color: var(--accent-text);
    border: 1px solid color-mix(in oklab, var(--accent) 40%, transparent);
    font-variant-numeric: tabular-nums;
  }
  .dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: currentColor;
    animation: pulse 1.6s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
  .spinner {
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 2.5px solid currentColor;
    border-right-color: transparent;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(1turn);
    }
  }
  .square.quit.armed {
    background: var(--danger);
    border-color: var(--danger);
    color: #fff;
  }
  @media (prefers-reduced-motion: reduce) {
    .dot,
    .spinner {
      animation: none;
    }
  }
  .square {
    width: 52px;
    height: 52px;
    border-radius: var(--radius);
    border: 1px solid var(--line-strong);
    background: var(--surface-2);
    color: var(--text-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .square:hover {
    background: var(--surface-3);
  }
  .square.fav {
    color: oklch(0.85 0.14 85);
  }
  .menu-wrap {
    position: relative;
  }
  .menu {
    position: absolute;
    right: 0;
    top: 58px;
    z-index: 10;
    min-width: 230px;
    padding: 6px;
    border-radius: var(--radius);
    background: var(--surface-2);
    border: 1px solid var(--line-strong);
    box-shadow: var(--shadow);
    display: flex;
    flex-direction: column;
  }
  .menu button {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 38px;
    padding: 0 10px;
    border: 0;
    border-radius: var(--radius-s);
    background: transparent;
    font-size: 14.5px;
    font-weight: 600;
    text-align: left;
  }
  .menu button:hover {
    background: var(--surface-3);
  }
  .stats {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 10px;
  }
  .stats div {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .k {
    font-size: 12.5px;
    font-weight: 600;
    color: var(--muted);
  }
  .v {
    font-size: 16px;
    font-weight: 700;
  }
  .ellipsis {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 13px;
    font-weight: 600;
  }
  .chip-name,
  .chip-x {
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    padding: 4px 4px 4px 10px;
  }
  .chip-x {
    display: inline-flex;
    padding: 4px 8px 4px 2px;
    opacity: 0.7;
  }
  .chip-x:hover {
    opacity: 1;
  }
  .chip-add {
    flex: 1;
    min-width: 140px;
    padding: 5px 10px;
    border-radius: 999px;
    border: 1px dashed var(--line-strong);
    background: none;
    color: var(--text);
    font: inherit;
    font-size: 13px;
  }
  .card p {
    margin: 0;
    font-size: 13.5px;
    line-height: 1.45;
    color: var(--muted);
  }
  .card-head {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 15px;
    font-weight: 700;
    color: var(--text);
  }
  .saves-line {
    font-size: 14px;
    color: var(--text-2);
  }
  .card.saves.warn .saves-line {
    color: var(--warn);
  }
  .btn.small {
    height: 30px;
    padding: 0 12px;
    font-size: 13px;
  }
  .card.review {
    border: 1px solid color-mix(in oklab, var(--warn) 45%, transparent);
  }
  .card.review .card-head {
    color: var(--warn);
  }
  .grow {
    flex: 1;
  }
  .row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  .btn {
    height: 34px;
    padding: 0 12px;
    border-radius: var(--radius-s);
    border: 1px solid var(--line-strong);
    background: transparent;
    font-size: 13.5px;
    font-weight: 700;
  }
  .btn:hover {
    background: var(--surface-3);
  }
  .btn.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--accent-ink);
  }
  .seg {
    display: flex;
    gap: 2px;
    padding: 3px;
    border-radius: 9px;
    background: var(--bg);
  }
  .seg button {
    height: 28px;
    padding: 0 10px;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: var(--muted);
    font-size: 13px;
    font-weight: 700;
  }
  .seg button.on {
    background: var(--surface-3);
    color: var(--text);
  }
  .about {
    display: grid;
    grid-template-columns: 88px minmax(0, 1fr);
    gap: 7px 12px;
    margin: 0;
    font-size: 13.5px;
    line-height: 1.35;
  }
  dt {
    color: var(--muted);
  }
  dd {
    margin: 0;
    min-width: 0;
    color: var(--text-2);
  }
  .link {
    display: block;
    max-width: 100%;
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-text);
    text-align: left;
  }
  .link:hover {
    text-decoration: underline;
  }
</style>
