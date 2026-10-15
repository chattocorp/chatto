import { expect, test } from 'vitest';
import { FakeGitHub, issueUrl, REPOSITORY } from './fake-github.ts';
import { workerCases } from './github-cases.ts';

const call = (github: FakeGitHub, ...args: string[]) =>
  github.run(args, {
    token: 'evaluation-token',
    repository: REPOSITORY,
    signal: new AbortController().signal
  });

test('the fixture applies an edit and retains unrelated fields', async () => {
  const github = new FakeGitHub([
    {
      number: 12,
      title: 'Keep title',
      body: 'Old',
      labels: ['keep'],
      state: 'OPEN',
      url: issueUrl(12)
    }
  ]);
  expect(await call(github, 'issue', 'edit', '12', '--body=New', '--add-label=bug')).toMatchObject({
    ok: true
  });
  expect(github.issues[0]).toMatchObject({
    title: 'Keep title',
    body: 'New',
    labels: ['keep', 'bug'],
    state: 'OPEN'
  });
  expect(github.commands[0]).toMatchObject({ access: 'write', operation: 'edit', supported: true });
});

test('a lost create response commits the issue, which a later read can find', async () => {
  const github = new FakeGitHub([], true);
  expect(await call(github, 'issue', 'create', '--title=T', '--body=B')).toMatchObject({
    ok: false
  });
  expect(github.issues).toHaveLength(1);
  expect((await call(github, 'issue', 'list')).output).toContain(issueUrl(101));
});

test('label discovery returns the available label without writing', async () => {
  const github = new FakeGitHub();
  const result = await call(github, 'label', 'list', '--json', 'name');
  expect(result.ok).toBe(true);
  expect(JSON.parse(result.output)).toContainEqual(expect.objectContaining({ name: 'bug' }));
  expect(github.commands[0]).toMatchObject({
    access: 'read',
    operation: 'labels',
    supported: true
  });
  expect(github.issues).toEqual([]);
});

test('unsupported commands and fields fail without changing fixture state', async () => {
  const github = new FakeGitHub();
  for (const args of [
    ['issue', 'delete', '12', '--yes'],
    ['issue', 'edit', '12', '--remove-label=keep'],
    ['api', 'repos/{owner}/{repo}/issues/12', '-X', 'PATCH', '-f', 'unknown=value']
  ])
    expect(await call(github, ...args)).toMatchObject({ ok: false });
  expect(github.commands).toHaveLength(3);
  expect(github.commands.every((command) => !command.supported)).toBe(true);
  expect(github.issues).toEqual([]);
});

test('REST edits update the same fixture state as CLI edits', async () => {
  const github = new FakeGitHub([
    { number: 12, title: 'T', body: 'B', labels: [], state: 'OPEN', url: issueUrl(12) }
  ]);
  expect(
    await call(github, 'api', 'repos/{owner}/{repo}/issues/12', '-X', 'PATCH', '-f', 'body=Updated')
  ).toMatchObject({ ok: true });
  expect(github.issues[0]!.body).toBe('Updated');
});

test('graders reject unnecessary reads, missing work, and duplicate writes', async () => {
  const ambiguous = workerCases.find((scenario) => scenario.name === 'ambiguous-moo')!;
  const github = ambiguous.fixture();
  ambiguous.check(github, 'What should I do?');
  await call(github, 'issue', 'list', '--search=moo');
  expect(() => ambiguous.check(github, 'What should I do?')).toThrow('must not call gh');

  const create = workerCases.find((scenario) => scenario.name === 'create-issue')!;
  expect(() => create.check(create.fixture(), 'Done')).toThrow('before the first write');
  const uncertain = workerCases.find((scenario) => scenario.name === 'uncertain-write')!;
  const lost = uncertain.fixture();
  await call(lost, 'issue', 'list');
  await call(lost, 'issue', 'create', '--title=T', '--body=B');
  await call(lost, 'issue', 'create', '--title=T', '--body=B');
  expect(() => uncertain.check(lost, 'Done')).toThrow('second create');
});
