import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  globalSetup: "./global-setup.js",
  timeout: 60_000,
  expect: { timeout: 20_000 },
  // Each test starts its own server and fixture, so tests can run in parallel.
  fullyParallel: true,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    ...devices["Desktop Chrome"],
    // CI uses the Google Chrome that hosted runners already have (PW_CHANNEL=chrome),
    // which avoids downloading browsers and installing system packages.
    channel: process.env.PW_CHANNEL || undefined,
    viewport: { width: 1280, height: 800 },
    trace: "retain-on-failure",
  },
});
