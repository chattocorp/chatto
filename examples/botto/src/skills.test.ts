/** Exercise the real Pi catalog and restricted extension reader without a model request. */
import { mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { afterEach, expect, test } from 'vitest';
import {
  createAgentSession,
  DefaultResourceLoader,
  ModelRuntime,
  SessionManager,
  SettingsManager
} from '@earendil-works/pi-coding-agent';
import { skillReadExtension } from './skills.ts';

const directories: string[] = [];
afterEach(async () => {
  await Promise.all(
    directories.splice(0).map((path) => rm(path, { recursive: true, force: true }))
  );
});

async function setup() {
  await mkdir(resolve('../../.context'), { recursive: true });
  const directory = await mkdtemp(resolve('../../.context/botto-skills-'));
  directories.push(directory);
  const skills = join(directory, '.agents/skills');
  await mkdir(join(skills, 'sample'), { recursive: true });
  // Explicit discovery must not include Pi's global skills directory.
  await mkdir(join(directory, 'skills/ambient'), { recursive: true });
  await writeFile(
    join(directory, 'skills/ambient/SKILL.md'),
    '---\nname: ambient\ndescription: Must not be advertised.\n---\nAmbient instructions.'
  );
  await writeFile(
    join(skills, 'sample/SKILL.md'),
    '---\nname: sample\ndescription: Help with a synthetic sample task.\n---\nRead reference.txt for the sample.'
  );
  await writeFile(join(skills, 'sample/reference.txt'), 'first\nsecond\nthird');
  await writeFile(join(directory, 'private.txt'), 'PRIVATE OUTSIDE CONTENT');
  const settingsManager = SettingsManager.inMemory();
  const loader = new DefaultResourceLoader({
    cwd: directory,
    agentDir: directory,
    settingsManager,
    noExtensions: true,
    noSkills: true,
    additionalSkillPaths: [skills],
    noContextFiles: true,
    noPromptTemplates: true,
    noThemes: true,
    systemPromptOverride: () => 'You are a test assistant.',
    extensionFactories: [skillReadExtension(skills)]
  });
  await loader.reload();
  const runtime = await ModelRuntime.create({
    authPath: join(directory, 'auth.json'),
    modelsPath: null,
    refreshOnCreate: false,
    allowModelNetwork: false
  });
  const model = runtime.getModel('anthropic', 'claude-sonnet-4-6');
  if (!model) throw new Error('Built-in test model unavailable');
  const { session } = await createAgentSession({
    cwd: directory,
    agentDir: directory,
    settingsManager,
    resourceLoader: loader,
    modelRuntime: runtime,
    model,
    noTools: 'builtin',
    sessionManager: SessionManager.inMemory()
  });
  await session.bindExtensions({});
  const read = session.agent.state.tools.find((tool) => tool.name === 'read');
  if (!read) throw new Error('Restricted read tool missing');
  return { directory, skills, loader, session, read };
}

test('Pi advertises skills with only the restricted reader and loads supporting text', async () => {
  const { session, loader, read, skills } = await setup();
  try {
    expect(session.agent.state.tools.map((tool) => tool.name)).toEqual(['read']);
    expect(loader.getSkills().skills.map((skill) => skill.name)).toEqual(['sample']);
    expect(session.systemPrompt).toContain('Help with a synthetic sample task.');
    expect(session.systemPrompt).toContain('Use the read tool');
    expect(session.systemPrompt).not.toContain('Must not be advertised.');
    expect(
      JSON.stringify(await read.execute('skill', { path: join(skills, 'sample/SKILL.md') }))
    ).toContain('Read reference.txt');
    const result = await read.execute('reference', {
      path: 'sample/reference.txt',
      offset: 2,
      limit: 1
    });
    expect(result.content).toEqual([{ type: 'text', text: 'second\n[Continue with offset=3.]' }]);
    await rm(skills, { recursive: true });
    await loader.reload();
    expect(loader.getSkills().skills).toEqual([]);
  } finally {
    session.dispose();
  }
});

test('a line at the output limit advances pagination instead of returning an empty page', async () => {
  const { skills, session, read } = await setup();
  try {
    const line = 'x'.repeat(20_000);
    await writeFile(join(skills, 'boundary.txt'), `${line}\nlast`);
    expect((await read.execute('first', { path: 'boundary.txt' })).content).toEqual([
      { type: 'text', text: `${line}\n[Continue with offset=2.]` }
    ]);
    expect((await read.execute('last', { path: 'boundary.txt', offset: 2 })).content).toEqual([
      { type: 'text', text: 'last' }
    ]);
  } finally {
    session.dispose();
  }
});

test('reader rejects outside files, traversal, symlink escapes, directories, binary and oversized files', async () => {
  const { directory, skills, session, read } = await setup();
  try {
    await symlink(join(directory, 'private.txt'), join(skills, 'escape.txt'));
    await symlink(directory, join(skills, 'escape-dir'));
    await symlink(join(skills, 'sample/reference.txt'), join(skills, 'safe-link.txt'));
    await writeFile(join(skills, 'binary'), Buffer.from([0xff, 0, 0xfe]));
    await writeFile(join(skills, 'large'), 'x'.repeat(256 * 1024 + 1));
    await mkdir(`${skills}-other`);
    await writeFile(join(`${skills}-other`, 'secret'), 'PRIVATE OUTSIDE CONTENT');
    for (const path of [
      join(directory, 'private.txt'),
      '../../private.txt',
      'escape.txt',
      'escape-dir/private.txt',
      'sample',
      'binary',
      'large',
      join(`${skills}-other`, 'secret')
    ]) {
      await expect(read.execute('denied', { path })).rejects.toThrow('Cannot read this skill file');
    }
    expect(JSON.stringify(await read.execute('allowed', { path: 'safe-link.txt' }))).toContain(
      'first'
    );
    const controller = new AbortController();
    controller.abort(new Error('Cancelled'));
    await expect(
      read.execute('cancel', { path: 'sample/reference.txt' }, controller.signal)
    ).rejects.toThrow('Cancelled');
  } finally {
    session.dispose();
  }
});
