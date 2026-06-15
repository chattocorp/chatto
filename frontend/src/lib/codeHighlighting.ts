import { createLowlight } from 'lowlight';
import type { LanguageFn } from 'highlight.js';

type HighlightLanguageModule = {
  default: LanguageFn;
};

const languageModules = import.meta.glob<HighlightLanguageModule>(
  [
    '/node_modules/highlight.js/es/languages/*.js',
    '!/node_modules/highlight.js/es/languages/*.js.js'
  ]
);

const languageAliases: Record<string, string> = {
  as: 'actionscript',
  asc: 'angelscript',
  apacheconf: 'apache',
  osascript: 'applescript',
  ino: 'arduino',
  arm: 'armasm',
  adoc: 'asciidoc',
  ahk: 'autohotkey',
  'x++': 'axapta',
  sh: 'bash',
  zsh: 'bash',
  bf: 'brainfuck',
  h: 'c',
  capnp: 'capnproto',
  icl: 'clean',
  dcl: 'clean',
  clj: 'clojure',
  edn: 'clojure',
  'cmake.in': 'cmake',
  coffee: 'coffeescript',
  cson: 'coffeescript',
  iced: 'coffeescript',
  cls: 'cos',
  cc: 'cpp',
  'c++': 'cpp',
  'h++': 'cpp',
  hpp: 'cpp',
  hh: 'cpp',
  hxx: 'cpp',
  cxx: 'cpp',
  crm: 'crmsh',
  pcmk: 'crmsh',
  cr: 'crystal',
  cs: 'csharp',
  'c#': 'csharp',
  dpr: 'delphi',
  dfm: 'delphi',
  pas: 'delphi',
  pascal: 'delphi',
  patch: 'diff',
  jinja: 'django',
  bind: 'dns',
  zone: 'dns',
  docker: 'dockerfile',
  bat: 'dos',
  cmd: 'dos',
  dst: 'dust',
  ex: 'elixir',
  exs: 'elixir',
  erl: 'erlang',
  xlsx: 'excel',
  xls: 'excel',
  f90: 'fortran',
  f95: 'fortran',
  fs: 'fsharp',
  'f#': 'fsharp',
  gms: 'gams',
  gss: 'gauss',
  nc: 'gcode',
  feature: 'gherkin',
  golang: 'go',
  gql: 'graphql',
  hbs: 'handlebars',
  'html.hbs': 'handlebars',
  'html.handlebars': 'handlebars',
  htmlbars: 'handlebars',
  hs: 'haskell',
  hx: 'haxe',
  https: 'http',
  hylang: 'hy',
  i7: 'inform7',
  toml: 'ini',
  jsp: 'java',
  js: 'javascript',
  jsx: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  'wildfly-cli': 'jboss-cli',
  jsonc: 'json',
  jldoctest: 'julia-repl',
  kt: 'kotlin',
  kts: 'kotlin',
  ls: 'livescript',
  lassoscript: 'lasso',
  tex: 'latex',
  pluto: 'lua',
  mk: 'makefile',
  mak: 'makefile',
  make: 'makefile',
  md: 'markdown',
  mkdown: 'markdown',
  mkd: 'markdown',
  mma: 'mathematica',
  wl: 'mathematica',
  m: 'mercury',
  moo: 'mercury',
  mips: 'mipsasm',
  moon: 'moonscript',
  nt: 'nestedtext',
  nginxconf: 'nginx',
  nixos: 'nix',
  mm: 'objectivec',
  objc: 'objectivec',
  'obj-c': 'objectivec',
  'obj-c++': 'objectivec',
  'objective-c++': 'objectivec',
  ml: 'sml',
  scad: 'openscad',
  pl: 'perl',
  pm: 'perl',
  'pf.conf': 'pf',
  postgres: 'pgsql',
  postgresql: 'pgsql',
  text: 'plaintext',
  txt: 'plaintext',
  pwsh: 'powershell',
  ps: 'powershell',
  ps1: 'powershell',
  pde: 'processing',
  proto: 'protobuf',
  pp: 'puppet',
  pb: 'purebasic',
  pbi: 'purebasic',
  py: 'python',
  gyp: 'python',
  ipython: 'python',
  pycon: 'python-repl',
  k: 'q',
  kdb: 'q',
  qt: 'qml',
  re: 'reasonml',
  graph: 'roboconf',
  instances: 'roboconf',
  mikrotik: 'routeros',
  rb: 'ruby',
  gemspec: 'ruby',
  podspec: 'ruby',
  thor: 'ruby',
  irb: 'ruby',
  rs: 'rust',
  scm: 'scheme',
  sci: 'scilab',
  console: 'shell',
  shellsession: 'shell',
  st: 'smalltalk',
  stanfuncs: 'stan',
  do: 'stata',
  ado: 'stata',
  p21: 'step21',
  step: 'step21',
  stp: 'step21',
  styl: 'stylus',
  tk: 'tcl',
  craftcms: 'twig',
  ts: 'typescript',
  tsx: 'typescript',
  mts: 'typescript',
  cts: 'typescript',
  vb: 'vbnet',
  vbs: 'vbscript',
  v: 'verilog',
  sv: 'verilog',
  svh: 'verilog',
  tao: 'xl',
  html: 'xml',
  xhtml: 'xml',
  rss: 'xml',
  atom: 'xml',
  xjb: 'xml',
  xsd: 'xml',
  xsl: 'xml',
  plist: 'xml',
  wsf: 'xml',
  svg: 'xml',
  xpath: 'xquery',
  xq: 'xquery',
  xqm: 'xquery',
  yml: 'yaml',
  zep: 'zephir'
};

