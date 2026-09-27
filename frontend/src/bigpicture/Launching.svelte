<script lang="ts">
  // The launch sequence in big picture: the hooks before the game (with
  // their questions), the wait for the game to open, the game while it
  // runs (when the interface stays open) and a summary when it ends.
  import GameArt from "../components/GameArt.svelte";
  import { api } from "../lib/api";
  import { clock, totalLine } from "../lib/format";
  import { feedback, useInput } from "../lib/input.svelte";
  import { crashText, sessionLoading, title, type Game, type Session } from "../lib/types";
  import Hints from "./Hints.svelte";

  let { session, game, onclose }: { session: Session; game: Game | null; onclose: () => void } = $props();

  let logoFailed = $state(false);
  let choice = $state(0);
  let quitArmed = $state(false);
  // One disarm timer: an older one would cut a fresh "press again" short.
  let quitTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => () => clearTimeout(quitTimer));

  const phase = $derived(session.phase);
  const loading = $derived(sessionLoading(session));
  const q = $derived(session.question);
  const steps = $derived(phase === "finishing" || phase === "ended" ? session.after : session.before);
  const running = $derived(steps.find((s) => s.status === "running"));

  $effect(() => {
    q?.id;
    choice = 0;
  });

  // Feedback as phases change; the summary and cancels close by themselves.
  let lastPhase = "";
  $effect(() => {
    const p = phase;
    if (p === lastPhase) return;
    lastPhase = p;
    if (p === "running") feedback.confirm();
    else if (p === "failed") feedback.error();
    else if (p === "cancelled") onclose();
    if (p === "ended") {
      const t = setTimeout(onclose, session.note ? 9000 : 6000);
      return () => clearTimeout(t);
    }
  });

  const label = $derived.by(() => {
    switch (phase) {
      case "preparing":
        return "Getting ready";
      case "starting":
        return "Starting";
      case "running":
        return loading ? "Loading" : "Playing";
      case "finishing":
        return "Finishing";
      case "failed":
        return "Couldn't start";
      default:
        return session.crash ? "Closed unexpectedly" : "Played";
    }
  });

  async function quit() {
    if (!quitArmed) {
      quitArmed = true;
      feedback.error();
      clearTimeout(quitTimer);
      quitTimer = setTimeout(() => (quitArmed = false), 4000);
      return;
    }
    quitArmed = false;
    clearTimeout(quitTimer);
    try {
      await api.launch.quitGame();
    } catch {
      feedback.error();
    }
  }

  $effect(() =>
    useInput((i) => {
      if (q) {
        if (i === "left" || i === "up") choice = Math.max(0, choice - 1);
        else if (i === "right" || i === "down") choice = Math.min(q.options.length - 1, choice + 1);
        else if (i === "confirm") api.launch.answer(q.id, q.options[choice].id);
        else if (i === "back") api.launch.answer(q.id, q.options.find((o) => o.id === "cancel")?.id ?? q.options[q.options.length - 1].id);
        else return;
        feedback.move();
        return;
      }
      switch (phase) {
        case "preparing":
        case "starting":
          if (i === "back") api.launch.cancel();
          else if (i === "action" && running) api.launch.skip(running.id);
          break;
        case "running":
          if (i === "back") onclose();
          else if (i === "action") quit();
          break;
        case "finishing":
          if (i === "back") onclose();
          else if (i === "action" && running) api.launch.skip(running.id);
          break;
        default:
          if (i === "back" || i === "confirm") onclose();
      }
    }),
  );

  const hints = $derived.by(() => {
    if (q) return [{ button: "confirm" as const, label: "Choose" }];
    switch (phase) {
      case "preparing":
      case "starting":
        return [...(running ? [{ button: "action" as const, label: "Skip step" }] : []), { button: "back" as const, label: "Cancel" }];
      case "running":
        return [
          { button: "action" as const, label: quitArmed ? "Press again to quit" : "Quit game" },
          { button: "back" as const, label: "Library" },
        ];
      case "finishing":
        return [...(running ? [{ button: "action" as const, label: "Skip step" }] : []), { button: "back" as const, label: "Library" }];
      default:
        return [{ button: "confirm" as const, label: "Back to library" }];
    }
  });
</script>

