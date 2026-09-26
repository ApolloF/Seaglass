<script lang="ts">
  // A game as a round tile (Orbit): its key art with the logo over it, the
  // way a console draws a game's icon, instead of a cover cut into a circle
  // (which cuts its title in half). The stored tile is used when there is
  // one; otherwise it's put together here from the hero and logo, else the
  // cover. `big` swaps in the sharp full-size art underneath, for when a
  // tile opens up to fill a large circle.
  import { accentOf } from "../lib/bp";
  import { title, type Game } from "../lib/types";
  import GameArt from "./GameArt.svelte";

  let { game, big = false }: { game: Game; big?: boolean } = $props();

  const m = $derived(game.meta);
  const accent = $derived(accentOf(game) ?? "#35516b");
  const composed = $derived(!m?.tile && !!m?.hero && !!m?.logo);
  let logoFailed = $state(false);
  let heroFailed = $state(false);
  $effect(() => {
    game.id;
    logoFailed = heroFailed = false;
  });
  const large = $derived(m?.backdrop ?? m?.hero);
</script>

<span class="tile" style:--a={accent}>
  {#if m?.tile}
    <img class="fill" src={m.tile} alt="" decoding="async" draggable="false" />
  {:else if composed && !heroFailed}
    <img class="fill" src={m?.hero} alt="" decoding="async" draggable="false" onerror={() => (heroFailed = true)} />
    {#if !logoFailed}
      <span class="shade"></span>
      <img class="logo" src={m?.logo} alt="" decoding="async" draggable="false" onerror={() => (logoFailed = true)} />
    {/if}
  {:else if m?.cover}
    <img class="fill cover" src={m.cover} alt="" decoding="async" draggable="false" />
  {:else}
    <GameArt {game} kind="cover" />
    <span class="name">{title(game)}</span>
  {/if}
  {#if big && large}
    <img class="fill big" src={large} alt="" decoding="async" draggable="false" />
  {/if}
  <span class="gloss"></span>
</span>

<style>
  .tile {
    position: absolute;
    inset: 0;
    overflow: hidden;
    border-radius: inherit;
    /* While the art loads: the game's colour, not a black hole. */
    background:
      radial-gradient(circle at 32% 26%, color-mix(in oklab, var(--a) 70%, white) 0%, transparent 55%),
      linear-gradient(160deg, var(--a), color-mix(in oklab, var(--a) 35%, black));
  }
  .fill {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .cover {
    object-position: 50% 30%;
  }
  .big {
    animation: in 0.5s ease both;
  }
  @keyframes in {
    from {
      opacity: 0;
    }
  }
  .shade {
    position: absolute;
    inset: 0;
    background: radial-gradient(ellipse 70% 45% at 50% 58%, rgba(0, 0, 0, 0.45), transparent 75%);
  }
  .logo {
    position: absolute;
    left: 13%;
    right: 13%;
    top: 34%;
    width: 74%;
    height: 42%;
    object-fit: contain;
    filter: drop-shadow(0 3px 10px rgba(0, 0, 0, 0.6));
  }
  .big ~ .gloss {
    opacity: 0.5;
  }
  .name {
    position: absolute;
    inset: 22% 12%;
    display: flex;
    align-items: center;
    justify-content: center;
    text-align: center;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 27px; /* the tile is 240 pixels (Orbit), only ever scaled */
    line-height: 0.95;
    color: #fff;
    text-shadow: 0 1px 7px rgba(0, 0, 0, 0.6);
    overflow: hidden;
  }
  /* A soft light from above and a thin rim, like glass. */
  .gloss {
    position: absolute;
    inset: 0;
    border-radius: inherit;
    background: linear-gradient(165deg, rgba(255, 255, 255, 0.16) 0%, rgba(255, 255, 255, 0) 38%);
    box-shadow:
      inset 0 0 0 2px rgba(255, 255, 255, 0.12),
      inset 0 -29px 58px rgba(0, 0, 0, 0.22);
    pointer-events: none;
  }
</style>
