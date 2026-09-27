<script lang="ts">
  // A game's page in big picture (△ / Y on a game): details and the few
  // choices worth making from the couch. A game matched by its folder name
  // only is checked here: it's the right game, another one (picked from the
  // Steam store), or not a game at all.
  import GameArt from "../components/GameArt.svelte";
  import Icon, { type IconName } from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { metaLine, padSummary } from "../lib/bp";
  import { feedback, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import { savesSummary } from "../lib/saves";
  import { storeName, title, type AddonBadge, type Game, type Saves, type StoreHit } from "../lib/types";
  import ArtPicker from "./ArtPicker.svelte";
  import CollectionPicker from "./CollectionPicker.svelte";
  import Hints from "./Hints.svelte";

  let { game, onplay, onclose }: { game: Game; onplay: () => void; onclose: () => void } = $props();

  const modes = ["", "native", "steam"] as const;
  const modeLabel: Record<string, string> = { "": "Auto", native: "Native", steam: "Steam Input" };

  let b = $state(0);
  let logoFailed = $state(false);

  let addonBadges = $state<AddonBadge[]>([]);
  $effect(() => {
    const id = game.id;
    let live = true;
    addonBadges = [];
    if (game.installed)
      api.addons
        .forGame(id)
        .then((r) => live && (addonBadges = r.flatMap((a) => a.badges).slice(0, 4)))
        .catch(() => {});
    return () => (live = false);
  });

  let saves = $state<Saves | null>(null);
  const savesInfo = $derived(savesSummary(saves));
  $effect(() => {
    const id = game.id;
    let live = true;
    if (game.installed) api.saves.get(id).then((s) => live && (saves = s)).catch(() => {});
    return () => (live = false);
  });

  type Button = { id: string; label: string; icon: IconName };
  const buttons = $derived<Button[]>(
    game.needsReview
      ? [
          { id: "right", label: "That's right", icon: "check" },
          { id: "pick", label: "Pick the right game", icon: "link" },
          { id: "hide", label: "Not a game", icon: "eyeOff" },
          { id: "play", label: "Play", icon: "play" },
        ]
      : [
          { id: "play", label: game.installed ? "Play" : game.installUri ? "Install with " + storeName(game) : "Not installed", icon: "play" },
          { id: "fav", label: game.favorite ? "Favorite" : "Add to favorites", icon: "star" },
          { id: "pad", label: `Controller: ${modeLabel[game.padMode ?? ""]}`, icon: "pad" },
          { id: "art", label: "Art", icon: "image" },
          { id: "coll", label: game.collections?.length ? `Collections · ${game.collections.length}` : "Collections", icon: "layers" },
        ],
  );
  $effect(() => {
    if (b >= buttons.length) b = 0;
  });

  // The store picker: Steam's results for the name, and for the folder's
  // name when that's different.
  let picking = $state(false);
  let hits = $state<StoreHit[] | null>(null);
  let h = $state(0);
  async function pick() {
    picking = true;
    hits = null;
    h = 0;
    const folder = game.dir.split(/[\\/]/).filter(Boolean).pop() ?? "";
    const terms = [...new Set([title(game), folder.replace(/[._]+/g, " ").replace(/\s*[[(].*$/, "").trim()].filter((t) => t.length > 1))];
    const seen = new Set<number>();
    const out: StoreHit[] = [];
    for (const t of terms) {
      try {
        for (const x of await api.searchSteam(t)) if (!seen.has(x.appId)) (seen.add(x.appId), out.push(x));
      } catch {
        /* offline: an empty list says so */
      }
    }
    if (picking) hits = out.slice(0, 8);
  }
  function choose(x: StoreHit) {
    feedback.confirm();
    picking = false;
    lib.run(() => api.setMatch(game.id, x.appId, x.name).then(() => lib.toast(`${x.name}: fetching its details and art`)));
  }

  let choosingArt = $state(false);
  let choosingColl = $state(false);

  function press(id: string) {
    feedback.confirm();
    if (id === "art") {
      choosingArt = true;
      return;
    }
    if (id === "coll") {
      choosingColl = true;
      return;
    }
    if (id === "play" && game.installed) onplay();
    else if (id === "play" && game.installUri) lib.run(() => api.install(game.id).then(() => lib.toast(storeName(game) + " will install " + title(game))));
    else if (id === "fav") lib.run(() => api.setFavorite(game.id, !game.favorite));
    else if (id === "pad") {
      const next = modes[(modes.indexOf((game.padMode ?? "") as (typeof modes)[number]) + 1) % modes.length];
      lib.run(() => api.setPadMode(game.id, next));
    } else if (id === "right") lib.run(() => api.confirmMatch(game.id));
    else if (id === "pick") pick();
    else if (id === "hide") {
      lib.run(() => api.setHidden(game.id, true).then(() => lib.toast(`${title(game)} is hidden. Hidden games are in desktop mode.`)));
      onclose();
    }
  }

  $effect(() =>
    useInput((i) => {
      if (picking) {
        const n = hits?.length ?? 0;
        if (i === "up" || i === "down") {
          const j = h + (i === "up" ? -1 : 1);
          if (j >= 0 && j < n) ((h = j), feedback.move());
          else feedback.edge();
        } else if (i === "confirm" && hits?.[h]) choose(hits[h]);
        else if (i === "back") ((picking = false), feedback.move());
        else if (i === "menu" || i === "home") return false;
        return;
      }
      if (i === "left" || i === "right") {
        const j = b + (i === "left" ? -1 : 1);
        if (j >= 0 && j < buttons.length) ((b = j), feedback.move());
        else feedback.edge();
      } else if (i === "up" || i === "down") feedback.edge();
      else if (i === "confirm") press(buttons[b].id);
      else if (i === "back" || i === "info") onclose();
      else if (i === "menu" || i === "home") return false;
    }),
  );
  const pad = $derived(padSummary(game));
  const facts = $derived(
    [game.meta?.developers?.[0], game.meta?.releaseYear ? String(game.meta.releaseYear) : "", game.meta?.genres?.slice(0, 3).join(" · ")].filter(Boolean).join("  ·  "),
  );
</script>

<div class="sheet">
  <div class="art"><GameArt {game} kind="backdrop" /></div>
  <div class="shade"></div>
  <div class="content">
    <span class="src">{game.sourceLabel}</span>
    {#if game.meta?.logo && !logoFailed}
      <img class="logo" src={game.meta.logo} alt={title(game)} onerror={() => (logoFailed = true)} />
    {:else}
      <h1>{title(game)}</h1>
    {/if}
    <div class="line">{metaLine(game)}{facts ? `  ·  ${facts}` : ""}</div>
    {#if game.needsReview}
      <div class="check">
        <span class="q"><Icon name="warn" size={24} />Is this {title(game)}?</span>
        <span class="why">Seaglass found it by its folder name and couldn't match it to a known game for sure.</span>
        <span class="path">{game.dir}</span>
      </div>
    {:else if game.meta?.description}<p class="desc">{game.meta.description}</p>{/if}
    <div class="buttons">
      {#each buttons as bt, k (bt.id)}
        <button type="button" class="btn" class:primary={k === 0} class:on={k === b && !picking} onclick={() => ((b = k), press(bt.id))}>
          <Icon name={bt.icon} size={22} stroke={bt.icon === "pad" ? 1.8 : 2} />
          {bt.label}
        </button>
      {/each}
    </div>
    {#if !game.needsReview}<div class="note">{pad.long}</div>{/if}
    {#if addonBadges.length}
      <div class="badges">
        {#each addonBadges as bd (bd.text)}<span class="badge {bd.tone ?? 'info'}">{bd.text}</span>{/each}
      </div>
    {/if}
    {#if savesInfo && saves?.installed}
      <div class="note saves" class:warn={savesInfo.tone === "warn"}>Saves: {savesInfo.text}</div>
    {/if}
  </div>

  {#if picking}
    <div class="picker" role="dialog" aria-label="Pick the right game">
      <h2>Which game is it?</h2>
      <p class="sub">From the Steam store. The one you pick gives the game its name, details and art.</p>
      {#if hits === null}
        <p class="sub">Looking…</p>
      {:else if hits.length === 0}
        <p class="sub">The Steam store knows nothing by that name. Rename the game in desktop mode, or keep it as it is.</p>
      {:else}
        <ul>
          {#each hits as x, k (x.appId)}
            <li>
              <button type="button" class:on={k === h} onclick={() => choose(x)} onmouseenter={() => (h = k)}>
                <Icon name="link" size={22} />
                <span class="hn">{x.name}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  {#if choosingColl}
    <CollectionPicker {game} onclose={() => ((choosingColl = false), feedback.move())} />
  {/if}
  {#if choosingArt}
    <ArtPicker {game} onclose={() => ((choosingArt = false), feedback.move())} />
  {/if}

  <div class="hints">
    <Hints hints={picking ? [{ button: "confirm", label: "Choose" }, { button: "back", label: "Back" }] : [{ button: "confirm", label: "Select" }, { button: "back", label: "Back" }]} />
  </div>
</div>

<style>
  .sheet {
    position: absolute;
    inset: 0;
    z-index: 25;
    background: #06080b;
    color: #f3f5f7;
    animation: in 0.3s ease both;
  }
  @keyframes in {
    from {
      opacity: 0;
      transform: scale(1.01);
    }
  }
  .art {
    position: absolute;
    inset: 0;
  }
  .shade {
    position: absolute;
    inset: 0;
    background:
      linear-gradient(90deg, rgba(6, 8, 11, 0.95) 0%, rgba(6, 8, 11, 0.85) 30%, rgba(6, 8, 11, 0.62) 52%, rgba(6, 8, 11, 0.1) 80%),
      linear-gradient(0deg, rgba(6, 8, 11, 0.9) 0%, rgba(6, 8, 11, 0) 50%);
  }
  .content {
    position: absolute;
    left: 110px;
    bottom: 150px;
    width: 1500px;
    display: flex;
    flex-direction: column;
    gap: 22px;
  }
  .src {
    width: fit-content;
    padding: 7px 14px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.24);
    font-size: 15px;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 110px;
    line-height: 0.95;
  }
  .logo {
    max-width: 720px;
    max-height: 240px;
    object-fit: contain;
    object-position: left bottom;
    filter: drop-shadow(0 6px 30px rgba(0, 0, 0, 0.6));
  }
  .line {
    font-size: 22px;
    color: rgba(243, 245, 247, 0.82);
  }
  .desc {
    margin: 0;
    font-size: 21px;
    line-height: 1.5;
    color: rgba(243, 245, 247, 0.75);
    max-width: 860px;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .check {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 22px 26px;
    border-radius: 20px;
    background: rgba(255, 210, 138, 0.1);
    border: 1px solid rgba(255, 210, 138, 0.3);
    max-width: 900px;
  }
  .q {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 26px;
    font-weight: 800;
    color: #ffd28a;
  }
  .why {
    font-size: 19px;
    color: rgba(243, 245, 247, 0.8);
  }
  .path {
    font-size: 17px;
    color: rgba(243, 245, 247, 0.55);
    overflow-wrap: anywhere;
  }
  .buttons {
    display: flex;
    flex-wrap: wrap;
    gap: 14px;
    margin-top: 6px;
  }
  .btn {
    height: 72px;
    padding: 0 26px;
    border-radius: 36px;
    border: 1px solid rgba(255, 255, 255, 0.2);
    background: rgba(14, 18, 24, 0.6);
    color: #f3f5f7;
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 22px;
    font-weight: 700;
  }
  .btn.primary {
    background: #f3f5f7;
    color: #06080b;
    border-color: transparent;
    padding: 0 40px;
  }
  .btn.on {
    box-shadow:
      0 0 0 4px #06080b,
      0 0 0 7px #f3f5f7;
  }
  .note {
    font-size: 18px;
    color: rgba(243, 245, 247, 0.6);
  }
  .badges {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
    margin-top: -8px;
  }
  .badge {
    font-size: 17px;
    font-weight: 700;
    padding: 5px 14px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.1);
    color: #dfe6ec;
  }
  .badge.ok {
    background: color-mix(in oklab, oklch(0.8 0.12 205) 22%, transparent);
    color: oklch(0.88 0.09 205);
  }
  .badge.warn {
    background: rgba(255, 210, 138, 0.16);
    color: #ffd28a;
  }
  .note.saves {
    margin-top: -12px;
  }
  .note.warn {
    color: #ffd28a;
  }
  .picker {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: 720px;
    padding: 70px 56px 130px;
    display: flex;
    flex-direction: column;
    gap: 14px;
    background: rgba(12, 17, 23, 0.97);
    border-left: 1px solid rgba(255, 255, 255, 0.08);
    box-shadow: -30px 0 80px rgba(0, 0, 0, 0.5);
    animation: slide 0.3s cubic-bezier(0.2, 0.8, 0.2, 1) both;
    z-index: 2;
  }
  @keyframes slide {
    from {
      transform: translateX(100%);
    }
  }
  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 44px;
  }
  .sub {
    margin: 0;
    font-size: 19px;
    line-height: 1.45;
    color: #9ba8b5;
  }
  ul {
    list-style: none;
    margin: 14px 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
    overflow: hidden;
  }
  li button {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 18px;
    padding: 18px 20px;
    border: 0;
    border-radius: 16px;
    background: #121a23;
    color: #e8edf2;
    text-align: left;
    font-size: 21px;
    font-weight: 700;
  }
  li button.on {
    background: #1b2632;
    box-shadow: 0 0 0 3px #fff;
  }
  .hn {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 48px;
    z-index: 3;
  }
</style>
