/** Command policy for ChattoBot's gh tools. The policy protects the bot host only: it blocks gh
 * features that run programs, read or write local files, open a browser or an editor, handle
 * credentials, or contact another host. What ChattoBot may do on GitHub is set by the GitHub
 * App's permissions: read commands run with a read-only token, and every other command runs,
 * after the authorization check, with the permissions of the App installation. */

/** A read runs with the read-only token; anything else is a change. */
export type GhCommand = { access: 'read' | 'write' };

/** A command that the policy does not permit. The message is for the agent. */
export class GhPolicyError extends Error {
  override name = 'GhPolicyError';
}

const MAX_ARGS = 100;
const MAX_ARG = 65_536;

/** Command groups that act on GitHub resources. Other groups, such as `alias`, `extension`,
 * `auth`, `config`, `gist`, `codespace`, `browse`, and `attestation`, run programs, handle
 * credentials, or use local state. */
const GROUPS = new Set([
  'api',
  'cache',
  'issue',
  'label',
  'pr',
  'release',
  'repo',
  'ruleset',
  'run',
  'search',
  'secret',
  'variable',
  'workflow'
]);

/** Subcommands that read or write local files or Git state, and why. */
const LOCAL: Record<string, string> = {
  'pr checkout': 'changes a local checkout',
  'repo clone': 'writes to the bot host',
  'repo fork': 'can clone to the bot host',
  'repo sync': 'changes a local checkout',
  'repo set-default': 'changes local configuration',
  'run download': 'writes to the bot host',
  'release download': 'writes to the bot host',
  'release upload': 'reads files on the bot host',
  'release create': 'can upload files from the bot host'
};

/** Subcommands that only read. Everything else is a change. */
const READ_SUBCOMMANDS = new Set(['list', 'view', 'status', 'checks', 'diff', 'get', 'check']);

/** Flags that read local files, open a browser or an editor, or change the host or repository. */
const DENIED_FLAGS = new Set([
  '--repo',
  '--hostname',
  '--body-file',
  '--notes-file',
  '--env-file',
  '--input',
  '--web',
  '--editor',
  '--recover',
  '--template-file'
]);
/** Their shorthand forms. */
const DENIED_SHORTHANDS = new Set(['-R', '-w', '-e']);

/** True for an argument that gh parses as a flag. After `--`, gh parses no more flags. */
const isFlag = (arg: string) => /^-[A-Za-z-]/.test(arg) && arg !== '--';

/** Split `--name=value`. Short flags are always two characters; checkFlags rejects clusters
 * such as `-cq` and attached values such as `-L5` or `-F=x`, which gh would parse in ways
 * that hide a flag or its value from these checks. */
function splitFlag(arg: string): { name: string; value?: string } {
  const index = arg.startsWith('--') ? arg.indexOf('=') : -1;
  return index === -1 ? { name: arg } : { name: arg.slice(0, index), value: arg.slice(index + 1) };
}

/** Arguments before the `--` separator, where gh parses flags. */
const flagArgs = (args: readonly string[]) =>
  args.includes('--') ? args.slice(0, args.indexOf('--')) : args;

/** True when a `-F` field value reads a file or standard input (`key=@path`, `@-`). */
const readsFile = (value: string) =>
  (value.includes('=') ? value.slice(value.indexOf('=') + 1) : value).startsWith('@');

/** Reject host-affecting flags anywhere in the arguments. Outside `gh api`, `-F` is the
 * shorthand of `--body-file` or reads field values from files. jq and Go templates must not read
 * the environment. */
