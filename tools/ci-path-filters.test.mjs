/** Exercise CI path selection with the workflow's globs and paths-filter matching semantics. */
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { load } from 'js-yaml';
import picomatch from 'picomatch';

const ci = load(readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8'));
const select = ci.jobs.changes.steps.find((step) => step.id === 'select');
const filters = load(select.with.filters);

/**
 * Match paths-filter's some-with-excludes policy with its Picomatch options:
 * any inclusion selects a file, but any exclusion rejects it, including dotfiles.
 * These workflow filters use plain strings, without change-status conditions.
 */
function matches(patterns, file) {
  const matchers = patterns.map((pattern) => {
    assert.equal(typeof pattern, 'string');
    return picomatch(pattern, { dot: true }, true);
  });
  return (
    matchers.some((matcher) => !matcher.state.negated && matcher(file)) &&
    !matchers.some((matcher) => matcher.state.negated && !matcher(file))
  );
}

/** Resolve the workflow's product outputs, including its common-change fallback. */
function products(files) {
  assert.equal(select.with['predicate-quantifier'], 'some-with-excludes');
  const matched = Object.fromEntries(
    Object.entries(filters).map(([name, patterns]) => [
      name,
      files.some((file) => matches(patterns, file))
    ])
  );
  return Object.entries(ci.jobs.changes.outputs)
    .filter(([, expression]) => {
      const terms = expression.slice(3, -2).trim().split(' || ');
      return terms
        .map((term) => {
          const match = /^steps\.select\.outputs\.(\w+) == 'true'$/.exec(term);
          assert.ok(match, `Unsupported selection expression: ${expression}`);
          return matched[match[1]];
        })
        .some(Boolean);
    })
    .map(([name]) => name)
    .sort();
}

test('release notes and changelogs alone do not select product jobs', () => {
  for (const directory of [
    '',
    'authling/',
    'packages/runling/',
    'packages/translatto/',
    'pkg/events/',
    'pkg/appconfig/',
    'pkg/datacrypto/',
    'pkg/jetstreamutil/',
    'pkg/natsruntime/'
  ]) {
    for (const file of [`${directory}.sweet/fix.md`, `${directory}CHANGELOG.md`]) {
      assert.deepEqual(products([file]), [], file);
    }
  }
});

test('release notes do not widen the selection for a frontend change', () => {
  const frontend = 'apps/frontend/src/lib/components/composer/MessageComposer.svelte';
  assert.deepEqual(products([frontend, '.sweet/fix.md']), products([frontend]));
  assert.ok(products([frontend]).includes('chatto'));
});

test('ordinary frontend changes select Chatto checks without native packaging', () => {
  for (const file of [
    'apps/frontend/src/lib/components/composer/MessageComposer.svelte',
    'apps/frontend/src/app.css',
    'apps/frontend/messages/de-DE/composer.json',
    'apps/frontend/e2e/threading.test.ts'
  ]) {
    assert.deepEqual(products([file]), ['chatto'], file);
  }
});

test('desktop shell, bridge, assets, and build inputs still select native packaging', () => {
  for (const file of [
    'apps/desktop/main.mjs',
    'apps/desktop/preload.cjs',
    'apps/desktop/scripts/windows-installer.mjs',
    'apps/frontend/src/lib/desktop/bridge.ts',
    'apps/frontend/package.json',
    'apps/frontend/vite.config.ts',
    'apps/frontend/svelte.config.js',
    'apps/frontend/scripts/check-csp.mjs',
    'apps/frontend/static/favicon.png',
    'packages/chatto-client/src/index.ts',
    'packages/api-types/src/index.ts',
    'packages/translatto/src/index.ts',
    'proto/chatto/api/v1/rooms.proto',
    'tools/test-desktop-windows-msi.ps1'
  ]) {
    assert.ok(products([file]).includes('desktop'), file);
  }
});

test('independent bot packages do not select desktop packaging', () => {
  for (const file of ['packages/runling/src/index.ts', 'packages/chattobot/src/index.ts']) {
    assert.equal(products([file]).includes('desktop'), false, file);
  }
});

test('documentation and agent instructions do not select product jobs', () => {
  assert.deepEqual(
    products(['CONTRIBUTING.md', 'docs/fdr/FDR-002.md', '.agents/skills/example/SKILL.md']),
    []
  );
});

test('shared build configuration still selects all products', () => {
  const all = Object.keys(ci.jobs.changes.outputs).sort();
  for (const file of [
    'pnpm-lock.yaml',
    'package.json',
    'mise.toml',
    'go.work.sum',
    '.github/workflows/ci.yml',
    '.sweet.config.yml',
    '.sweet.yml'
  ]) {
    assert.deepEqual(products([file]), all, file);
  }
});

test('release version files and package manifests still select their products', () => {
  for (const [file, product] of [
    ['cli/version.go', 'chatto'],
    ['apps/desktop/package.json', 'desktop'],
    ['authling/version.go', 'authling'],
    ['packages/runling/package.json', 'runling'],
    ['packages/translatto/package.json', 'translatto'],
    ['pkg/events/.sweet.yml', 'chatto']
  ]) {
    assert.ok(products([file]).includes(product), file);
  }
});

test('mixed changes select each affected product without the common fallback', () => {
  assert.deepEqual(
    products(['apps/frontend/src/app.css', 'packages/runling/src/index.ts', '.sweet/fix.md']),
    ['botto', 'chatto', 'chattobot', 'runling']
  );
});

test('Authling image filters skip notes on pushes and pull requests but keep builds', () => {
  const workflow = load(
    readFileSync(new URL('../.github/workflows/build-authling-image.yml', import.meta.url), 'utf8')
  );
  for (const event of ['push', 'pull_request']) {
    const patterns = workflow.on[event].paths;
    assert.equal(matches(patterns, 'authling/.sweet/fix.md'), false, event);
    assert.equal(matches(patterns, 'pkg/events/.sweet/fix.md'), false, event);
    assert.equal(matches(patterns, 'authling/CHANGELOG.md'), false, event);
    assert.equal(matches(patterns, 'authling/version.go'), true, event);
    assert.equal(matches(patterns, 'pkg/events/stream.go'), true, event);
    assert.equal(matches(patterns, 'authling/.sweet.yml'), true, event);
  }
});

test('release preparation still selects root and nested Sweet notes', () => {
  const workflow = load(
    readFileSync(new URL('../.github/workflows/bump-releases.yml', import.meta.url), 'utf8')
  );
  for (const file of ['.sweet/fix.md', 'authling/.sweet/fix.md', 'pkg/events/.sweet/fix.md']) {
    assert.equal(matches(workflow.on.push.paths, file), true, file);
  }
});
