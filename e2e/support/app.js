// Test support: an isolated home folder with a git fixture, and a running
// cleanmywt server pointed at it.
import { test as base } from "@playwright/test";
import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, mkdirSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");
export const binaryPath = join(repoRoot, "e2e", ".bin", process.platform === "win32" ? "cleanmywt.exe" : "cleanmywt");

const gitEnv = {
  ...process.env,
  GIT_AUTHOR_NAME: "t", GIT_AUTHOR_EMAIL: "t@example.com",
  GIT_COMMITTER_NAME: "t", GIT_COMMITTER_EMAIL: "t@example.com", GIT_CONFIG_NOSYSTEM: "1",
};

export function git(cwd, ...args) {
  return execFileSync("git", args, { cwd, env: gitEnv, encoding: "utf8" });
}

/** Same layout as internal/testrepo.NewFixture: merged (safe), dirty work (risky), gone (safe). */
export function makeFixture(root) {
  const code = join(root, "code");
  const repo = join(code, "app");
  const f = {
    repo,
    merged: join(repo, ".claude", "worktrees", "merged"),
    dirty: join(code, "dirty work"),
    gone: join(code, "gone"),
  };
  mkdirSync(repo, { recursive: true });
  git(repo, "init", "-q", "-b", "main");
  writeFileSync(join(repo, "README"), "hello\n");
  git(repo, "add", ".");
  git(repo, "commit", "-q", "-m", "init");
  git(repo, "worktree", "add", "-q", "-b", "feat/merged", f.merged);
  git(repo, "worktree", "add", "-q", "-b", "feat/dirty", f.dirty);
  writeFileSync(join(f.dirty, "notes.txt"), "work in progress\n");
  git(repo, "worktree", "add", "-q", "-b", "feat/gone", f.gone);
  rmSync(f.gone, { recursive: true, force: true });
  return f;
}

/** Starts cleanmywt with HOME and the config folder inside a fresh temp folder. */
async function startServer(home) {
  const config = join(home, "config");
  const child = spawn(binaryPath, ["--no-browser", "--port", "0"], {
    env: { ...process.env, HOME: home, USERPROFILE: home, APPDATA: config, XDG_CONFIG_HOME: config },
    stdio: ["ignore", "pipe", "pipe"],
  });
  let output = "";
  const url = await new Promise((resolveUrl, reject) => {
    const timer = setTimeout(() => reject(new Error("cleanmywt did not start:\n" + output)), 20_000);
    const onData = (chunk) => {
      output += chunk;
      const match = output.match(/http:\/\/127\.0\.0\.1:\d+\/#[0-9a-f]+/);
      if (match) { clearTimeout(timer); resolveUrl(match[0]); }
    };
    child.stdout.on("data", onData);
    child.stderr.on("data", onData);
    child.on("exit", (code) => { clearTimeout(timer); reject(new Error(`cleanmywt exited ${code}:\n${output}`)); });
  });
  return { url, child };
}

export const test = base.extend({
  // eslint-disable-next-line no-empty-pattern
  app: async ({}, use) => {
    const home = realpathSync(mkdtempSync(join(tmpdir(), "cleanmywt-e2e-")));
    const fixture = makeFixture(home);
    const { url, child } = await startServer(home);
    await use({ url, home, fixture });
    child.kill();
    await new Promise((r) => (child.exitCode !== null ? r() : child.once("exit", r)));
    rmSync(home, { recursive: true, force: true, maxRetries: 5 });
  },
});

export { expect } from "@playwright/test";
