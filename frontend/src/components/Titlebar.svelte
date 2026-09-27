<script lang="ts">
  // The frameless window's title bar: drag anywhere, double-click to
  // maximise, and the three caption buttons.
  import { api } from "../lib/api";
  import Logo from "./Logo.svelte";
</script>

<!-- Double-clicking a title bar maximises, as everywhere in Windows; the caption buttons do the same by keyboard. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<header class="bar" ondblclick={(e) => !(e.target as HTMLElement).closest("button") && api.window.toggleMaximise()}>
  <div class="brand">
    <Logo size={20} />
    <span>Seaglass</span>
  </div>
  <div class="caption">
    <button type="button" aria-label="Minimize" onclick={() => api.window.minimise()}>
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M0 5h10" stroke="currentColor" /></svg>
    </button>
    <button type="button" aria-label="Maximize" onclick={() => api.window.toggleMaximise()}>
      <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true"><rect x=".5" y=".5" width="9" height="9" stroke="currentColor" /></svg>
    </button>
    <button type="button" class="close" aria-label="Close" onclick={() => api.window.close()}>
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M0 0l10 10M10 0L0 10" stroke="currentColor" /></svg>
    </button>
  </div>
</header>

<style>
  .bar {
    --wails-draggable: drag;
    height: 40px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    background: var(--surface);
    border-bottom: 1px solid var(--line);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    padding-left: 16px;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 17px;
    letter-spacing: 0.01em;
  }
  .caption {
    --wails-draggable: no-drag;
    margin-left: auto;
    display: flex;
    height: 100%;
  }
  .caption button {
    width: 46px;
    height: 100%;
    border: 0;
    background: transparent;
    color: var(--text-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .caption button:hover {
    background: var(--surface-3);
  }
  .caption .close:hover {
    background: #c42b1c;
    color: #fff;
  }
</style>
