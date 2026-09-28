import { readdir, readFile } from 'node:fs/promises';
import { relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontendRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const sourceRoot = resolve(frontendRoot, 'src');
const appCssPath = resolve(sourceRoot, 'app.css');

// Each allowlisted <style> block must open with a comment that explains why
// Tailwind or a semantic utility from src/app.css is insufficient.
const styleBlockAllowlist = new Set([
  'src/lib/components/chat/FullscreenVideoOverlay.svelte',
  'src/lib/components/chat/VideoPlayer.svelte',
  'src/lib/components/composer/TipTapEditor.svelte',
  'src/lib/ui/AppHeader.svelte',
  'src/lib/ui/ModalSurface.svelte',
  'src/lib/ui/Dialog.svelte',
  'src/lib/ui/toast/ToastContainer.svelte'
]);

// These components own specialized native-dialog behavior. Standard task
// dialogs must use Dialog, FormDialog, or ConfirmDialog instead.
const nativeDialogAllowlist = new Set([
  'src/lib/components/QuickSwitcher.svelte',
  'src/lib/ui/ModalSurface.svelte',
  'src/routes/chat/ModalContainerConfirmDialogMock.svelte',
  'src/routes/chat/ModalContainerDialogMock.svelte',
  'src/routes/chat/[serverId]/manage/rooms/[roomId]/RoomMembersConfirmDialogMock.svelte'
]);

// Known drift that a focused issue tracks. Remove each entry when its issue
// lands; do not add new files here to silence a check.
const knownDrift = {
  // Join/Leave status buttons and scope labels: #2667
  roomDirectoryActions: new Set(['src/lib/RoomDirectory.svelte']),
  // Join/Leave status buttons: #2667
  buttonRecipes: new Set(['src/lib/RoomDirectory.svelte'])
};

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
    // The wordmark easter egg is a canvas game with its own fixed art palette.
    allow: new Set(['src/lib/components/SimulatedChattoWordmark.svelte']),
    pattern:
      /(?:text|bg|border|ring|outline|from|via|to|fill|stroke|shadow|divide|placeholder|decoration|caret|accent)-(?:gray|slate|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d+/g
  },
  {
    description:
      'physical corner utility does not exist; use rounded-ss, rounded-se, rounded-es, or rounded-ee',
    pattern: /(?<![\w-])rounded-(?:ts|te|bs|be)(?:-[\w.[\]]+)?(?![\w-])/g
  },
  {
    description: 'important cursor override; use a disabled: or state variant',
    allow: knownDrift.roomDirectoryActions,
    pattern: /(?:^|[\s"'`])!cursor-[a-z-]+/g
  },
  {
    description: 'hand-built inline link; use the link utility',
    pattern:
      /\btext-action\b[^"'`\n]*\bhover:underline\b|\bhover:underline\b[^"'`\n]*\btext-action\b/g
  },
  {
    description:
      'Tailwind 4 spacing and divide utilities are already logical; remove the RTL reverse',
    pattern: /\brtl:(?:space|divide)-x-reverse\b/g
  },
  {
    description: 'mirror directional icons with rtl:-scale-x-100',
    pattern: /\brtl:(?:scale-x-\[-1\]|rotate-180)(?![\w-])/g
  },
  {
    // Door arrows and redo point along the reading direction. MenuItem uses
    // its mirrorIconInRtl prop instead of the utility.
    description:
      'mirror door-arrow and redo icons in right-to-left layouts: add rtl:-scale-x-100, or mirrorIconInRtl on MenuItem',
    pattern:
      /icon-\[uil--(?:sign-in-alt|sign-out-alt|signin|signout|redo|undo)\](?![^'"`\n]*rtl:-scale-x-100)(?![^>]*mirrorIconInRtl)/g
  },
  {
    description:
      'wheelchair or accessibility icons do not describe content; use icon-[uil--file-edit-alt]',
    pattern: /icon-\[[a-z0-9]+--[\w-]*(?:wheelchair|accessib|universal-access)[\w-]*\]/g
  },
  {
    description: 'hand-built button recipe; use Button from $lib/ui/form',
    featureOnly: true,
    allow: knownDrift.buttonRecipes,
    // Attachment actions pair their square geometry with a btn tone by design.
    skipLine: /attachment-action-button/,
    pattern:
      /(?<![\w-])btn(?:-(?:action|neutral|secondary|ghost|warning|danger|danger-secondary|danger-ghost|icon|sm|lg|xs))?(?![\w-])/g
  },
  {
    description: 'raw select or textarea; use Select or TextArea from $lib/ui/form',
    featureOnly: true,
    allow: new Set([
      // The code-block language picker is an invisible native select over
      // editor chrome; the Select field treatment does not apply.
      'src/lib/components/composer/TipTapEditor.svelte'
    ]),
    pattern: /<(?:select|textarea)\b/g
  },
  {
    description:
      'clickable table row or cell; put a data-table-row-link on a real link in the row instead',
    pattern:
      /<(?:tr|td)\b[^>]*\bon(?:click|auxclick|dblclick|keydown|pointerdown|pointerup|mousedown|mouseup)\b/g
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
const unusedStyleAllowlistEntries = new Set(styleBlockAllowlist);

// Tailwind silently ignores color utilities whose token does not exist, so a
// typo or a token from another design system renders nothing. Accept only the
// --color-* tokens that src/app.css defines, plus CSS keywords.
const colorTokens = new Set([
  ...[...appCss.matchAll(/--color-([a-z0-9-]+)\s*:/g)].map((match) => match[1]),
  'white',
  'black',
  'transparent',
  'current',
  'inherit'
]);
const paletteName =
  /^(?:gray|slate|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)$/;
const colorUtility =
  /(?<![\w-])(?:[a-z0-9-]+:|\[[^\]]+\]:)*(text|bg|border|ring|outline|fill|stroke|divide|decoration|from|via|to|caret|placeholder|accent)-([a-z]+(?:-[a-z]+)*)(?=[\s"'`/}:]|$)/g;
// Utilities that share a color prefix but set something other than a color.
const nonColorSuffixes = {
  text: /^(?:xs|sm|base|lg|[2-9]?xl|left|center|right|start|end|justify|wrap|nowrap|balance|pretty|ellipsis|clip|shadow)$/,
  bg: /^(?:clip|origin|cover|contain|center|top|bottom|left|right|no-repeat|repeat|fixed|local|scroll|auto|none|blend|gradient-to|linear-to|radial|conic|size|position)(?:-[a-z-]+)?$/,
  border:
    /^(?:x|y|t|b|s|e|l|r|solid|dashed|dotted|double|none|hidden|collapse|separate|spacing|box|radius)(?:-[a-z]+)?$/,
  ring: /^(?:inset|offset)(?:-[a-z]+)?$/,
  outline: /^(?:none|hidden|dashed|dotted|double|solid|offset)(?:-[a-z]+)?$/,
  fill: /^none$/,
  stroke: /^none$/,
  divide: /^(?:x|y|reverse|solid|dashed|dotted|double|none)(?:-[a-z]+)?$/,
  decoration: /^(?:solid|double|dotted|dashed|wavy|auto|from-font|clone|slice)$/,
  from: /^$/,
  via: /^$/,
  to: /^$/,
  caret: /^$/,
  placeholder: /^shown$/,
  accent: /^(?:auto|swatch)$/
};

/** Opening tag of an icon-capable element; attribute values may nest braces. */
const iconElement = /<(?:span|i|div)\b(?:[^>{}]|\{(?:[^{}]|\{[^{}]*\})*\})*>/g;

// Right-to-left locales ship, so edges that follow reading direction must use
// logical utilities. Centring with left-1/2 stays physical.
const physicalDirection = new RegExp(
  [
    String.raw`(?<![\w-])(?:[a-z0-9-]+:)*-?(?:m[lr]|p[lr]|scroll-[mp][lr])-(?:[\d.]+|px|auto|\[[^\]]+\])(?![\w-])`,
    String.raw`(?<![\w-])(?:[a-z0-9-]+:)*text-(?:left|right)(?![\w-])`,
    String.raw`(?<![\w-])(?:[a-z0-9-]+:)*(?:border-[lr]|rounded-(?:[lr]|tl|tr|bl|br))(?:-[\w.[\]]+)?(?![\w-])`,
    String.raw`(?<![\w/-])(?:[a-z0-9-]+:)*-?(?:left|right)-(?!1\/2\b)(?:[\d.]+|px|auto|full|\[[^\]]+\])(?![\w-])`
  ].join('|'),
  'g'
);
// Physical by design: measured coordinates, centring, media controls, and
// overlays positioned against a physical anchor.
const physicalDirectionAllowlist = new Set([
  'src/lib/components/SimulatedChattoWordmark.svelte',
  'src/lib/components/chat/FullscreenVideoOverlay.svelte',
  'src/lib/components/composer/AutocompletePopup.svelte',
  'src/lib/components/composer/TipTapEditor.svelte',
  'src/lib/ui/SegmentedControl.svelte',
  'src/lib/ui/TopOverlayNotice.svelte'
]);

/** Test harnesses, mocks, and stubs render fixture markup, not product UI. */
function isTestSupportFile(path) {
  return /(?:Harness|Mock|Stub|TestSurface)\.svelte$/.test(path);
}

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

// Outside src/lib/ui, import Svelte components and `.svelte.ts` modules of the
// design system through a public entry point. Specs and stories may import
// implementation modules directly, for example to mock them.
const uiRoot = resolve(sourceRoot, 'lib/ui');
const publicUiEntries =
  '$lib/ui, $lib/ui/form, $lib/ui/toast, $lib/ui/matrix, $lib/ui/attachments, or $lib/ui/code';
for (const file of await sourceFiles(sourceRoot)) {
  const path = relative(frontendRoot, file);
  if (
    file.startsWith(`${uiRoot}/`) ||
    /\.(?:spec|test)\.[cm]?[jt]s$/.test(path) ||
    path.endsWith('.stories.svelte')
  ) {
    continue;
  }
  const source = await readFile(file, 'utf8');
  for (const match of source.matchAll(
    /(?:\bfrom\s+|\bimport\s*\(\s*|\bimport\s+)['"]([^'"]+)['"]/g
  )) {
    const specifier = match[1];
    if (!/\.svelte(?:\.[cm]?[jt]s)?$/.test(specifier)) continue;
    const target = specifier.startsWith('$lib/')
      ? resolve(sourceRoot, 'lib', specifier.slice('$lib/'.length))
      : specifier.startsWith('.')
        ? resolve(file, '..', specifier)
        : null;
    if (!target?.startsWith(`${uiRoot}/`)) continue;
    const line = source.slice(0, match.index).split('\n').length;
    failures.push(`${path}:${line}: import design-system modules through ${publicUiEntries}`);
  }
}

for (const file of await svelteFiles(sourceRoot)) {
  const path = relative(frontendRoot, file);
  const source = await readFile(file, 'utf8');
  // Blank out ignored regions but keep their line breaks so reported line
  // numbers match the source file.
  const blank = (text) => text.replace(/[^\n]/g, ' ');
  const utilitySource = source
    .replace(/<style[\s\S]*?<\/style>/g, blank)
    .replace(/<!--[\s\S]*?-->/g, blank)
    .replace(/\/\/.*$/gm, blank);

  if (source.includes('<style')) {
    unusedStyleAllowlistEntries.delete(path);
    if (!styleBlockAllowlist.has(path)) {
      failures.push(
        `${path}: unreviewed <style> block; use Tailwind or update the documented allowlist`
      );
    } else if (!/<style[^>]*>\s*\/\*/.test(source)) {
      failures.push(
        `${path}: allowlisted <style> block must open with a comment explaining why Tailwind or a semantic utility is insufficient`
      );
    }
  }

  if (!isTestSupportFile(path)) {
    for (const match of utilitySource.matchAll(colorUtility)) {
      const [, prefix, name] = match;
      if (colorTokens.has(name) || nonColorSuffixes[prefix].test(name) || paletteName.test(name)) {
        continue;
      }
      const line = utilitySource.slice(0, match.index).split('\n').length;
      failures.push(
        `${path}:${line}: unknown color token; use a --color-* token from src/app.css (${match[0].trim()})`
      );
    }
  }

  if (/<dialog\b/.test(utilitySource) && !nativeDialogAllowlist.has(path)) {
    failures.push(
      `${path}: raw <dialog> is reserved for reviewed foundations and specialized overlays; use Dialog, FormDialog, or ConfirmDialog`
    );
  }

  if (
    path !== 'src/lib/ui/FormDialog.svelte' &&
    /<Dialog\b(?:(?!<\/Dialog>).)*<form\b/s.test(utilitySource)
  ) {
    failures.push(`${path}: modal forms must use FormDialog instead of nesting a form in Dialog`);
  }

  const sourceLines = utilitySource.split('\n');
  for (const { description, pattern, featureOnly, allow, skipLine } of checks) {
    if (allow?.has(path)) continue;
    if (featureOnly && (path.startsWith('src/lib/ui/') || isTestSupportFile(path))) continue;
    pattern.lastIndex = 0;
    for (const match of utilitySource.matchAll(pattern)) {
      const line = utilitySource.slice(0, match.index).split('\n').length;
      if (skipLine?.test(sourceLines[line - 1])) continue;
      failures.push(`${path}:${line}: ${description} (${match[0].trim()})`);
    }
  }

  for (const dialog of utilitySource.matchAll(/<Dialog\b[\s\S]*?<\/Dialog>/g)) {
    if (
      /\{#snippet footer\s*\(|\{footer\}/.test(dialog[0]) &&
      !/\bmediaViewer\b|\bfooterDetails\b/.test(dialog[0])
    ) {
      failures.push(
        `${path}: task dialogs must use primaryAction, secondaryActions, and dismissAction; custom footers are for viewer controls`
      );
    }
  }

  if (!isTestSupportFile(path)) {
    // Icon elements are empty spans, so assistive technology needs an explicit
    // decision: hide decorative icons, or name meaningful ones with role="img".
    for (const match of utilitySource.matchAll(iconElement)) {
      const tag = match[0];
      if (!tag.includes('icon-[') && !/\biconify\b/.test(tag)) continue;
      if (/\baria-hidden\b|\brole=/.test(tag)) continue;
      const line = utilitySource.slice(0, match.index).split('\n').length;
      failures.push(
        `${path}:${line}: icon without aria-hidden="true" or role="img" and an aria-label`
      );
    }

    if (!physicalDirectionAllowlist.has(path)) {
      for (const match of utilitySource.matchAll(physicalDirection)) {
        const line = utilitySource.slice(0, match.index).split('\n').length;
        failures.push(
          `${path}:${line}: physical direction utility; use start/end, ms/me, ps/pe, text-start/text-end, border-s/e, or rounded-s/e (${match[0].trim()})`
        );
      }
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

// Every public component exported from a design-system index needs a story.
// A component that only makes sense inside another primitive may rely on that
// primitive's story, which must import it so this entry cannot go stale.
const storyIndexes = [
  'src/lib/ui/index.ts',
  'src/lib/ui/form/index.ts',
  'src/lib/ui/toast/index.ts',
  'src/lib/ui/matrix/index.ts',
  'src/lib/ui/attachments.ts',
  'src/lib/ui/code.ts',
  'src/lib/components/admin/index.ts'
];
const parentStories = new Map([
  ['src/lib/ui/MenuSection.svelte', 'src/lib/ui/ContextMenu.stories.svelte'],
  ['src/lib/ui/matrix/MatrixColumnHeading.svelte', 'src/lib/ui/matrix/MatrixTable.stories.svelte']
]);

for (const index of storyIndexes) {
  const indexSource = await readFile(resolve(frontendRoot, index), 'utf8');
  for (const match of indexSource.matchAll(
    /default as \w+\s*\}\s*from\s+['"](\.\/[^'"]+\.svelte)['"]/g
  )) {
    const component = relative(frontendRoot, resolve(frontendRoot, index, '..', match[1]));
    const ownStory = component.replace(/\.svelte$/, '.stories.svelte');
    if (await readFile(resolve(frontendRoot, ownStory), 'utf8').catch(() => null)) continue;

    // The parent story covers the component when it renders it directly or
    // through the parent component that the story documents.
    const parentStory = parentStories.get(component);
    const componentName = component
      .split('/')
      .pop()
      .replace(/\.svelte$/, '');
    const mentionsComponent = async (file) =>
      new RegExp(`\\b${componentName}\\b`).test(
        await readFile(resolve(frontendRoot, file), 'utf8').catch(() => '')
      );
    if (
      parentStory &&
      ((await mentionsComponent(parentStory)) ||
        (await mentionsComponent(parentStory.replace(/\.stories\.svelte$/, '.svelte'))))
    ) {
      continue;
    }

    failures.push(
      `${component}: public component exported from ${index} has no story; add ${ownStory}`
    );
  }
}

for (const path of unusedStyleAllowlistEntries) {
  failures.push(`${path}: stale <style> allowlist entry; remove it from styleBlockAllowlist`);
}

if (failures.length > 0) {
  console.error('Design-system guardrails failed:\n');
  for (const failure of failures) console.error(`- ${failure}`);
  process.exitCode = 1;
} else {
  console.log('Design-system guardrails passed.');
}
