/** Check tracked files changed since a base branch, without rewriting files. */
import { execFileSync } from 'node:child_process';
import { lstat, readFile } from 'node:fs/promises';
import path from 'node:path';
import * as prettier from 'prettier';

const git = (...args) => execFileSync('git', args, { encoding: 'utf8' }).trimEnd();

try {
  const args = process.argv.slice(2);
  // pnpm forwards the separator used in `pnpm format:changed -- <base>`.
  if (args[0] === '--') args.shift();
  const [base = 'origin/main', ...extra] = args;
  if (extra.length) throw new Error('Usage: node tools/check-format.mjs [base-ref]');
  const root = git('rev-parse', '--show-toplevel');
  process.chdir(root);
  // Resolve the argument before using it in other Git commands, including option-like input.
  const baseCommit = git('rev-parse', '--verify', '--end-of-options', `${base}^{commit}`);
  const ancestor = git('merge-base', baseCommit, 'HEAD');
  // Compare with the working tree so local staged and unstaged edits are checked too.
  // NUL separation preserves spaces, newlines, and glob characters in filenames.
  const changed = execFileSync(
    'git',
    ['diff', '--name-only', '-z', '--diff-filter=ACMRT', ancestor, '--'],
    { encoding: 'utf8' }
  )
    .split('\0')
    .filter(Boolean);
  let checked = 0;
  let failed = 0;
  for (const file of changed) {
    // Never follow a changed symlink outside the repository.
    if (!(await lstat(file)).isFile()) continue;
    const info = await prettier.getFileInfo(file, {
      ignorePath: path.join(root, '.prettierignore')
    });
    if (info.ignored || !info.inferredParser) continue;
    checked++;
    const config = await prettier.resolveConfig(file, { editorconfig: true });
    if (!(await prettier.check(await readFile(file, 'utf8'), { ...config, filepath: file }))) {
      console.error(`Formatting required: ${JSON.stringify(file)}`);
      failed++;
    }
  }
  console.log(`Formatting: ${checked} changed files checked, ${failed} need formatting.`);
  if (failed) {
    console.error('Run mise x -- pnpm format -- <file> for each reported file.');
    process.exitCode = 1;
  }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
