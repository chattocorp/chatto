// Formats files after a coding agent edits them. Claude Code and Codex run this
// script as a PostToolUse hook and pass the hook payload as JSON on stdin.
//
// Prettier formats every file type it supports, except for files that
// `.prettierignore` excludes. gofmt formats Go files, except generated files.
// The hook never blocks the agent: it reports formatter failures, such as a
// syntax error in a partial edit, on stderr and exits successfully.

import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const prettierBin = path.join(repoRoot, 'node_modules/prettier/bin/prettier.cjs');

// Go marks generated files with this header. See https://go.dev/s/generatedcode.
const generatedGoPattern = /^\/\/ Code generated .* DO NOT EDIT\.$/m;

// Codex `apply_patch` input names each changed file on one of these lines.
const patchFilePattern = /^\*\*\* (?:Add File|Update File|Move to): (.+)$/gm;

/**
 * Returns the absolute paths of the files that a hook payload edited.
 *
 * Claude Code edit tools report a single `tool_input.file_path`. Codex reports
 * edits as an `apply_patch` envelope in `tool_input.command`, with paths
 * relative to the session working directory.
 */
function editedFiles(payload) {
  const input = payload.tool_input ?? {};
  const cwd = payload.cwd ?? process.cwd();

  if (typeof input.file_path === 'string') {
    return [path.resolve(cwd, input.file_path)];
  }

  const patch = Array.isArray(input.command) ? input.command.join('\n') : input.command;
  if (typeof patch !== 'string') return [];

  return [...patch.matchAll(patchFilePattern)].map((match) => path.resolve(cwd, match[1].trim()));
}

function run(command, args) {
  try {
    execFileSync(command, args, { cwd: repoRoot, stdio: ['ignore', 'ignore', 'pipe'] });
  } catch (error) {
    process.stderr.write(error.stderr?.toString() || `${error.message}\n`);
  }
}

const payload = JSON.parse(readFileSync(0, 'utf8') || '{}');
const files = [...new Set(editedFiles(payload))].filter(
  (file) => existsSync(file) && !path.relative(repoRoot, file).startsWith('..')
);

const goFiles = files.filter(
  (file) => file.endsWith('.go') && !generatedGoPattern.test(readFileSync(file, 'utf8'))
);
const otherFiles = files.filter((file) => !file.endsWith('.go'));

if (goFiles.length > 0) {
  run('gofmt', ['-w', ...goFiles]);
}

// Prettier reads `.prettierignore` from its working directory, the repository
// root, and skips ignored paths even when they are named explicitly.
// `--ignore-unknown` skips file types that Prettier cannot parse.
if (otherFiles.length > 0) {
  run(process.execPath, [
    prettierBin,
    '--write',
    '--ignore-unknown',
    '--log-level',
    'warn',
    '--',
    ...otherFiles
  ]);
}
