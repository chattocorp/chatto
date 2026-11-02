/** Check the real mise clean commands against disposable output and data fixtures. */
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const rootOutputs = [
  'dist/release.tar.gz',
  'cli/bin/.hidden-tool',
  'cli/chatto',
  'cli/cmd/embedded/NOTICE',
  'cli/internal/http_server/.client/_app/output.js',
  'cli/internal/http_server/.client/.hidden-output',
  'apps/frontend/build/200.html',
  'apps/frontend/.svelte-kit/types.d.ts',
  'apps/frontend/storybook-static/index.html',
  'apps/frontend/e2e/fixtures/bin/chatto',
  'apps/frontend/e2e/fixtures/chatto-e2e-server',
  'apps/docs-website/dist/index.html',
  'apps/docs-website/.astro/content.d.ts',
  'apps/desktop/dist/Chatto.app/binary',
  'apps/mobile/www/index.html',
  'apps/mobile/ios/App/App/public/index.html',
  'apps/mobile/ios/App/App/capacitor.config.json',
  'apps/mobile/ios/App/App/config.xml',
  'packages/chatto-client/dist/index.js',
  'packages/translatto/dist/index.js',
  'packages/runling/dist/index.js',
  'packages/runling/.svelte-kit/types.d.ts',
  '.context/build-frontend/completion'
];
const authlingOutputs = [
  'authling/bin/authling',
  'authling/bin/proto/protoc-gen-go',
  'authling/internal/web/assets/app.js',
  'authling/e2e/.generated/fixture.json'
];
const preserved = [
  'cli/internal/http_server/.client/.gitkeep',
  'cli/data/nats/store',
  'cli/backups/snapshot',
  'cli/chatto.toml',
  'cli/NOTICE',
  '.env',
  'mise.local.toml',
  'node_modules/dependency/index.js',
  '.turbo/cache/entry',
  '.context/notes.md',
  '.context/dev/authling/database',
  '.context/mobile-ios-build/Build/binary',
  'apps/desktop/native/macos-capture-probe/.build/cache',
  'apps/desktop/native/macos-capture-probe/captures/video.mov',
  'apps/mobile/ios/App/CapApp-SPM/.build/cache',
  'apps/frontend/test-results/report.json',
  'apps/frontend/e2e/fixtures/data-worker-0/database',
  'packages/api-types/src/chatto/api.ts',
  'packages/runling/.runling/runs/state.json',
  'authling/.authling/database',
  'authling/.env',
  'authling/node_modules/dependency/index.js',
  'authling/internal/pb/generated.pb.go',
  'authling/test-results/report.json'
];

for (const product of ['root', 'authling']) {
  test(`${product} clean removes outputs, preserves other files, and is repeatable`, (t) => {
    const productRoot = product === 'root' ? root : path.join(root, 'authling');
    const task = JSON.parse(
      execFileSync('mise', ['tasks', 'info', 'clean', '--json'], {
        cwd: productRoot,
        encoding: 'utf8'
      })
    );
    mkdirSync(path.join(root, '.context'), { recursive: true });
    const sandbox = mkdtempSync(path.join(root, '.context/clean-test-'));
    t.after(() => rmSync(sandbox, { recursive: true, force: true }));
    for (const file of [...rootOutputs, ...authlingOutputs, ...preserved]) {
      const destination = path.join(sandbox, file);
      mkdirSync(path.dirname(destination), { recursive: true });
      writeFileSync(destination, `fixture: ${file}\n`);
    }
    // Execute the task's current commands, but keep every removal inside the fixture.
    const cwd = product === 'root' ? sandbox : path.join(sandbox, 'authling');
    const removed = product === 'root' ? rootOutputs : authlingOutputs;
    const retained = [...preserved, ...(product === 'root' ? authlingOutputs : rootOutputs)];
    for (let attempt = 0; attempt < 2; attempt++) {
      execFileSync('sh', ['-eu', '-c', task.run.join('\n')], { cwd });
      for (const file of removed) assert.equal(existsSync(path.join(sandbox, file)), false, file);
      for (const file of retained) {
        assert.equal(readFileSync(path.join(sandbox, file), 'utf8'), `fixture: ${file}\n`, file);
      }
    }
  });
}
