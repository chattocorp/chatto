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

/** The subcommands of each command group, as reads and changes. Only these names work: gh
 * aliases, such as `release new` for `release create`, and subcommands that read or write local
 * files or Git state (`release create`, `release upload`, `release download`, `run download`,
 * `pr checkout`, `pr create`, `repo clone`, `repo create`, `repo deploy-key`) are not listed.
 * Groups that run programs or handle credentials (`alias`, `extension`, `auth`, `config`) are
 * not listed either. `gh api` is classified separately. */
const COMMANDS: Record<string, { read: readonly string[]; write: readonly string[] }> = {
  cache: { read: ['list'], write: ['delete'] },
  issue: {
    read: ['list', 'view', 'status'],
    write: [
      'create',
      'edit',
      'comment',
      'close',
      'reopen',
      'delete',
      'lock',
      'unlock',
      'pin',
      'unpin',
      'transfer'
    ]
  },
  label: { read: ['list'], write: ['create', 'edit', 'delete'] },
  pr: {
    read: ['list', 'view', 'checks', 'diff', 'status'],
    write: [
      'comment',
      'edit',
      'review',
      'close',
      'reopen',
      'merge',
      'ready',
      'lock',
      'unlock',
      'update-branch'
    ]
  },
  release: { read: ['list', 'view'], write: ['edit', 'delete', 'delete-asset'] },
  repo: { read: ['view'], write: ['edit'] },
  ruleset: { read: ['list', 'view', 'check'], write: [] },
  run: { read: ['list', 'view'], write: ['rerun', 'cancel', 'delete'] },
  search: { read: ['issues', 'prs', 'code', 'commits'], write: [] },
  secret: { read: ['list'], write: ['set', 'delete'] },
  variable: { read: ['list', 'get'], write: ['set', 'delete'] },
  workflow: { read: ['list', 'view'], write: ['run', 'enable', 'disable'] }
};

/** Flags that read local files, open a browser or an editor, or change the host or repository. */
const DENIED_FLAGS = new Set([
  '--repo',
  '--hostname',
  '--body-file',
  '--notes-file',
  '--env-file',
  '--attach',
  '--input',
  '--web',
  '--editor',
  '--recover',
  '--template-file'
]);
/** Their shorthand forms. */
const DENIED_SHORTHANDS = new Set(['-R', '-w', '-e']);

/** True for an argument that gh can parse as a flag. The policy rejects `--`: gh reads it as the
 * value of a preceding flag, so it cannot mark the end of flags safely. */
const isFlag = (arg: string) => /^-[A-Za-z-]/.test(arg);

/** Flags whose values gh never reads as a repository, such as branch names and text. In the
 * `--flag=value` form, their values can contain slashes. A separate value argument is checked like
 * any other argument, because the policy cannot tell whether the previous argument is a flag. */
const TEXT_VALUE_FLAGS = new Set([
  '--head',
  '--base',
  '--branch',
  '--label',
  '--add-label',
  '--remove-label',
  '--milestone',
  '--title',
  '--body',
  '--search',
  '--json',
  '--jq',
  '-q',
  '--template'
]);

/** True when gh could contact a host other than github.com because of this argument: a URL to
 * another host anywhere in it, an scp-style `git@host:path`, or a `HOST/OWNER/REPO` with three
 * or more segments. `text` marks the value of a flag in TEXT_VALUE_FLAGS, which gh does not read
 * as a repository; only the URL check applies to it. */
function namesOtherHost(arg: string, text: boolean): boolean {
  for (const [url] of arg.matchAll(/[a-z][a-z0-9+.-]*:\/\/\S*/gi))
    if (!/^https:\/\/github\.com\//i.test(url)) return true;
  if (text) return false;
  const value = arg.trimStart();
  if (/^https:\/\/github\.com\//i.test(value)) return false;
  // gh treats a `git@` prefix as an scp address and splits other values on `/`, whatever they
  // contain, so whitespace after the host does not make an argument harmless.
  return value.startsWith('git@') || /^[^/\s]+\/[^/]+\/[^/]/.test(value);
}

/** Split `--name=value`. Short flags are always two characters; checkFlags rejects clusters
 * such as `-cq` and attached values such as `-L5` or `-F=x`, which gh would parse in ways
 * that hide a flag or its value from these checks. */
function splitFlag(arg: string): { name: string; value?: string } {
  const index = arg.startsWith('--') ? arg.indexOf('=') : -1;
  return index === -1 ? { name: arg } : { name: arg.slice(0, index), value: arg.slice(index + 1) };
}

/** True when a `-F` field value reads a file or standard input (`key=@path`, `@-`). */
const readsFile = (value: string) =>
  (value.includes('=') ? value.slice(value.indexOf('=') + 1) : value).startsWith('@');

/** Reject host-affecting flags anywhere in the arguments. Outside `gh api`, `-F` is the
 * shorthand of `--body-file` or reads field values from files. jq and Go templates must not read
 * the environment. */
function checkFlags(args: readonly string[]) {
  const api = args[0] === 'api';
  const workflowRun = args[0] === 'workflow' && args[1] === 'run';
  // In `label create` and `label edit`, -f is --force.
  const label = args[0] === 'label';
  if (args.includes('--'))
    throw new GhPolicyError('`--` is not available. Pass values with their flags.');
  // gh contacts the host of a repository or URL argument. Only github.com is allowed; tokens are
  // limited to one repository there. `gh api` endpoints are checked separately.
  if (!api)
    for (let index = 0; index < args.length; index++) {
      const arg = args[index]!;
      const { name, value } = splitFlag(arg);
      const text = value !== undefined && TEXT_VALUE_FLAGS.has(name);
      if (namesOtherHost(value ?? arg, text))
        throw new GhPolicyError(
          'Arguments can name only OWNER/REPO or https://github.com/ addresses. Write a branch name with slashes as --flag=value, for example --head=a/b/c, and put text with links to other sites in the body parameter of ghWrite.'
        );
    }
  // Check every argument, including flag values: a value can hide a flag from a simple parser.
  const parsed = args;
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
    // Outside `gh api` and `gh workflow run`, `-f` is the shorthand of `--env-file`.
    if (name === '-f' && !api && !workflowRun && !label)
      throw new GhPolicyError('-f is not available here: it reads a file. Pass values directly.');
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
  if (group === 'api') return classifyApi(args.slice(1));
  const commands = COMMANDS[group];
  if (!commands)
    throw new GhPolicyError(
      `gh ${group} is not available. Available: api, ${Object.keys(COMMANDS).join(', ')}.`
    );
  if (subcommand && commands.read.includes(subcommand)) return { access: 'read' };
  if (subcommand && commands.write.includes(subcommand)) return { access: 'write' };
  throw new GhPolicyError(
    `gh ${group} ${subcommand ?? ''} is not available. Available: ${[...commands.read, ...commands.write].join(', ')}.`
  );
}

/** Quote an argument for display in a POSIX shell command line. */
export function shellQuote(arg: string): string {
  return /^[A-Za-z0-9_@%+=:,./-]+$/.test(arg) ? arg : `'${arg.replaceAll("'", `'\\''`)}'`;
}
