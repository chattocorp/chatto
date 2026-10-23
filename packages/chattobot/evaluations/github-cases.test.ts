import { expect, test } from 'vitest';
import { FakeGitHub, issueUrl, REPOSITORY } from './fake-github.ts';
import { checkGitHubLink, checkIssueType, supervisorCases, workerCases } from './github-cases.ts';

const call = (github: FakeGitHub, ...args: string[]) =>
  github.run(args, {
    token: 'evaluation-token',
    repository: REPOSITORY,
    signal: new AbortController().signal
  });

test('the no-match grader accepts natural clarification and rejects the old search-first reply', async () => {
  const scenario = supervisorCases.find((item) => item.name === 'issue-discovery-no-match')!;
  const github = scenario.fixture!();
  await call(github, 'issue', 'list', '--state', 'all');
  scenario.check!(
    ['This appears to be a new feature request. Who should be able to upload emojis?'],
    github.commands,
    []
  );
  scenario.check!(['Who should be able to upload custom emojis?'], github.commands, []);
  expect(() =>
    scenario.check!(
      ["I couldn't find an issue. This appears to be a new request. Who should upload emojis?"],
      github.commands,
      []
    )
  ).toThrow('empty issue search');
});

test.each(['issues', 'pull'])('GitHub %s references need a linked, visible number', (kind) => {
  const url = `https://github.com/evaluation/fixture/${kind}/12`;
  checkGitHubLink(`See [#12 — Custom emojis](${url}).`, url, 12);
  for (const reference of [url, '#12', `#12: [Custom emojis](${url})`, `[#123](${url})`])
    expect(() => checkGitHubLink(reference, url, 12)).toThrow('number in the visible text');
});

test('the discovery grader allows an unreleased fix but rejects a claimed release', async () => {
  const scenario = supervisorCases.find((item) => item.name === 'issue-discovery-bug')!;
  const github = scenario.fixture!();
  await call(github, 'issue', 'list', '--state', 'all');
  await call(github, 'issue', 'view', '12');
  const answer = `[#12](${issueUrl(12)}) proposes refreshing the timeline. Does that match?`;
  scenario.check!([`${answer} No fix is released yet.`], github.commands, []);
  expect(() => scenario.check!([`${answer} The fix is released.`], github.commands, [])).toThrow(
    'shipped feature or fix'
  );
});

test('the greeting grader allows a social question but rejects a slang clarification', () => {
  const check = supervisorCases.find((scenario) => scenario.name === 'casual-greeting')!.check!;
  check(["Hey! What's up?"], [], []);
  expect(() => check(['What do you mean by my mang?'], [], [])).toThrow('greeting slang');
});

test('the image report grader rejects missing fallback search and identity disclosure', async () => {
  const scenario = workerCases.find((item) => item.name === 'private-image-report')!;
  const github = scenario.fixture();
  await call(github, 'issue', 'list', '--search', 'images no error message');
  await call(
    github,
    'issue',
    'create',
    '--title',
    'Images do not load',
    '--type=Bug',
    '--body',
    'Unconfirmed report.'
  );
  expect(() => scenario.check(github, issueUrl(101))).toThrow('one broader search');

  const fallback = scenario.fixture();
  await call(fallback, 'issue', 'list', '--search', 'images no error message');
  await call(fallback, 'issue', 'list', '--search', 'images');
  await call(
    fallback,
    'issue',
    'create',
    '--title',
    'Images do not load',
    '--type=Bug',
    '--body',
    'Unconfirmed report.'
  );
  scenario.check(fallback, issueUrl(101));
  fallback.commands.at(-1)!.args = [
    'issue',
    'create',
    '--title',
    'Images do not load',
    '--type=Bug',
    '--body',
    'Zephyr Testperson reports this.'
  ];
  expect(() => scenario.check(fallback, issueUrl(101))).toThrow('identity details');
});

