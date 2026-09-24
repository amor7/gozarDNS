// Regenerates docs/screenshots from a running dev stack:
//
//   go run ./tools/devstack -fresh          (terminal 1)
//   npm --prefix web run dev:admin          (terminal 2)
//   npm --prefix web run dev:portal         (terminal 3)
//   node tools/screenshots/shoot.mjs        (CHROME=/path/to/chrome if needed)
//
// Drives headless Chrome over the DevTools protocol; no npm packages needed.
import { spawn } from "node:child_process";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, "..", "..", "docs", "screenshots");
mkdirSync(out, { recursive: true });

const chrome = process.env.CHROME || (process.platform === "win32"
  ? "C:/Program Files/Google/Chrome/Application/chrome.exe"
  : "google-chrome");
const port = 9333;
const ADMIN = "http://localhost:5173";
const PORTAL = "http://localhost:5174";

const shots = [
  { file: "admin-dashboard.png", origin: ADMIN, path: "/", lang: "fa", theme: "dark", w: 1440, h: 1500, login: ["admin", "admin12345"] },
  { file: "admin-users.png", origin: ADMIN, path: "/users", lang: "fa", theme: "light", w: 1440, h: 980, login: ["admin", "admin12345"] },
  { file: "admin-plan-editor.png", origin: ADMIN, path: "/plans", lang: "fa", theme: "dark", w: 1440, h: 1000, login: ["admin", "admin12345"], click: "ویرایش" },
  { file: "admin-receipts.png", origin: ADMIN, path: "/payments", lang: "en", theme: "light", w: 1440, h: 900, login: ["admin", "admin12345"] },
  { file: "admin-login.png", origin: ADMIN, path: "/", lang: "fa", theme: "light", w: 1440, h: 900 },
  { file: "portal-home.png", origin: PORTAL, path: "/", lang: "fa", theme: "dark", w: 390, h: 844, full: true, login: ["sara", "sara1234"], mobile: true },
  { file: "portal-home-light.png", origin: PORTAL, path: "/", lang: "fa", theme: "light", w: 390, h: 844, full: true, login: ["sara", "sara1234"], mobile: true },
  { file: "portal-setup.png", origin: PORTAL, path: "/setup", lang: "fa", theme: "dark", w: 390, h: 844, full: true, login: ["sara", "sara1234"], mobile: true },
  { file: "portal-plans.png", origin: PORTAL, path: "/plans", lang: "fa", theme: "light", w: 1280, h: 900, login: ["sara", "sara1234"] },
  { file: "portal-welcome.png", origin: PORTAL, path: "/", lang: "en", theme: "dark", w: 1280, h: 900 },
];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const proc = spawn(chrome, [
  "--headless=new", `--remote-debugging-port=${port}`, `--user-data-dir=${mkdtempSync(join(tmpdir(), "gz-shots-"))}`,
  "--no-first-run", "--no-default-browser-check", "--hide-scrollbars", "--force-device-scale-factor=2", "about:blank",
], { stdio: "ignore" });

let ws;
let seq = 0;
const pending = new Map();
function send(method, params = {}) {
  const id = ++seq;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
}

async function connect() {
  for (let i = 0; i < 50; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
      const page = list.find((t) => t.type === "page");
      if (page) {
        ws = new WebSocket(page.webSocketDebuggerUrl);
        await new Promise((r, j) => { ws.onopen = r; ws.onerror = j; });
        ws.onmessage = (ev) => {
          const m = JSON.parse(ev.data);
          if (m.id && pending.has(m.id)) {
            const p = pending.get(m.id);
            pending.delete(m.id);
            m.error ? p.reject(new Error(m.error.message)) : p.resolve(m.result);
          }
        };
        return;
      }
    } catch { /* not up yet */ }
    await sleep(200);
  }
  throw new Error("could not reach headless Chrome");
}

const evaluate = (expression) => send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });

async function shoot(s) {
  await send("Emulation.setDeviceMetricsOverride", { width: s.w, height: s.h, deviceScaleFactor: 2, mobile: !!s.mobile });
  await send("Page.navigate", { url: s.origin + "/" });
  await sleep(1500);
  await evaluate(`localStorage.setItem("gz-lang", ${JSON.stringify(s.lang)}); localStorage.setItem("gz-theme", ${JSON.stringify(s.theme)});`);
  await evaluate(`fetch("/api/logout", { method: "POST", headers: { "X-Requested-With": "gozar" } })`);
  if (s.login) {
    const [u, p] = s.login;
    await evaluate(`fetch("/api/login", { method: "POST", headers: { "X-Requested-With": "gozar", "Content-Type": "application/json" }, body: JSON.stringify({ username: "${u}", password: "${p}" }) }).then(r => r.status)`);
  }
  await send("Page.navigate", { url: s.origin + s.path });
  await sleep(2500);
  if (s.click) {
    await evaluate(`[...document.querySelectorAll("button")].find(b => b.textContent.includes(${JSON.stringify(s.click)}))?.click()`);
    await sleep(1500);
  }
  let clip;
  if (s.full) {
    const { result } = await evaluate("document.documentElement.scrollHeight");
    const h = Math.min(result.value, 4000);
    await send("Emulation.setDeviceMetricsOverride", { width: s.w, height: h, deviceScaleFactor: 2, mobile: !!s.mobile });
    await sleep(800);
    clip = { x: 0, y: 0, width: s.w, height: h, scale: 1 };
  }
  const { data } = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: !!s.full, ...(clip ? { clip } : {}) });
  writeFileSync(join(out, s.file), Buffer.from(data, "base64"));
  console.log("wrote", s.file);
}

try {
  await connect();
  await send("Page.enable");
  await send("Runtime.enable");
  for (const s of shots) await shoot(s);
} finally {
  ws?.close();
  proc.kill();
}
