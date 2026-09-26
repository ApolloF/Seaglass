// Drives every big picture screen in every layout, and desktop mode, with
// the controller only, then with the keyboard only, taking screenshots at
// every screen size and checking each screen's layout (audit.mjs).
//
//   node tour.mjs [--input=pad|keys|both] [--layouts=deck,console,orbit] [--extra=40]
//
// Output: %TEMP%\wl-harness\tour-<input>\<layout>-<screen>.sheet.png and
// report.json (layout issues per screen and size, and every press that
// didn't change what's focused).
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { auditPage } from "./audit.mjs";
import { SIZES } from "./sizes.mjs";
import { OUT, backupAppData, makeDevData, restoreAppData, sleep, startApp } from "./lib.mjs";

const argv = process.argv.slice(2);
const opt = (k, d) => argv.find((a) => a.startsWith(`--${k}=`))?.split("=")[1] ?? d;
const inputs = opt("input", "both") === "both" ? ["pad", "keys"] : [opt("input")];
const layouts = opt("layouts", "deck,console,orbit").split(",");
const sizes = opt("sizes", "") ? SIZES.filter(([w, h]) => opt("sizes").split(",").includes(`${w}x${h}`)) : SIZES;

// Controller buttons and the keys that do the same.
const KEYS = {
  up: "ArrowUp", down: "ArrowDown", left: "ArrowLeft", right: "ArrowRight",
  south: "Enter", east: "Escape", west: "x", north: "i", start: "m", back: "f",
  lb: "PageUp", rb: "PageDown", lt: "[", rt: "]", guide: "Home",
};

backupAppData();
try {
  for (const input of inputs) await tour(input);
} finally {
  restoreAppData();
}

async function tour(input) {
  const dir = path.join(OUT, `tour-${input}`);
  fs.rmSync(dir, { recursive: true, force: true });
  const data = makeDevData(`tour-${input}`, { extra: Number(opt("extra", 40)), settings: { bigPictureLayout: layouts[0] } });
  const app = await startApp({ data });
  try {
    await run(app, input, dir);
  } finally {
    await app.quit();
  }
}

async function run(app, input, dir) {
  const report = { input, snaps: {}, stuck: [] };
  const focus = () =>
    app.page.evaluate(() =>
      [...document.querySelectorAll(".on, :focus-visible, [aria-selected=true]")]
        .filter((e) => e.checkVisibility())
        .map((e) => (e.getAttribute("aria-label") || e.textContent || "").trim().slice(0, 30))
        .join(" | "),
    );
  let where = "start";
  async function press(button, { wait = 220, expectMove = false } = {}) {
    const before = expectMove ? await focus() : "";
    if (input === "pad") await app.pad.press(button, wait);
    else {
      await app.page.keyboard.press(KEYS[button]);
      await sleep(wait);
    }
    if (expectMove && (await focus()) === before) report.stuck.push({ where, button, focus: before });
  }
  // Closes Quick access if it's open (Esc on Home opens it with the keyboard).
  async function noQuickAccess() {
    if (await app.page.evaluate(() => !!document.querySelector("aside.qa"))) await press("start", { wait: 500 });
  }
  const move = (b, n = 1) => (async () => { for (let k = 0; k < n; k++) await press(b, { expectMove: true }); })();
  async function snap(name) {
    where = name;
    const files = [];
    const issues = {};
    for (const [w, h, scale] of sizes) {
      await app.viewport(w, h, scale);
      await sleep(400);
      const f = path.join(dir, name, `${w}x${h}.png`);
      await app.shot(f);
      files.push(f);
      const found = await app.page.evaluate(auditPage);
      if (found.length) issues[`${w}x${h}`] = found;
    }
    await app.viewport(1920, 1080);
    execFileSync("python", [path.join(import.meta.dirname, "sheet.py"), path.join(dir, `${name}.sheet.png`), ...files]);
    report.snaps[name] = issues;
    console.log(name, Object.values(issues).flat().length, "issues");
  }
  await app.viewport(1920, 1080);
  await sleep(800);

  // Desktop mode.
  await snap("desktop");
  if (input === "pad") {
    await move("down");
    await move("right", 2);
    await snap("desktop-grid");
    await press("rb"); // sidebar: next filter
    await press("rb");
    await snap("desktop-sidebar");
    await press("start", { wait: 700 }); // Settings
    await snap("desktop-settings");
    await press("east", { wait: 500 });
    await press("guide", { wait: 1500 }); // into big picture
  } else {
    // Tab through the window, then the arrow keys in the grid.
    for (let k = 0; k < 6; k++) await app.page.keyboard.press("Tab");
    await sleep(200);
    await snap("desktop-tab");
    await app.page.keyboard.press("F11");
    await sleep(1500);
  }

  for (const layout of layouts) {
    where = layout;
    await snap(`${layout}-home`);
    await move("right", 2);
    await move("down");
    await snap(`${layout}-home-moved`);
    await press("north", { wait: 700 });
    await snap(`${layout}-sheet`);
    await move("right", 2);
    await press("east", { wait: 600 });
    await noQuickAccess();
    await press("start", { wait: 600 });
    await snap(`${layout}-quick-access`);
    await press("east", { wait: 500 });
    await noQuickAccess();
    await press("rb", { wait: 600 });
    await snap(`${layout}-library`);
    await move("down", 2);
    await move("right", 3);
    await press("rt", { wait: 500 });
    await snap(`${layout}-library-filter`);
    await press("lt", { wait: 400 });
    await press("rb", { wait: 600 });
    // Search: type "st" (keys type straight into the field).
    if (input === "pad") {
      await press("down");
      await press("south");
      await press("up");
      await move("right", 3);
      await press("south");
    } else {
      await app.page.keyboard.type("st", { delay: 80 });
    }
    await sleep(500);
    await snap(`${layout}-search`);
    await press("lb", { wait: 500 });
    await press("lb", { wait: 600 });
    // Settings, through Quick access.
    await press("start", { wait: 600 });
    await move("down", 4);
    await press("south", { wait: 700 });
    await snap(`${layout}-settings`);
    await move("down", 18);
    await snap(`${layout}-settings-end`);
    for (let k = 0; k < 20; k++) await press("up", { wait: 90 });
    await press("right", { wait: 900 }); // the next layout
    await press("east", { wait: 1200 });
    await noQuickAccess();
  }
  // New on this PC, from the rail (Deck).
  fs.writeFileSync(path.join(dir, "report.json"), JSON.stringify(report, null, 1));
  console.log("stuck presses:", report.stuck.length);
}
