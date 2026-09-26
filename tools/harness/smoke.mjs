// Quick check that the harness can start, see and drive the real app.
import path from "node:path";
import { OUT, backupAppData, restoreAppData, makeDevData, startApp, sleep } from "./lib.mjs";

backupAppData();
let app;
try {
  const data = makeDevData("smoke");
  app = await startApp({ data });
  console.log("started in", app.startMs, "ms:", app.page.url());
  await sleep(1500);
  console.log(await app.state());
  await app.shot(path.join(OUT, "smoke", "start.png"));
  await app.pad.press("down");
  await app.pad.press("right");
  await app.shot(path.join(OUT, "smoke", "after-pad.png"));
  console.log(await app.mem());
} finally {
  await app?.quit();
  restoreAppData();
}
