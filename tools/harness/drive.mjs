// Starts the real app and keeps it running behind a small local HTTP
// server, so screens can be explored one step at a time:
//
//   node drive.mjs [dataName] [--extra=N] [--layout=deck] [--real]
//   curl "127.0.0.1:9444/pad?b=down"        press a controller button
//   curl "127.0.0.1:9444/key?k=ArrowDown"   press a key
//   curl "127.0.0.1:9444/snap?name=home"    screenshots at every size + audit
//   curl "127.0.0.1:9444/shot?name=x"       one screenshot at the current size
//   curl -d "document.title" 127.0.0.1:9444/eval
//   curl "127.0.0.1:9444/quit"              close, restore %APPDATA%, exit
import { execFileSync } from "node:child_process";
import http from "node:http";
import path from "node:path";
import { auditPage } from "./audit.mjs";
import { OUT, backupAppData, makeDevData, restoreAppData, sleep, startApp } from "./lib.mjs";

export { SIZES } from "./sizes.mjs";
import { SIZES } from "./sizes.mjs";

const argv = process.argv.slice(2);
const opt = (k, d) => argv.find((a) => a.startsWith(`--${k}=`))?.split("=")[1] ?? d;
const run = argv.find((a) => !a.startsWith("--")) ?? "drive";
let base = opt("base", "1920x1080").split("x").map(Number);

backupAppData();
const settings = { bigPictureLayout: opt("layout", "deck") };
if (argv.includes("--bp")) settings.startInBigPicture = true;
const data = argv.includes("--real") ? undefined : makeDevData(run, { extra: Number(opt("extra", 0)), settings });
let app;
try {
  app = await startApp({ data, pad: opt("pad", "ps") });
} catch (e) {
  restoreAppData();
  throw e;
}
await app.viewport(...base);
console.log(`ready in ${app.startMs} ms`);

async function snap(name, sizes = SIZES) {
  const dir = path.join(OUT, run, name);
  const files = [];
  const issues = {};
  for (const [w, h, scale] of sizes) {
    await app.viewport(w, h, scale ?? 1);
    await sleep(350);
    const f = path.join(dir, `${w}x${h}.png`);
    await app.shot(f);
    files.push(f);
    const found = await app.page.evaluate(auditPage);
    if (found.length) issues[`${w}x${h}`] = found;
  }
  await app.viewport(...base);
  const sheet = path.join(OUT, run, `${name}.sheet.png`);
  execFileSync("python", [path.join(import.meta.dirname, "sheet.py"), sheet, ...files]);
  return { sheet, issues };
}

const server = http.createServer(async (req, res) => {
  const u = new URL(req.url, "http://x");
  const q = Object.fromEntries(u.searchParams);
  let body = "";
  for await (const c of req) body += c;
  const reply = (v) => res.end((typeof v === "string" ? v : JSON.stringify(v, null, 1)) + "\n");
  try {
    switch (u.pathname) {
      case "/pad":
        for (const b of q.b.split(",")) await app.pad.press(b, Number(q.wait ?? 200));
        return reply("ok");
      case "/raw":
        return reply(await app.pad.send(q.cmd));
      case "/key":
        for (const k of q.k.split(",")) {
          await app.page.keyboard.press(k);
          await sleep(Number(q.wait ?? 200));
        }
        return reply("ok");
      case "/type":
        await app.page.keyboard.type(q.t, { delay: 40 });
        return reply("ok");
      case "/snap":
        return reply(await snap(q.name, q.sizes ? q.sizes.split(",").map((s) => SIZES.find(([w, h]) => `${w}x${h}` === s) ?? s.split("x").map(Number)) : SIZES));
      case "/shot": {
        const f = path.join(OUT, run, `${q.name}.png`);
        await app.shot(f);
        return reply(f);
      }
      case "/size":
        base = q.s.split("x").map(Number);
        await app.viewport(...base);
        return reply("ok");
      case "/audit":
        return reply(await app.page.evaluate(auditPage));
      case "/eval":
        return reply(await app.page.evaluate(body));
      case "/state":
        return reply(await app.state());
      case "/mem":
        return reply(await app.mem());
      case "/main":
        await app.mainPage();
        await app.viewport(...base);
        return reply(app.page.url());
      case "/pages":
        return reply(app.pages().map((p) => p.url()));
      case "/quit":
        await app.quit();
        restoreAppData();
        reply("bye");
        setTimeout(() => process.exit(0), 100);
    }
    res.statusCode = 404;
    reply("unknown");
  } catch (e) {
    res.statusCode = 500;
    reply(String(e?.stack ?? e));
  }
});
server.listen(9444, "127.0.0.1");
for (const sig of ["SIGINT", "SIGTERM"]) {
  process.on(sig, async () => {
    await app.quit();
    restoreAppData();
    process.exit(0);
  });
}
