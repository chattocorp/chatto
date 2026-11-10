/** Worktree manifests are framework-owned temporary resources, independent of Docker. */
import { mkdtemp, mkdir, writeFile, readFile, rm, stat } from 'node:fs/promises';
import { execFile } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
import { expect, test } from 'vitest';
import { workingTreeManifest } from './git.ts';
import { createTaskContext } from './context.ts';

const exec = promisify(execFile);

test('selects local source and owns private manifest cleanup without creating application directories', async () => {
  const repository = await mkdtemp(join(tmpdir(), 'runling-git-manifest-'));
  let manifest = '';
  try {
    await exec('git', ['init', '-q'], { cwd: repository });
    await writeFile(join(repository, '.gitignore'), 'node_modules/\n.env\ntracked.txt\n');
    await writeFile(join(repository, 'tracked.txt'), 'original');
    await writeFile(join(repository, 'deleted.txt'), 'deleted');
    await exec('git', ['add', '.gitignore', 'deleted.txt'], { cwd: repository });
    await exec('git', ['add', '--force', 'tracked.txt'], { cwd: repository });
    await writeFile(join(repository, 'tracked.txt'), 'local change');
    await rm(join(repository, 'deleted.txt'));
    await writeFile(join(repository, 'untracked.ts'), 'new source');
    await writeFile(join(repository, 'line\nbreak.txt'), 'valid filename');
    await writeFile(join(repository, '.env'), 'private fixture');
    await mkdir(join(repository, 'node_modules'));
    await writeFile(join(repository, 'node_modules', 'host-binary'), 'host dependency');
    await mkdir(join(repository, 'submodule'));
    await writeFile(join(repository, 'submodule', 'nested.txt'), 'separate worktree');
    const { stdout: object } = await exec('git', ['hash-object', 'tracked.txt'], {
      cwd: repository
    });
    await exec(
      'git',
      ['update-index', '--add', '--cacheinfo', `160000,${object.trim()},submodule`],
      { cwd: repository }
    );
    {
      await using files = await workingTreeManifest(createTaskContext(), repository);
      manifest = files.path;
      expect((await readFile(manifest, 'utf8')).split('\0').filter(Boolean).sort()).toEqual([
        '.gitignore',
        'line\nbreak.txt',
        'tracked.txt',
        'untracked.ts'
      ]);
      if (process.platform !== 'win32') expect((await stat(manifest)).mode & 0o777).toBe(0o600);
      await expect(stat(join(repository, '.context'))).rejects.toMatchObject({ code: 'ENOENT' });
      expect(await readFile(join(repository, 'tracked.txt'), 'utf8')).toBe('local change');
    }
    await expect(stat(manifest)).rejects.toMatchObject({ code: 'ENOENT' });
  } finally {
    await rm(repository, { recursive: true, force: true });
  }
});

test('rejects an already cancelled selection before trying Git in a missing directory', async () => {
  const reason = new Error('Manifest cancelled');
  await expect(
    workingTreeManifest({ signal: AbortSignal.abort(reason) }, '/missing-repository')
  ).rejects.toBe(reason);
});