<div class="launch">
  {#if game}<div class="art"><GameArt {game} kind="backdrop" /></div>{/if}
  <div class="shade"></div>
  <div class="content">
    <span class="label">{label}</span>
    {#if game?.meta?.logo && !logoFailed}
      <img class="logo" src={game.meta.logo} alt={session.title} onerror={() => (logoFailed = true)} />
    {:else}
      <h1>{game ? title(game) : session.title}</h1>
    {/if}

    {#if steps.length && (phase === "preparing" || phase === "starting" || phase === "finishing")}
      <ul class="steps">
        {#each steps as st (st.id)}
          <li class={st.status}>
            <span class="mark">{st.status === "done" ? "✓" : st.status === "failed" ? "!" : st.status === "skipped" ? "–" : ""}</span>
            <span>{st.label}</span>
            {#if st.detail}<span class="detail">{st.detail}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}

    {#if q}
      <p>{q.text}</p>
      <div class="opts">
        {#each q.options as o, i (o.id)}
          <button type="button" class:on={i === choice} onclick={() => api.launch.answer(q.id, o.id)} onmouseenter={() => (choice = i)}>{o.label}</button>
        {/each}
      </div>
    {:else if phase === "preparing" || phase === "starting" || phase === "finishing"}
      <div class="bar"><span></span></div>
      {#if phase === "starting" && session.route === "steamInput"}
        <p>Starting through Steam Input, so the controller works as an Xbox controller.</p>
      {:else if phase === "starting" && session.route === "store"}
        <p>Handed to its store. The game can take a moment to appear.</p>
      {/if}
    {:else if loading}
      <div class="bar"><span></span></div>
      <p>The game is loading. It comes to the front when it's ready.</p>
    {:else if phase === "running"}
      <p class="time">{clock(session.seconds)}</p>
      <p>{session.route === "external" ? "Started outside Seaglass; its playtime counts here too." : "The game is running."} Press the PS button in the game to open the overlay.</p>
    {:else if phase === "failed"}
      <p class="err">{session.error}</p>
    {:else if session.note}
      <p>{session.note}</p>
    {:else}
      <p class="time">{clock(session.seconds)}</p>
      {#if session.crash}<p class="err">{crashText(session)}</p>{/if}
      <p>{game ? totalLine(Math.max(game.playtime ?? 0, game.storePlaytime ?? 0)) : ""} {session.crash ? "" : "Welcome back."}</p>
    {/if}
  </div>
  <div class="hints"><Hints {hints} /></div>
</div>

<style>
  .launch {
    position: absolute;
    inset: 0;
    z-index: 40;
    background: #05080b;
    animation: in 0.35s ease both;
  }
  @keyframes in {
    from {
      opacity: 0;
    }
  }
  .art {
    position: absolute;
    inset: 0;
    opacity: 0.55;
    animation: kb 12s ease-out both;
  }
  @keyframes kb {
    from {
      transform: scale(1.06);
    }
  }
  .shade {
    position: absolute;
    inset: 0;
    background: linear-gradient(0deg, rgba(5, 8, 11, 0.97) 0%, rgba(5, 8, 11, 0.55) 55%, rgba(5, 8, 11, 0.25) 100%);
  }
  .content {
    position: absolute;
    left: 140px;
    bottom: 170px;
    width: 1100px;
    display: flex;
    flex-direction: column;
    gap: 26px;
    color: #f3f5f7;
  }
  .label {
    font-size: 20px;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: oklch(0.8 0.12 205);
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 110px;
    line-height: 0.95;
  }
  .logo {
    max-width: 760px;
    max-height: 260px;
    object-fit: contain;
    object-position: left bottom;
    filter: drop-shadow(0 6px 30px rgba(0, 0, 0, 0.6));
  }
  .steps {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 12px;
    font-size: 24px;
  }
  .steps li {
    display: flex;
    align-items: center;
    gap: 16px;
  }
  .mark {
    width: 30px;
    height: 30px;
    border-radius: 50%;
    border: 2px solid rgba(255, 255, 255, 0.25);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 18px;
    font-weight: 700;
  }
  .running .mark {
    border-color: oklch(0.8 0.12 205);
    border-right-color: transparent;
    animation: spin 0.8s linear infinite;
  }
  .done .mark {
    border-color: oklch(0.8 0.12 205);
    color: oklch(0.8 0.12 205);
  }
  .failed .mark {
    border-color: #ffd28a;
    color: #ffd28a;
  }
  .skipped {
    color: #8795a3;
  }
  .detail {
    color: #9fb0bf;
  }
  @keyframes spin {
    to {
      transform: rotate(1turn);
    }
  }
  .opts {
    display: flex;
    gap: 16px;
  }
  .opts button {
    height: 64px;
    padding: 0 30px;
    border-radius: 14px;
    border: 2px solid rgba(255, 255, 255, 0.16);
    background: rgba(255, 255, 255, 0.06);
    color: #e8edf2;
    font-size: 22px;
    font-weight: 600;
    transition: transform 0.15s ease;
  }
  .opts button.on {
    border-color: oklch(0.8 0.12 205);
    background: oklch(0.8 0.12 205);
    color: #071016;
    transform: scale(1.04);
  }
  .bar {
    width: 520px;
    height: 8px;
    border-radius: 4px;
    background: rgba(255, 255, 255, 0.12);
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    width: 40%;
    border-radius: 4px;
    background: oklch(0.8 0.12 205);
    animation: slide 1.1s ease-in-out infinite;
  }
  @keyframes slide {
    from {
      transform: translateX(-110%);
    }
    to {
      transform: translateX(260%);
    }
  }
  p {
    margin: 0;
    font-size: 24px;
    line-height: 1.5;
    color: #c9d3dc;
  }
  .time {
    font-family: var(--font-display);
    font-size: 72px;
    line-height: 1;
    color: #f3f5f7;
    font-variant-numeric: tabular-nums;
  }
  .err {
    color: #ffb3b3;
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 48px;
  }
  @media (prefers-reduced-motion: reduce) {
    .art,
    .bar span,
    .running .mark {
      animation: none;
    }
  }
</style>