const aliasesByLanguage: Record<string, string[]> = {};

for (const [alias, language] of Object.entries(languageAliases)) {
  aliasesByLanguage[language] ??= [];
  aliasesByLanguage[language].push(alias);
}

const languageImporters = new Map<string, () => Promise<HighlightLanguageModule>>();

for (const [path, importer] of Object.entries(languageModules)) {
  const language = path.match(/\/([^/]+)\.js$/)?.[1];
  if (language) languageImporters.set(language, importer);
}

const languageLoadPromises = new Map<string, Promise<boolean>>();

const preferredCodeLanguageOptions = [
  { value: 'text', label: 'TEXT' },
  { value: 'ts', label: 'TS' },
  { value: 'js', label: 'JS' },
  { value: 'json', label: 'JSON' },
  { value: 'html', label: 'HTML' },
  { value: 'css', label: 'CSS' },
  { value: 'bash', label: 'BASH' },
  { value: 'py', label: 'PY' },
  { value: 'go', label: 'GO' },
  { value: 'rust', label: 'RUST' },
  { value: 'sql', label: 'SQL' },
  { value: 'yaml', label: 'YAML' },
  { value: 'md', label: 'MD' },
  { value: 'graphql', label: 'GRAPHQL' },
  { value: 'dockerfile', label: 'DOCKERFILE' },
  { value: 'ruby', label: 'RUBY' }
];

const preferredCodeLanguageValues = new Set(
  preferredCodeLanguageOptions.map((language) => language.value)
);

export const CODE_LANGUAGE_OPTIONS = [
  ...preferredCodeLanguageOptions,
  ...[...languageImporters.keys()]
    .filter((language) => !preferredCodeLanguageValues.has(language))
    .sort()
    .map((language) => ({
      value: language,
      label: language.toUpperCase()
    }))
];

export const lowlight = createLowlight();

export function normalizeCodeLanguage(language: string | null | undefined): string {
  const token = language?.trim().toLowerCase().match(/[a-z0-9+#_.-]+/)?.[0];
  return token || 'text';
}

export function resolveCodeLanguage(language: string | null | undefined): string | null {
  const normalized = normalizeCodeLanguage(language);
  if (languageImporters.has(normalized)) return normalized;
  return languageAliases[normalized] ?? null;
}

export function canHighlightCodeLanguage(language: string): boolean {
  return resolveCodeLanguage(language) !== null;
}

export function isCodeLanguageLoaded(language: string | null | undefined): boolean {
  const normalized = normalizeCodeLanguage(language);
  const resolved = resolveCodeLanguage(normalized);
  return Boolean(resolved && (lowlight.registered(normalized) || lowlight.registered(resolved)));
}

export async function ensureCodeLanguageLoaded(language: string | null | undefined): Promise<boolean> {
  const resolved = resolveCodeLanguage(language);
  if (!resolved) return false;
  if (lowlight.registered(resolveCodeLanguage(language) ?? resolved)) return false;

  const existing = languageLoadPromises.get(resolved);
  if (existing) return existing;

  const loadPromise = (async () => {
    const importer = languageImporters.get(resolved);
    if (!importer) return false;

    try {
      const module = await importer();
      lowlight.register(resolved, module.default);
      const aliases = aliasesByLanguage[resolved];
      if (aliases) lowlight.registerAlias(resolved, aliases);
      return true;
    } catch (err) {
      languageLoadPromises.delete(resolved);
      console.warn('[CodeHighlighting] Failed to load language:', resolved, err);
      return false;
    }
  })();

  languageLoadPromises.set(resolved, loadPromise);
  return loadPromise;
}

export async function ensureCodeLanguagesLoaded(
  languages: Iterable<string | null | undefined>
): Promise<boolean> {
  const results = await Promise.all([...languages].map((language) => ensureCodeLanguageLoaded(language)));
  return results.some(Boolean);
}
