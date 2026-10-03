<script lang="ts">
  // Every achievement of a game: unlocked first (newest first), then the
  // locked ones, with rarity and unlock dates.
  import { untrack } from "svelte";
  import AchievementIcon from "../components/AchievementIcon.svelte";
  import Icon from "../components/Icon.svelte";
  import { achievementsSummary, masked, rarityText, shown, sortAchievements, statusText } from "../lib/achievements";
  import { lib } from "../lib/store.svelte";
  import { title, type Achievements, type Game } from "../lib/types";

  let { game, list, onclose }: { game: Game; list: Achievements; onclose: () => void } = $props();

  type Tab = "all" | "unlocked" | "locked";
  let tab = $state<Tab>("all");
  let reveal = $state(untrack(() => lib.settings?.showHiddenAchievements ?? false));
  let dialog: HTMLDivElement | undefined = $state();

  const info = $derived(achievementsSummary(list));
  const sorted = $derived(sortAchievements(list.items));
  const items = $derived(sorted.filter((a) => tab === "all" || (tab === "unlocked") === a.unlocked));
  const hiddenLocked = $derived(list.items.some((a) => masked(a, false)));

  $effect(() => {
    dialog?.focus();
  });
</script>

<div class="scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <div class="dialog" bind:this={dialog} role="dialog" aria-modal="true" aria-label="Achievements of {title(game)}" tabindex="-1" onkeydown={(e) => e.key === "Escape" && (e.preventDefault(), onclose())}>
    <div class="head">
      <div class="grow">
        <h2>Achievements</h2>
        <span class="sub">{title(game)}{info ? " · " + info.text : ""}</span>
      </div>
      <button type="button" class="close" aria-label="Close" onclick={onclose}><Icon name="close" size={18} stroke={2.2} /></button>
    </div>
    {#if info && list.total > 0}
      <div class="bar" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={info.pct} aria-label="Unlocked"><span style:width="{info.pct}%"></span></div>
    {/if}
    {#if list.hint}<p class="hint">{list.hint}</p>{/if}
    <div class="tools">
      <div class="seg" role="group" aria-label="Show">
        {#each [["all", `All ${list.total}`], ["unlocked", `Unlocked ${list.unlocked}`], ["locked", `Locked ${list.total - list.unlocked}`]] as [id, label] (id)}
          <button type="button" class:on={tab === id} aria-pressed={tab === id} onclick={() => (tab = id as Tab)}>{label}</button>
        {/each}
      </div>
      {#if hiddenLocked}
        <label class="reveal"><input type="checkbox" bind:checked={reveal} /> Show hidden ones</label>
      {/if}
    </div>
    <ul class="list">
      {#each items as raw (raw.id)}
        {@const a = shown(raw, reveal)}
        {@const hide = masked(raw, reveal)}
        <li class:locked={!a.unlocked}>
          <AchievementIcon {a} size={52} masked={hide} />
          <div class="text">
            <span class="name">{a.name}</span>
            {#if a.desc}<span class="desc">{a.desc}</span>{/if}
            {#if !a.unlocked && a.max}
              <span class="progress"><span style:width="{Math.min(100, ((a.progress ?? 0) / a.max) * 100)}%"></span></span>
            {/if}
          </div>
          <div class="meta">
            {#if statusText(a)}<span class="status" class:done={a.unlocked}>{statusText(a)}</span>{/if}
            {#if rarityText(a)}<span class="rarity" class:rare={(a.percent ?? 100) < 5}>{rarityText(a)}</span>{/if}
          </div>
        </li>
      {:else}
        <li class="empty">{list.total ? "Nothing here." : info?.detail || "No achievements."}</li>
      {/each}
    </ul>
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
    width: min(720px, calc(100vw - 64px));
    height: min(760px, calc(100vh - 96px));
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 22px 24px 18px;
    border-radius: var(--radius-l);
    background: var(--surface);
    border: 1px solid var(--line-strong);
    box-shadow: var(--shadow);
    outline: none;
  }
  .head {
    display: flex;
    align-items: flex-start;
    gap: 12px;
  }
  .grow {
    flex: 1;
    min-width: 0;
  }
  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 26px;
  }
  .sub {
    color: var(--muted);
    font-size: 14px;
    font-weight: 600;
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
  .bar,
  .progress {
    height: 6px;
    border-radius: 99px;
    background: var(--surface-3);
    overflow: hidden;
  }
  .bar span,
  .progress span {
    display: block;
    height: 100%;
    background: var(--accent);
    border-radius: inherit;
  }
  .progress {
    height: 4px;
    margin-top: 4px;
    max-width: 220px;
  }
  .hint {
    margin: 0;
    color: var(--warn);
    font-size: 13.5px;
  }
  .tools {
    display: flex;
    align-items: center;
    gap: 14px;
    flex-wrap: wrap;
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
  .reveal {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--muted);
  }
  .list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    margin: 0;
    padding: 0 4px 0 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  li {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 8px 10px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  li.locked .name {
    color: var(--text-2);
  }
  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .name {
    font-weight: 700;
    font-size: 14.5px;
  }
  .desc {
    color: var(--muted);
    font-size: 13px;
    line-height: 1.35;
  }
  .meta {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 2px;
    flex-shrink: 0;
    font-size: 12.5px;
    font-weight: 600;
    color: var(--muted);
    text-align: right;
  }
  .status.done {
    color: var(--accent-text);
  }
  .rarity.rare {
    color: oklch(0.8 0.13 85);
  }
  .empty {
    justify-content: center;
    color: var(--muted);
    padding: 24px;
    background: none;
  }
</style>
