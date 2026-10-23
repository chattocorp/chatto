/** In-memory GitHub for model evaluations. Every command is recorded, including rejected
 * commands. There is no shell or network fallback, and no real credentials are loaded. */
import { generateKeyPairSync } from 'node:crypto';
import type { GhRunner } from '../github/gh.ts';
import { classifyGh } from '../github/policy.ts';
import type { GitHubToolOptions } from '../github/tools.ts';

export interface Issue {
  number: number;
  title: string;
  body: string;
  labels: string[];
  /** GitHub's native issue type, independent of labels. */
  issueType?: { id: number; name: string };
  /** Native field values for the priority evaluation, independent of labels and type. */
  fieldValues?: {
    issue_field_id: number;
    value: number;
    single_select_option?: { id: number; name: string };
  }[];
  state: 'OPEN' | 'CLOSED';
  stateReason?: 'completed' | 'not planned';
  comments?: string[];
  url: string;
}

export interface Command {
  args: readonly string[];
  access: 'read' | 'write';
  operation: string;
  /** False for commands outside this fixture's implemented CLI subset. */
  supported: boolean;
}

export const REPOSITORY = 'evaluation/fixture';
export const issueUrl = (number: number) => `https://github.com/${REPOSITORY}/issues/${number}`;

/** Return a flag's value for either --flag=value or --flag value. */
function flag(args: readonly string[], ...names: string[]): string | undefined {
  for (let i = 0; i < args.length; i++)
    for (const name of names) {
      if (args[i] === name) return args[i + 1];
      if (args[i]!.startsWith(`${name}=`)) return args[i]!.slice(name.length + 1);
    }
  return undefined;
}

/** Parse CLI flags before dispatch, so a title or body cannot be mistaken for a target. */
function positionals(args: readonly string[]): string[] {
  const output: string[] = [];
  for (let i = 0; i < args.length; i++) {
    const arg = args[i]!;
    if (!arg.startsWith('-')) output.push(arg);
    else if (!arg.includes('=') && !['--comments', '--yes'].includes(arg)) i++;
  }
  return output;
}

/** A small REST/CLI fixture for issues. Unsupported commands fail the evaluation instead of
 * silently succeeding. An optional lost response simulates a write that committed remotely. */
export class FakeGitHub {
  readonly commands: Command[] = [];
  readonly issues: Issue[];
  private lostResponse: boolean;

  constructor(
    issues: Issue[] = [],
    loseCreateResponse = false,
    readonly issueTypes = ['Bug', 'Feature', 'Enhancement'].map((name, index) => ({
      id: index + 1,
      name
    }))
  ) {
    this.issues = structuredClone(issues);
    this.lostResponse = loseCreateResponse;
  }