test('native issue types are discovered, set, and read independently of labels', async () => {
  const github = new FakeGitHub();
  expect(JSON.parse((await call(github, 'repo', 'view', '--json=nameWithOwner')).output)).toEqual({
    nameWithOwner: REPOSITORY
  });
  const types = await call(github, 'api', 'repos/{owner}/{repo}/issue-types');
  expect(JSON.parse(types.output).map((type: { name: string }) => type.name)).toEqual([
    'Bug',
    'Feature',
    'Enhancement'
  ]);
  await call(
    github,
    'issue',
    'create',
    '--title=Images fail to load',
    '--body=Report',
    '--type=Bug'
  );
  expect(
    JSON.parse((await call(github, 'issue', 'view', '101', '--json=issueType,labels')).output)
  ).toMatchObject({ issueType: { name: 'Bug' }, labels: [] });
  await call(github, 'issue', 'edit', '101', '--type=Enhancement');
  checkIssueType(github.issues[0]!, 'Enhancement');
  await call(github, 'api', 'repos/{owner}/{repo}/issues/101', '-X', 'PATCH', '-f', 'type=Feature');
  checkIssueType(github.issues[0]!, 'Feature');
  expect(await call(github, 'issue', 'edit', '101', '--type=Missing')).toMatchObject({ ok: false });
  checkIssueType(github.issues[0]!, 'Feature');
});

test('classification graders reject title prefixes and label substitutes', () => {
  const issue = {
    number: 101,
    title: 'Images fail to load',
    body: 'Report',
    labels: [],
    state: 'OPEN' as const,
    url: issueUrl(101),
    issueType: { id: 1, name: 'Bug' }
  };
  checkIssueType(issue, 'Bug');
  for (const title of [
    'fix: images fail to load',
    'fix(client): images fail to load',
    '[Bug]: Images fail to load'
  ])
    expect(() => checkIssueType({ ...issue, title }, 'Bug')).toThrow('without prefixes');
  expect(() => checkIssueType({ ...issue, issueType: undefined, labels: ['bug'] }, 'Bug')).toThrow(
    'native GitHub issue type'
  );
  expect(() => checkIssueType({ ...issue, labels: ['bug'] }, 'Bug')).toThrow(
    'classification labels'
  );
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

test('search results expose only requested fields until the issue body is read', async () => {
  const github = new FakeGitHub([
    {
      number: 12,
      title: 'T',
      body: 'Detailed proposal',
      labels: [],
      state: 'OPEN',
      url: issueUrl(12)
    }
  ]);
  const search = await call(github, 'issue', 'list', '--json', 'number,title,url,state');
  expect(JSON.parse(search.output)).toEqual([
    { number: 12, title: 'T', url: issueUrl(12), state: 'OPEN' }
  ]);
  const detail = await call(github, 'issue', 'view', '12', '--json=body');
  expect(JSON.parse(detail.output)).toEqual({ body: 'Detailed proposal' });
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

test('closure records its reason and an explicit comment without changing the issue body', async () => {
  const github = new FakeGitHub([
    { number: 12, title: 'T', body: 'Keep', labels: [], state: 'OPEN', url: issueUrl(12) }
  ]);
  expect(
    await call(
      github,
      'issue',
      'close',
      '12',
      '--reason=not planned',
      '--comment=Requested explanation'
    )
  ).toMatchObject({ ok: true });
  expect(github.issues[0]).toMatchObject({
    state: 'CLOSED',
    stateReason: 'not planned',
    body: 'Keep',
    comments: ['Requested explanation']
  });
});

test('unsupported commands and fields fail without changing fixture state', async () => {
  const github = new FakeGitHub();
  for (const args of [
    ['search', 'issues', '--search=images'],
    ['issue', 'delete', '12', '--yes'],
    ['issue', 'edit', '12', '--remove-label=keep'],
    ['api', 'repos/{owner}/{repo}/issues/12', '-X', 'PATCH', '-f', 'unknown=value']
  ])
    expect(await call(github, ...args)).toMatchObject({ ok: false });
  expect(github.commands).toHaveLength(4);
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
