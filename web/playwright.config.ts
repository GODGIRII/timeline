import { defineConfig } from '@playwright/test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
export default defineConfig({
  testDir: './tests', timeout: 90000, workers: 1,
  expect: { timeout: 15000 },
  use: { baseURL: 'http://localhost:4180', viewport: { width: 1440, height: 1000 }, trace: 'retain-on-failure' },
  webServer: {
    command: `go run ./cmd/server -dev -addr 127.0.0.1:4180 -origin http://localhost:4180 -web-dir web/dist -db /tmp/sequence-ui-${process.pid}.db`,
    cwd: root, url: 'http://localhost:4180/healthz', reuseExistingServer: false,
    env: { GOCACHE: '/tmp/timeline-go-cache' },
  },
});
