import "@fontsource/barlow/400.css";
import "@fontsource/barlow/500.css";
import "@fontsource/barlow/600.css";
import "@fontsource/barlow/700.css";
import "@fontsource/barlow-condensed/600.css";
import "@fontsource/barlow-condensed/700.css";
import "./app.css";
import { mount } from "svelte";
import { api } from "./lib/api";
import App from "./App.svelte";
import Overlay from "./overlay/Overlay.svelte";

// Errors the interface runs into go to Seaglass's log (and so into
// diagnostics), not only to a console nobody sees.
window.addEventListener("error", (e) => api.reportUIError(`${e.message} at ${e.filename}:${e.lineno}:${e.colno}`));
window.addEventListener("unhandledrejection", (e) => {
  const r = e.reason;
  api.reportUIError(`unhandled rejection: ${r instanceof Error ? `${r.message} ${r.stack ?? ""}` : String(r)}`);
});

// One bundle, two windows: the main window and the in-game overlay.
const overlay = new URLSearchParams(location.search).get("view") === "overlay";
if (overlay) document.documentElement.classList.add("overlay-window");
mount(overlay ? Overlay : App, { target: document.getElementById("app")! });
