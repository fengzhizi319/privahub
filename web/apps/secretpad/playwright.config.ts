import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { chromium, defineConfig, devices } from '@playwright/test';

/**
 * Playwright E2E configuration for SecretPad new frontend.
 * Runs against the local Vite dev server (default http://127.0.0.1:8000).
 * Make sure the backend is running on port 8080 and `pnpm dev` is started
 * before executing E2E tests, or use the `webServer` auto-launch block below.
 *
 * Environment:
 *   E2E_BASE_URL                    frontend URL (default http://127.0.0.1:8000)
 *   E2E_USER / E2E_PASSWORD         login (default admin / 12345678, dev profile)
 *   PLAYWRIGHT_CHROMIUM_EXECUTABLE  Chromium / headless-shell binary to use
 *   PLAYWRIGHT_BROWSERS_PATH        browser cache (default ~/.cache/ms-playwright)
 *
 * Browser: @playwright/test pins one Chromium revision. When that revision is
 * not installed (e.g. the cache holds a newer one and downloads are slow or
 * offline), the newest cached chromium_headless_shell-* / chromium-* build is
 * used instead of failing with "Executable doesn't exist".
 */

/** Executable locations inside a cached browser directory, per platform. */
const EXECUTABLES: Record<string, string[]> = {
  chromium_headless_shell: [
    'chrome-headless-shell-linux64/chrome-headless-shell',
    'chrome-headless-shell-linux-arm64/chrome-headless-shell',
    'chrome-headless-shell-mac-arm64/chrome-headless-shell',
    'chrome-headless-shell-mac-x64/chrome-headless-shell',
    'chrome-headless-shell-win64/chrome-headless-shell.exe',
  ],
  chromium: [
    'chrome-linux64/chrome',
    'chrome-linux/chrome',
    'chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing',
    'chrome-mac/Chromium.app/Contents/MacOS/Chromium',
    'chrome-win64/chrome.exe',
  ],
};

function browsersRoot(): string {
  const env = process.env.PLAYWRIGHT_BROWSERS_PATH;
  if (env && env !== '0') return env;
  if (process.platform === 'darwin') return path.join(os.homedir(), 'Library', 'Caches', 'ms-playwright');
  if (process.platform === 'win32') return path.join(os.homedir(), 'AppData', 'Local', 'ms-playwright');
  return path.join(os.homedir(), '.cache', 'ms-playwright');
}

/** Newest cached Chromium executable (headless shell preferred), if any. */
export function newestCachedChromium(root = browsersRoot()): string | undefined {
  let dirs: string[];
  try {
    dirs = fs.readdirSync(root);
  } catch {
    return undefined;
  }
  const found = dirs
    .map((d) => /^(chromium_headless_shell|chromium)-(\d+)$/.exec(d))
    .filter((m): m is RegExpExecArray => !!m)
    .sort((a, b) => Number(b[2]) - Number(a[2]) || (a[1] === 'chromium_headless_shell' ? -1 : 1));
  for (const m of found) {
    for (const rel of EXECUTABLES[m[1]]) {
      const exe = path.join(root, m[0], rel);
      if (fs.existsSync(exe)) return exe;
    }
  }
  return undefined;
}

/** undefined = let Playwright use its pinned (installed) revision. */
function resolveChromium(): string | undefined {
  const explicit = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE;
  if (explicit) return explicit;
  try {
    if (fs.existsSync(chromium.executablePath())) return undefined;
  } catch {
    // fall through to the cache scan
  }
  return newestCachedChromium();
}

const executablePath = resolveChromium();

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL || 'http://127.0.0.1:8000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        launchOptions: executablePath ? { executablePath } : {},
      },
    },
  ],
  // Auto-start the Vite dev server for E2E if E2E_START_SERVER is set.
  // In CI you may prefer to start the server externally.
  webServer: process.env.E2E_START_SERVER
    ? {
        command: 'pnpm dev',
        url: 'http://127.0.0.1:8000',
        reuseExistingServer: !process.env.CI,
        timeout: 120000,
      }
    : undefined,
});
