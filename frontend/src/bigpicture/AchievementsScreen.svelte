<script lang="ts">
  // A game's achievements from the couch: the count, then every
  // achievement, unlocked ones first. Up / down moves through them; the
  // action button shows hidden ones.
  import { untrack } from "svelte";
  import AchievementIcon from "../components/AchievementIcon.svelte";
  import { achievementsSummary, masked, rarityText, shown, sortAchievements, statusText } from "../lib/achievements";
  import { feedback, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import { title, type Achievements, type Game } from "../lib/types";
  import Hints from "./Hints.svelte";
  import { step } from "./nav";

  let { game, list, onclose }: { game: Game; list: Achievements; onclose: () => void } = $props();

  const items = $derived(sortAchievements(list.items));
  const info = $derived(achievementsSummary(list));
  const canReveal = $derived(list.items.some((a) => masked(a, false)));
  let reveal = $state(untrack(() => lib.settings?.showHiddenAchievements ?? false));

  let i = $state(0);
  let ul: HTMLUListElement | undefined = $state();
  $effect(() => {
    ul?.querySelector<HTMLElement>(`[data-i="${i}"]`)?.scrollIntoView({ block: "nearest" });
  });

  $effect(() =>
    useInput((intent) => {
      switch (intent) {
        case "up":
        case "down": {
          const j = step(i, items.length, intent === "up" ? -1 : 1);
          if (j === null) feedback.edge();
          else ((i = j), feedback.move());
          return;
        }
        case "action":
          if (canReveal) ((reveal = !reveal), feedback.confirm());
          return;
        case "back":
        case "confirm":
          onclose();
          return;
        case "menu":
        case "home":
          return false;
      }
    }),
  );
</script>

<div class="screen" role="dialog" aria-label="Achievements of {title(game)}">
  <h2>Achievements</h2>
  <p class="sub">{title(game)}{info ? "  ·  " + info.text : ""}</p>
  {#if info && list.total > 0}
    <div class="bar"><span style:width="{info.pct}%"></span></div>
  {/if}
  {#if info?.detail}<p class="hint">{info.detail}</p>{/if}
  <ul bind:this={ul}>
    {#each items as raw, k (raw.id)}
      {@const a = shown(raw, reveal)}
      <li>
        <div class="row" class:on={k === i} class:locked={!a.unlocked} data-i={k} role="presentation" onmouseenter={() => (i = k)}>
          <AchievementIcon {a} size={76} masked={masked(raw, reveal)} />
          <div class="text">
            <span class="n">{a.name}</span>
            {#if a.desc}<span class="d">{a.desc}</span>{/if}
          </div>
          <div class="meta">
            {#if statusText(a)}<span class:done={a.unlocked}>{statusText(a)}</span>{/if}
            {#if rarityText(a)}<span class="r">{rarityText(a)}</span>{/if}
          </div>
        </div>
      </li>
    {/each}
  </ul>
  <div class="hints">
    <Hints hints={[...(canReveal ? [{ button: "action" as const, label: reveal ? "Hide hidden ones" : "Show hidden ones" }] : []), { button: "back", label: "Done" }]} />
  </div>
</div>

<style>
  .screen {
    position: absolute;
    inset: 0;
    z-index: 30;
    background: rgba(6, 8, 11, 0.96);
    padding: 70px 110px 140px;
    display: flex;
    flex-direction: column;
    animation: in 0.25s ease both;
  }
  @keyframes in {
    from {
      opacity: 0;
    }
  }
  h2 {
    margin: 0 0 6px;
    font-family: var(--font-display);
    font-size: 44px;
  }
  .sub {
    margin: 0 0 16px;
    font-size: 22px;
    font-weight: 600;
    color: rgba(232, 237, 242, 0.65);
  }
  .bar {
    height: 8px;
    max-width: 900px;
    border-radius: 99px;
    background: rgba(255, 255, 255, 0.1);
    overflow: hidden;
    margin-bottom: 14px;
  }
  .bar span {
    display: block;
    height: 100%;
    background: oklch(0.82 0.13 190);
  }
  .hint {
    margin: 0 0 14px;
    font-size: 20px;
    color: oklch(0.83 0.13 80);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 6px;
    flex: 1;
    min-height: 0;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    gap: 10px;
    max-width: 1300px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 22px;
    padding: 12px 20px;
    border-radius: 18px;
    background: rgba(255, 255, 255, 0.05);
    color: #e8edf2;
  }
  .row.on {
    box-shadow: 0 0 0 4px #f3f5f7;
    background: rgba(255, 255, 255, 0.09);
  }
  .row.locked .n {
    color: rgba(232, 237, 242, 0.75);
  }
  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .n {
    font-size: 26px;
    font-weight: 700;
  }
  .d {
    font-size: 19px;
    color: rgba(232, 237, 242, 0.6);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .meta {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 4px;
    font-size: 18px;
    font-weight: 600;
    color: rgba(232, 237, 242, 0.6);
    flex-shrink: 0;
  }
  .done {
    color: oklch(0.82 0.13 190);
  }
  .hints {
    position: absolute;
    right: 110px;
    bottom: 48px;
  }
</style>
