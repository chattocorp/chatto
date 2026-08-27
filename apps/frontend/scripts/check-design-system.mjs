import { readdir, readFile } from 'node:fs/promises';
import { relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontendRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const sourceRoot = resolve(frontendRoot, 'src');
const appCssPath = resolve(sourceRoot, 'app.css');

const styleBlockAllowlist = new Set([
  'src/lib/components/MobileSidebarChrome.svelte',
  'src/lib/components/chat/FullscreenVideoOverlay.svelte',
  'src/lib/components/chat/VideoPlayer.svelte',
  'src/lib/components/composer/TipTapEditor.svelte',
  'src/lib/components/voice/VoiceCallPanel.svelte',
  'src/lib/ui/AppHeader.svelte',
  'src/lib/ui/BottomSheet.svelte',
  'src/lib/ui/Dialog.svelte',
  'src/lib/ui/toast/ToastContainer.svelte'
]);

const checks = [
  {
    description: 'bare transition utilities; name the transitioned properties',
    pattern: /(?:^|\s)transition(?:-all)?(?:\s|$)/g
  },
  {
    description: 'stateful important overrides; add a semantic component variant',
    pattern: /(?:hover|focus|focus-visible|active):!/g
  },
  {
    description: 'global font smoothing; use browser/platform rendering',
    pattern: /(?:\bantialiased\b|-webkit-font-smoothing|-moz-osx-font-smoothing)/g
  },
  {
    description: 'raw palette colors; use Chatto semantic color tokens',
    pattern:
      /(?:text|bg|border|ring|outline|from|to)-(?:gray|slate|zinc|neutral|stone|red|green|blue|sky|amber|yellow|indigo)-\d+/g
  },
  {
    description: 'retired color token; use action or neutral-action',
    pattern: /(?:text|bg|border|ring|outline|from|to)-(?:accent|primary)(?:\b|\/)/g
  },
  {
    description: 'retired numbered surface token; use a semantic surface token',
    pattern: /surface-(?:100|200|300|highlighted)(?:\b|\/)/g
  },
  {
    description: 'hard-coded white on a semantic fill; use the matching on-* foreground token',
    pattern:
      /(?:(?:bg|from|to)-(?:action|success|warning|danger)[^'"\n]*text-white|text-white[^'"\n]*(?:bg|from|to)-(?:action|success|warning|danger))/g
  }
];

async function svelteFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map(async (entry) => {
      const path = resolve(directory, entry.name);
      if (entry.isDirectory()) return svelteFiles(path);
      if (!entry.name.endsWith('.svelte') || entry.name.endsWith('.stories.svelte')) return [];
      return [path];
    })
  );
  return files.flat();
}

async function sourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map(async (entry) => {
      const path = resolve(directory, entry.name);
      if (entry.isDirectory()) return sourceFiles(path);
      if (!/\.(?:svelte|[cm]?[jt]s)$/.test(entry.name)) return [];
      return [path];
    })
  );
  return files.flat();
}

const failures = [];
const appCss = await readFile(appCssPath, 'utf8');

if (!/@import\s+['"]tailwindcss['"]\s+source\(['"]\.\/['"]\)/.test(appCss)) {
  failures.push(
    "src/app.css: scope Tailwind source detection to source('./') so production ignores sources outside src"
  );
}

if (/\bprefixes\s*:/.test(appCss)) {
  failures.push(
    'src/app.css: Iconify prefixes eagerly expand complete icon collections; use dynamic utilities such as icon-[uil--check]'
  );
}

for (const file of await sourceFiles(sourceRoot)) {
  const path = relative(frontendRoot, file);
  const source = await readFile(file, 'utf8');
  const legacyIcon = /(?<!icon-\[)\b(?:uil|mdi|logos)--[a-z0-9-]+/g;

  for (const match of source.matchAll(legacyIcon)) {
    const line = source.slice(0, match.index).split('\n').length;
    failures.push(
      `${path}:${line}: legacy Iconify class eagerly requires complete collections; use icon-[${match[0]}]`
    );
  }
}

for (const file of await svelteFiles(sourceRoot)) {
  const path = relative(frontendRoot, file);
  const source = await readFile(file, 'utf8');
  const utilitySource = source
    .replace(/<style[\s\S]*?<\/style>/g, '')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\/.*$/gm, '');

  if (source.includes('<style') && !styleBlockAllowlist.has(path)) {
    failures.push(
      `${path}: unreviewed <style> block; use Tailwind or update the documented allowlist`
    );
  }

  for (const { description, pattern } of checks) {
    pattern.lastIndex = 0;
    for (const match of utilitySource.matchAll(pattern)) {
      const line = utilitySource.slice(0, match.index).split('\n').length;
      failures.push(`${path}:${line}: ${description} (${match[0].trim()})`);
    }
  }

  for (const contextMenu of utilitySource.matchAll(/<ContextMenu\b[\s\S]*?<\/ContextMenu>/g)) {
    const actionSeparator = contextMenu[0].match(
      /(?:class\s*=\s*["'][^"']*\bborder-t\b[^"']*["']|role\s*=\s*["']separator["'])[\s\S]*?<(?:a|button)\b/
    );
    if (!actionSeparator) continue;

    const line = utilitySource
      .slice(0, (contextMenu.index ?? 0) + (actionSeparator.index ?? 0))
      .split('\n').length;
    failures.push(
      `${path}:${line}: context-menu action groups must use sibling menu-section surfaces, not inline separators`
    );
  }
}

if (failures.length > 0) {
  console.error('Design-system guardrails failed:\n');
  for (const failure of failures) console.error(`- ${failure}`);
  process.exitCode = 1;
} else {
  console.log('Design-system guardrails passed.');
}
