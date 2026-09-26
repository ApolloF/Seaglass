<script lang="ts">
  import { lib } from "../lib/store.svelte";
  import Icon from "./Icon.svelte";
</script>

<div class="toasts" aria-live="polite">
  {#each lib.toasts as t (t.id)}
    <div class="toast" class:error={t.tone === "error"} role={t.tone === "error" ? "alert" : "status"}>
      <Icon name={t.tone === "error" ? "warn" : "info"} size={18} />
      <span>{t.text}</span>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    right: 20px;
    bottom: 20px;
    z-index: 100;
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-width: 420px;
    pointer-events: none;
  }
  /* Big picture keeps its button prompts along the bottom: toasts sit
     above them, a little larger for the distance to a TV. */
  :global(html[data-mode="bigpicture"]) .toasts {
    right: 4vw;
    bottom: 12vh;
    max-width: min(640px, 40vw);
    font-size: max(15px, 1.4vh);
  }
  .toast {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px 14px;
    border-radius: var(--radius);
    background: var(--surface-3);
    border: 1px solid var(--line-strong);
    box-shadow: var(--shadow);
    font-size: 14.5px;
    font-weight: 600;
    animation: in 0.25s var(--ease) both;
  }
  .toast.error {
    border-color: color-mix(in oklab, var(--danger) 55%, transparent);
  }
  .toast.error :global(svg) {
    color: var(--danger);
    flex-shrink: 0;
  }
  @keyframes in {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
  }
</style>