  readonly run: GhRunner = async (args, { signal, repository, token }) => {
    signal.throwIfAborted();
    if (repository !== REPOSITORY || token !== 'evaluation-token')
      throw new Error('Evaluation must use fixture credentials and repository');
    const command: Command = {
      args: [...args],
      access: classifyGh(args).access,
      operation: '',
      supported: false
    };
    this.commands.push(command);
    if (this.commands.length > 16) throw new Error('Evaluation command budget exceeded');
    try {
      const { operation, target, fields } = this.parse(args);
      command.operation = operation;
      if (
        ![
          'list',
          'labels',
          'types',
          'field-definitions',
          'field-values',
          'set-field',
          'repository',
          'view',
          'create',
          'edit',
          'close',
          'comment'
        ].includes(operation)
      )
        throw new Error('Unsupported fixture command');
      command.supported = true;
      if (operation === 'repository')
        return { ok: true, output: JSON.stringify({ nameWithOwner: REPOSITORY }) };
      if (operation === 'types') return { ok: true, output: JSON.stringify(this.issueTypes) };
      if (operation === 'field-definitions')
        return {
          ok: true,
          output: JSON.stringify([
            {
              id: 7,
              name: 'Priority',
              data_type: 'single_select',
              options: [
                { id: 71, name: 'High' },
                { id: 72, name: 'Low' }
              ]
            },
            { id: 8, name: 'Effort', data_type: 'number' }
          ])
        };
      const issueType =
        fields.type === undefined
          ? undefined
          : this.issueTypes.find((type) => type.name === fields.type);
      if (fields.type !== undefined && !issueType)
        return { ok: false, output: 'Issue type not found' };
      if (operation === 'labels')
        return {
          ok: true,
          output: JSON.stringify([{ name: 'bug', color: 'd73a4a', description: 'A defect' }])
        };
      if (operation === 'list') return this.json(this.issues, flag(args, '--json'));
      const issue = this.issues.find((item) => item.number === target);
      if (operation === 'view') {
        if (!issue) return { ok: false, output: 'Issue not found' };
        return this.json(issue, flag(args, '--json'));
      }
      if (operation === 'create') {
        if (!fields.title || !fields.body) throw new Error('Create needs a title and body');
        const number = Math.max(100, ...this.issues.map((item) => item.number)) + 1;
        const created: Issue = {
          number,
          title: fields.title,
          body: fields.body,
          labels: fields.addLabel?.split(',') ?? [],
          ...(issueType ? { issueType } : {}),
          state: 'OPEN',
          url: issueUrl(number)
        };
        this.issues.push(created);
        if (this.lostResponse) {
          this.lostResponse = false;
          return {
            ok: false,
            output:
              'Connection lost after submitting the request. The issue may have been created. Check GitHub before retrying.'
          };
        }
        return { ok: true, output: created.url };
      }
      if (!issue) return { ok: false, output: 'Issue not found' };
      if (operation === 'field-values')
        return { ok: true, output: JSON.stringify(issue.fieldValues ?? []) };
      if (operation === 'set-field') {
        if (fields.fieldId !== '7' || fields.fieldValue !== 'High')
          throw new Error('Fixture expects Priority by field ID and option name');
        issue.fieldValues = [
          ...(issue.fieldValues ?? []).filter((value) => value.issue_field_id !== 7),
          { issue_field_id: 7, value: 71, single_select_option: { id: 71, name: 'High' } }
        ];
        return { ok: true, output: JSON.stringify(issue.fieldValues) };
      }
      if (operation === 'comment') {
        if (!fields.body) throw new Error('Comment needs a body');
        (issue.comments ??= []).push(fields.body);
        return { ok: true, output: `${issue.url}#issuecomment-${issue.comments.length}` };
      }
      if (operation === 'close') {
        const reason = fields.reason ?? 'completed';
        if (reason !== 'completed' && reason !== 'not planned')
          throw new Error('Unsupported closure reason');
        issue.state = 'CLOSED';
        issue.stateReason = reason;
        if (fields.comment) (issue.comments ??= []).push(fields.comment);
      }
      if (fields.title !== undefined) issue.title = fields.title;
      if (fields.body !== undefined) issue.body = fields.body;
      if (issueType) issue.issueType = issueType;
      if (fields.addLabel)
        issue.labels = [...new Set([...issue.labels, ...fields.addLabel.split(',')])];
      return { ok: true, output: issue.url };
    } catch (error) {
      command.supported = false;
      return {
        ok: false,
        output: error instanceof Error ? error.message : 'Unsupported fixture command'
      };
    }
  };

  /** Settings compatible with production tools, backed only by this fixture. */
  settings(): GitHubToolOptions {
    return {
      settings: {
        repository: REPOSITORY,
        clientId: 'Iv23liEvaluation',
        privateKey: generateKeyPairSync('rsa', { modulusLength: 2048 }).privateKey
      },
      tokens: async () => 'evaluation-token',
      run: this.run
    };
  }

  /** Honor CLI field selection so a title-only search cannot reveal unread issue bodies. */
  private json(value: Issue | Issue[], fields?: string) {
    const selected = fields?.split(',');
    const encode = (issue: Issue) => {
      const full = { ...issue, labels: issue.labels.map((name) => ({ name })) };
      return selected
        ? Object.fromEntries(Object.entries(full).filter(([key]) => selected.includes(key)))
        : full;
    };
    return {
      ok: true,
      output: JSON.stringify(Array.isArray(value) ? value.map(encode) : encode(value))
    };
  }