function checkFlags(args: readonly string[]) {
  const api = args[0] === 'api';
  // A value can be a URL, but a bare URL to another host is a positional argument that makes gh
  // contact that host. GitHub URLs are fine: tokens are limited to one repository.
  for (const arg of args)
    if (!/\s/.test(arg) && /:\/\//.test(arg) && !/^https:\/\/github\.com\//.test(arg) && !api)
      throw new GhPolicyError('Arguments can link only to https://github.com/.');
  const parsed = flagArgs(args);
  for (let index = 0; index < parsed.length; index++) {
    const arg = parsed[index]!;
    if (!isFlag(arg)) continue;
    if (!arg.startsWith('--') && arg.length > 2)
      throw new GhPolicyError(
        `${arg}: use each short flag on its own and its value as the next argument, for example -L 5.`
      );
    const { name, value } = splitFlag(arg);
    if (DENIED_FLAGS.has(name) || DENIED_SHORTHANDS.has(name))
      throw new GhPolicyError(
        `${name} is not available. The host selects the repository, and commands cannot read local files, open a browser, or open an editor.`
      );
    if (name === '-F' && !api)
      throw new GhPolicyError(
        '-F is not available here: it reads files. Pass text with body, or fields with -f.'
      );
    // Outside `gh api`, `-t` is `--title`.
    if (['-q', '--jq', '--template', ...(api ? ['-t'] : [])].includes(name)) {
      const expression = value ?? parsed[index + 1] ?? '';
      if (/\$ENV|\benv\b/.test(expression))
        throw new GhPolicyError('jq and template expressions cannot read the environment.');
    }
    if (name === '-F' || name === '--field') {
      const field = value ?? parsed[index + 1] ?? '';
      if (readsFile(field))
        throw new GhPolicyError('Field values cannot read files or standard input (@).');
    }
  }
}

/** api flags that take a value; all other api flags must be in API_BOOLEANS. */
const API_VALUE_FLAGS = new Set([
  '-X',
  '--method',
  '-f',
  '--raw-field',
  '-F',
  '--field',
  '-H',
  '--header',
  '-q',
  '--jq',
  '-t',
  '--template',
  '--cache',
  '-p',
  '--preview'
]);
const API_BOOLEANS = new Set(['--paginate', '--slurp', '-i', '--include', '--silent', '--verbose']);

/** Classify `gh api`: GET requests and GraphQL queries read; other requests change. Endpoints
 * must be relative, so that requests stay on GitHub. */
function classifyApi(args: readonly string[]): GhCommand {
  let method: string | undefined;
  const fields: string[] = [];
  const positionals: string[] = [];
  for (let index = 0; index < args.length; index++) {
    const arg = args[index]!;
    if (arg === '--') {
      positionals.push(...args.slice(index + 1));
      break;
    }
    if (!isFlag(arg)) {
      positionals.push(arg);
      continue;
    }
    const { name, value } = splitFlag(arg);
    if (API_BOOLEANS.has(arg)) continue;
    if (!API_VALUE_FLAGS.has(name)) throw new GhPolicyError(`gh api does not accept ${name} here.`);
    const flagValue = value ?? args[++index];
    if (flagValue === undefined) throw new GhPolicyError(`${name} needs a value.`);
    if (name === '-X' || name === '--method') method = flagValue.toUpperCase();
    if (['-f', '--raw-field', '-F', '--field'].includes(name)) fields.push(flagValue);
  }
  if (positionals.length !== 1) throw new GhPolicyError('gh api needs exactly one endpoint.');
  const endpoint = positionals[0]!.replace(/^\//, '');
  if (/:\/\/|\.\.|^https?:/i.test(endpoint))
    throw new GhPolicyError('gh api accepts only relative endpoints.');
  if (endpoint === 'graphql')
    return { access: fields.some((field) => /\bmutation\b/i.test(field)) ? 'write' : 'read' };
  const effectiveMethod = method ?? (fields.length ? 'POST' : 'GET');
  return { access: effectiveMethod === 'GET' || effectiveMethod === 'HEAD' ? 'read' : 'write' };
}

/** Classify the arguments of one gh command, without the `gh` program name. Throws a
 * GhPolicyError for a command that the policy does not permit. */
export function classifyGh(args: readonly string[]): GhCommand {
  if (!args.length || args.length > MAX_ARGS)
    throw new GhPolicyError(`Pass between 1 and ${MAX_ARGS} arguments.`);
  if (args.some((arg) => arg.length > MAX_ARG || arg.includes('\0')))
    throw new GhPolicyError('An argument is too long or contains a NUL character.');
  checkFlags(args);
  const [group, subcommand] = args as [string, string | undefined];
  if (!GROUPS.has(group))
    throw new GhPolicyError(`gh ${group} is not available. Available: ${[...GROUPS].join(', ')}.`);
  if (group === 'api') return classifyApi(args.slice(1));
  if (group === 'search') return { access: 'read' };
  if (!subcommand || isFlag(subcommand))
    throw new GhPolicyError(`Name a ${group} subcommand, such as gh ${group} list.`);
  const local = LOCAL[`${group} ${subcommand}`];
  if (local) throw new GhPolicyError(`gh ${group} ${subcommand} is not available: it ${local}.`);
  return { access: READ_SUBCOMMANDS.has(subcommand) ? 'read' : 'write' };
}

/** Quote an argument for display in a POSIX shell command line. */
export function shellQuote(arg: string): string {
  return /^[A-Za-z0-9_@%+=:,./-]+$/.test(arg) ? arg : `'${arg.replaceAll("'", `'\\''`)}'`;
}
