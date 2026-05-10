#!/usr/bin/env node

import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import net from "node:net";

const root = new URL("..", import.meta.url).pathname;
const requireFromWeb = createRequire(join(root, "apps/web/package.json"));
const { chromium } = requireFromWeb("playwright");
const children = [];

async function main() {
  const serverPort = await freePort();
  const webPort = await freePort();
  const tempDir = await mkdtemp(join(tmpdir(), "mulwiki-smoke-"));
  const serverUrl = `http://127.0.0.1:${serverPort}`;
  const webUrl = `http://127.0.0.1:${webPort}`;

  try {
    children.push(spawnProcess("go", ["run", "./cmd/server"], {
      cwd: join(root, "server"),
      env: {
        ...process.env,
        PORT: String(serverPort),
        DATABASE_URL: `file:${join(tempDir, "mulwiki.db")}`,
        DATA_DIR: join(tempDir, "data"),
        JWT_SECRET: "smoke-test-secret",
        ALLOWED_ORIGINS: webUrl,
      },
    }));
    await waitForURL(`${serverUrl}/health`, "server health");

    children.push(spawnProcess("pnpm", ["--dir", "apps/web", "exec", "next", "dev", "--hostname", "127.0.0.1", "--port", String(webPort)], {
      cwd: root,
      env: {
        ...process.env,
        MULWIKI_API_URL: serverUrl,
        NEXT_TELEMETRY_DISABLED: "1",
      },
    }));
    await waitForURL(webUrl, "web app");

    await runBrowserSmoke(webUrl);
  } finally {
    await Promise.all(children.map(stopProcess));
    await rm(tempDir, { recursive: true, force: true });
  }
}

async function runBrowserSmoke(webUrl) {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const suffix = Date.now().toString(36);
  const email = `smoke-${suffix}@example.com`;
  const slug = `smoke-${suffix}`;

  try {
    await page.goto(`${webUrl}/register`, { waitUntil: "networkidle" });
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password", { exact: true }).fill("12345678");
    await page.getByLabel("Confirm password").fill("12345678");
    await page.getByRole("button", { name: "Create account" }).click();
    await page.waitForURL("**/workspaces", { timeout: 15_000 });

    await page.getByRole("button", { name: "New" }).click();
    await page.getByLabel("Name").fill(`Smoke ${suffix}`);
    await page.getByLabel("Slug").fill(slug);
    await page.getByRole("button", { name: "Create" }).click();
    await page.waitForURL(`**/${slug}/wiki`, { timeout: 15_000 });
    await page.getByText("No wiki pages yet").waitFor({ timeout: 10_000 });
  } finally {
    await browser.close();
  }
}

function spawnProcess(command, args, options) {
  const child = spawn(command, args, {
    ...options,
    stdio: ["ignore", "pipe", "pipe"],
  });

  child.stdout.on("data", (chunk) => process.stdout.write(`[${command}] ${chunk}`));
  child.stderr.on("data", (chunk) => process.stderr.write(`[${command}] ${chunk}`));
  child.on("exit", (code, signal) => {
    if (code !== 0 && signal !== "SIGTERM") {
      process.stderr.write(`[${command}] exited with code ${code} signal ${signal}\n`);
    }
  });

  return child;
}

async function stopProcess(child) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  child.kill("SIGTERM");
  await new Promise((resolve) => {
    const timeout = setTimeout(() => {
      child.kill("SIGKILL");
      resolve();
    }, 5_000);
    child.once("exit", () => {
      clearTimeout(timeout);
      resolve();
    });
  });
}

async function waitForURL(url, label) {
  const deadline = Date.now() + 60_000;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
      lastError = new Error(`${label} returned ${res.status}`);
    } catch (err) {
      lastError = err;
    }
    await delay(500);
  }
  throw new Error(`Timed out waiting for ${label}: ${lastError?.message ?? "unknown error"}`);
}

async function freePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close(() => resolve(address.port));
    });
    server.on("error", reject);
  });
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

main()
  .then(() => process.exit(0))
  .catch((err) => {
    process.stderr.write(`${err.stack || err.message}\n`);
    process.exit(1);
  });
