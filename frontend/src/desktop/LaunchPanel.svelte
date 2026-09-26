<script lang="ts">
  // Desktop mode's view of a launch: the hooks running before the game,
  // questions they ask, and why a launch failed. Hidden while the game runs.
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { lib } from "../lib/store.svelte";
  import { crashText } from "../lib/types";

  const s = $derived(lib.session);
  let dismissed = $state(0);

  const show = $derived(
    !!s &&
      s.id !== dismissed &&
      s.route !== "external" && // noticed, not started here: nothing to show until it's played
      (s.phase === "preparing" || s.phase === "starting" || s.phase === "finishing" || s.phase === "failed" || (s.phase === "ended" && (!!s.note || !!s.crash))),
  );
  const heading = $derived.by(() => {
    if (!s) return "";
    switch (s.phase) {
      case "failed":
        return `Couldn't start ${s.title}`;
      case "ended":
        return `${s.title} closed`;
      case "finishing":
        return `Finishing ${s.title}`;
      default:
        return `Starting ${s.title}`;
    }
  });
  const waiting = $derived.by(() => {
    if (s?.phase !== "starting") return "";
    if (s.route === "steamInput") return "Waiting for the game to open through Steam…";
    if (s.route === "store") return "Waiting for the game to open through its store…";
    return "Waiting for the game to open…";
  });
</script>

{#if show && s}
  <section class="panel" aria-live="polite" aria-label={heading}>
    <header>
      <strong>{heading}</strong>
      {#if s.phase === "failed" || s.phase === "ended"}
        <button type="button" class="x" aria-label="Close" onclick={() => (dismissed = s.id)}><Icon name="close" size={18} /></button>
      {/if}
    </header>

    {#if (s.phase === "finishing" || s.phase === "ended" ? s.after : s.before).length}
      <ul class="steps">
        {#each s.phase === "finishing" || s.phase === "ended" ? s.after : s.before as st (st.id)}
          <li class={st.status}>
            <span class="mark">
              {#if st.status === "running"}<span class="spinner"></span>
              {:else if st.status === "done"}<Icon name="check" size={16} stroke={2.4} />
              {:else if st.status === "failed"}<Icon name="warn" size={16} />
              {:else}<span class="pip"></span>{/if}
            </span>
            <span class="label">{st.label}</span>
            {#if st.detail}<span class="detail">{st.detail}</span>{/if}
            {#if st.status === "running" && !s.question}
              <button type="button" class="link" onclick={() => api.launch.skip(st.id)}>Skip</button>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}

    {#if s.question}
      <p class="q">{s.question.text}</p>
      <div class="opts">
        {#each s.question.options as o, i (o.id)}
          <button type="button" class:primary={i === 0} onclick={() => api.launch.answer(s.question!.id, o.id)}>{o.label}</button>
        {/each}
      </div>
    {:else if s.phase === "failed"}
      <p class="err">{s.error}</p>
    {:else if s.phase === "ended"}
      <p class:err={!!s.crash}>{s.note || crashText(s)}</p>
    {:else if waiting}
      <p>{waiting}</p>
    {/if}

    {#if (s.phase === "preparing" || s.phase === "starting") && !s.question}
      <div class="foot"><button type="button" class="link" onclick={() => api.launch.cancel()}>Cancel</button></div>
    {/if}
  </section>
{/if}

<style>
  .panel {
    position: fixed;
    left: 50%;
    bottom: 24px;
    transform: translateX(-50%);
    z-index: 90;
    width: min(460px, calc(100vw - 32px));
    padding: 16px 18px;
    border-radius: var(--radius-l);
    border: 1px solid var(--line-strong);
    background: var(--surface);
    box-shadow: var(--shadow);
    display: flex;
    flex-direction: column;
    gap: 12px;
    animation: rise 0.25s var(--ease) both;
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translate(-50%, 12px);
    }
  }
  header {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 16px;
  }
  header strong {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .x {
    border: 0;
    background: none;
    color: var(--muted);
    padding: 4px;
    border-radius: var(--radius-s);
    display: flex;
  }
  .x:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .steps {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .steps li {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 14px;
  }
  .mark {
    width: 18px;
    display: flex;
    justify-content: center;
    color: var(--muted);
  }
  .done .mark {
    color: var(--accent-text);
  }
  .failed .mark {
    color: var(--warn);
  }
  .skipped .label {
    color: var(--muted);
    text-decoration: line-through;
  }
  .detail {
    flex: 1;
    min-width: 0;
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .pip {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
    opacity: 0.6;
  }
  .spinner {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    border: 2px solid var(--accent);
    border-right-color: transparent;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(1turn);
    }
  }
  p {
    margin: 0;
    font-size: 14px;
    line-height: 1.5;
    color: var(--text-2);
  }
  .q {
    color: var(--text);
  }
  .err {
    color: var(--danger);
    user-select: text;
  }
  .opts {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .opts button {
    height: 36px;
    padding: 0 14px;
    border-radius: var(--radius-s);
    border: 1px solid var(--line-strong);
    background: var(--surface-2);
    color: var(--text);
    font-weight: 600;
  }
  .opts button:hover {
    background: var(--surface-3);
  }
  .opts .primary {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-ink);
  }
  .opts .primary:hover {
    background: var(--accent);
    filter: brightness(1.08);
  }
  .foot {
    display: flex;
    justify-content: flex-end;
  }
  .link {
    border: 0;
    background: none;
    color: var(--accent-text);
    font-weight: 600;
    padding: 2px 4px;
    border-radius: var(--radius-s);
  }
  .link:hover {
    text-decoration: underline;
  }
  @media (prefers-reduced-motion: reduce) {
    .panel,
    .spinner {
      animation: none;
    }
  }
</style>