  private parse(args: readonly string[]): {
    operation: string;
    target?: number;
    fields: {
      title?: string;
      body?: string;
      type?: string;
      addLabel?: string;
      reason?: string;
      comment?: string;
      fieldId?: string;
      fieldValue?: string;
    };
  } {
    if (args[0] === 'issue') {
      const allowed = new Set([
        '--title',
        '--type',
        '-t',
        '--body',
        '-b',
        '--add-label',
        '--label',
        '-l',
        '--json',
        '--jq',
        '-q',
        '--search',
        '-S',
        '--state',
        '-s',
        '--limit',
        '-L',
        '--comments',
        '--reason',
        '-r',
        '--comment',
        '-c'
      ]);
      for (let i = 2; i < args.length; i++) {
        const arg = args[i]!;
        if (!arg.startsWith('-')) continue;
        const name = arg.split('=', 1)[0]!;
        if (!allowed.has(name)) throw new Error('Unsupported fixture CLI flag');
        if (!arg.includes('=') && name !== '--comments') i++;
      }
      const target = positionals(args.slice(2))[0];
      return {
        operation: args[1] ?? '',
        target: target === undefined ? undefined : Number(target.match(/(?:^|\/)(\d+)$/)?.[1]),
        fields: {
          title: flag(args, '--title', '-t'),
          type: flag(args, '--type'),
          body: flag(args, '--body', '-b'),
          reason: flag(args, '--reason', '-r'),
          comment: flag(args, '--comment', '-c'),
          addLabel: args[1] === 'create' ? flag(args, '--label', '-l') : flag(args, '--add-label')
        }
      };
    }
    if (args[0] === 'repo' && args[1] === 'view' && flag(args, '--json') === 'nameWithOwner')
      return { operation: 'repository', fields: {} };
    if (args[0] === 'search' && args[1] === 'issues') {
      if (flag(args, '--search') !== undefined)
        throw new Error('gh search issues takes a positional query, not --search');
      return { operation: 'list', fields: {} };
    }
    if (args[0] === 'label' && args[1] === 'list') return { operation: 'labels', fields: {} };
    if (args[0] !== 'api') throw new Error('Unsupported fixture command');
    const endpoint = positionals(args.slice(1))[0]
      ?.replace(/^\//, '')
      .replace('repos/{owner}/{repo}', `repos/${REPOSITORY}`)
      .replace('orgs/{owner}', 'orgs/evaluation')
      .split('?')[0];
    if (endpoint === `repos/${REPOSITORY}/issue-types` && classifyGh(args).access === 'read')
      return { operation: 'types', fields: {} };
    if (endpoint === 'orgs/evaluation/issue-fields' && classifyGh(args).access === 'read')
      return { operation: 'field-definitions', fields: {} };
    const match = endpoint?.match(
      /^repos\/evaluation\/fixture\/issues(?:\/(\d+))?(\/issue-field-values)?$/
    );
    if (!match) throw new Error('Unsupported fixture API endpoint');
    const fields: Record<string, string> = {};
    for (let i = 1; i < args.length; i++) {
      const arg = args[i]!;
      let field: string | undefined;
      if (['-f', '-F', '--field', '--raw-field'].includes(arg)) field = args[++i];
      else if (arg.startsWith('--field=') || arg.startsWith('--raw-field='))
        field = arg.slice(arg.indexOf('=') + 1);
      if (field) {
        const index = field.indexOf('=');
        if (index > 0) fields[field.slice(0, index)] = field.slice(index + 1);
      }
    }
    if (
      Object.keys(fields).some(
        (key) =>
          ![
            'title',
            'body',
            'type',
            'state',
            'state_reason',
            'issue_field_values[][field_id]',
            'issue_field_values[][value]'
          ].includes(key)
      )
    )
      throw new Error('Unsupported fixture API field');
    const method = flag(args, '--method', '-X') ?? (Object.keys(fields).length ? 'POST' : 'GET');
    const target = match[1] === undefined ? undefined : Number(match[1]);
    if (match[2])
      return {
        operation: method === 'GET' ? 'field-values' : method === 'POST' ? 'set-field' : '',
        target,
        fields: {
          fieldId: fields['issue_field_values[][field_id]'],
          fieldValue: fields['issue_field_values[][value]']
        }
      };
    const operation =
      method === 'GET'
        ? target
          ? 'view'
          : 'list'
        : method === 'POST' && !target
          ? 'create'
          : method === 'PATCH' && target
            ? fields.state === 'closed'
              ? 'close'
              : 'edit'
            : '';
    return {
      operation,
      target,
      fields: { ...fields, reason: fields.state_reason?.replace('_', ' ') }
    };
  }
}
