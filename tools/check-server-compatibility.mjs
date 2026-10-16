/** Require PR labels for changes to the client's server version thresholds. */
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Use the client's parser without executing either revision's TypeScript source.
const require = createRequire(new URL('../packages/chatto-client/package.json', import.meta.url));
const ts = require('typescript');
const sourcePath = 'packages/chatto-client/src/server/compatibility.ts';
const requiredLabels = {
  MINIMUM_SUPPORTED_SERVER_VERSION: 'server-compatibility:full-support',
  MINIMUM_CONNECTABLE_SERVER_VERSION: 'server-compatibility:connection-floor'
};

/** Read one exported string constant per threshold. Ambiguous or invalid source fails closed. */
export function readThresholds(source) {
  const file = ts.createSourceFile(sourcePath, source, ts.ScriptTarget.Latest, true);
  if (file.parseDiagnostics.length) throw new Error(`Cannot parse ${sourcePath}`);
  const values = {};
  for (const name of Object.keys(requiredLabels)) {
    const declarations = [];
    for (const statement of file.statements) {
      if (!ts.isVariableStatement(statement)) continue;
      for (const declaration of statement.declarationList.declarations) {
        if (ts.isIdentifier(declaration.name) && declaration.name.text === name) {
          declarations.push({ statement, declaration });
        }
      }
    }
    if (declarations.length !== 1) {
      throw new Error(`${name}: expected one declaration, found ${declarations.length}`);
    }
    const { statement, declaration } = declarations[0];
    if (
      !(statement.declarationList.flags & ts.NodeFlags.Const) ||
      !statement.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword) ||
      !declaration.initializer ||
      !ts.isStringLiteral(declaration.initializer)
    ) {
      throw new Error(`${name}: expected an exported const with a string literal value`);
    }
    values[name] = declaration.initializer.text;
  }
  return values;
}

/** Return one diagnostic per changed threshold that has no exact matching label. */
export function checkThresholdChanges(baseSource, headSource, labels) {
  const base = readThresholds(baseSource);
  const head = readThresholds(headSource);
  const names = new Set(labels);
  return Object.entries(requiredLabels)
    .filter(([name, label]) => base[name] !== head[name] && !names.has(label))
    .map(([name, label]) => `${name}: ${base[name]} -> ${head[name]}; add PR label "${label}".`);
}

/** Check immutable PR revisions from the GitHub event. Git history must contain their merge base. */
export function checkPullRequest(event, cwd = process.cwd()) {
  const pr = event.pull_request;
  const base = pr?.base?.sha;
  const head = pr?.head?.sha;
  if (![base, head].every((sha) => typeof sha === 'string' && /^[a-f0-9]{40}$/.test(sha))) {
    throw new Error('Expected full PR base and head commit SHAs in the GitHub event');
  }
  if (!Array.isArray(pr.labels) || pr.labels.some((label) => typeof label?.name !== 'string')) {
    throw new Error('Expected PR labels in the GitHub event');
  }
  const git = (...args) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim();
  const mergeBase = git('merge-base', base, head);
  const read = (sha) => git('show', `${sha}:${sourcePath}`);
  return checkThresholdChanges(
    read(mergeBase),
    read(head),
    pr.labels.map((label) => label.name)
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const event = JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, 'utf8'));
    const errors = checkPullRequest(event);
    if (errors.length) {
      for (const error of errors) console.error(error);
      process.exitCode = 1;
    } else {
      console.log('Server compatibility threshold changes have the required PR labels.');
    }
  } catch (error) {
    console.error(`Server compatibility check failed: ${error.message}`);
    process.exitCode = 1;
  }
}
