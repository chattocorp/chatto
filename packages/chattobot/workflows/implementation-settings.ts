/** Host-owned implementation settings and repository policy. Chat messages cannot select these values. */
import { resolve } from 'node:path';
import type { ThinkingLevel } from 'runling/agents';
import { ConfigurationError, setting, thinkingSetting } from '../settings.ts';

/** Publication is opt-in. These host-owned values cannot be selected by a chat message. */
export interface ImplementationSettings {
  directory: string;
  repository: string;
  baseBranch?: string;
  model?: string;
  /** Reasoning effort of the implementation worker. Defaults to `medium`. */
  thinkingLevel?: ThinkingLevel;
  artifactsDirectory?: string;
}

/** Read opt-in publication settings from the bot process environment.
 * Throws a ConfigurationError for incomplete or invalid settings. */
export function implementationSettings(): ImplementationSettings | undefined {
  const repository = setting('CHATTO_IMPLEMENTATION_REPOSITORY');
  if (!repository) return;
  const directory = setting('CHATTO_SOURCE_DIRECTORY');
  if (!directory)
    throw new ConfigurationError(
      'CHATTO_IMPLEMENTATION_REPOSITORY requires CHATTO_SOURCE_DIRECTORY'
    );
  return normalizeImplementationSettings({
    directory: resolve(directory),
    repository,
    baseBranch: setting('CHATTO_SOURCE_REF'),
    model: setting('CHATTO_IMPLEMENTATION_MODEL') ?? 'openai-codex/gpt-5.6-sol',
    thinkingLevel: thinkingSetting('CHATTO_IMPLEMENTATION_THINKING', 'medium')
  });
}

/** Validate the repository and resolve the base branch name. The base defaults to `main`
 * and accepts the remote-tracking notation commonly copied from git status. */
export function normalizeImplementationSettings(
  settings: ImplementationSettings
): ImplementationSettings & { baseBranch: string } {
  const baseBranch = (settings.baseBranch ?? 'main').replace(/^(?:refs\/remotes\/)?origin\//, '');
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(settings.repository))
    throw new ConfigurationError('CHATTO_IMPLEMENTATION_REPOSITORY must be owner/repo');
  if (!/^[A-Za-z0-9][A-Za-z0-9._/-]*$/.test(baseBranch))
    throw new ConfigurationError('CHATTO_SOURCE_REF must name a branch on origin');
  return { ...settings, baseBranch };
}

/** Repository commands must not inherit the bot's routing policy or model credentials. */
export function implementationCommandEnvKeys(env: NodeJS.ProcessEnv): string[] {
  return Object.keys(env).filter(
    (key) =>
      /^(?:CHATTO_|AUTHLING_|OPENROUTER_|OPENAI_|ANTHROPIC_)/.test(key) ||
      /^(?:GH_TOKEN|GITHUB_TOKEN)$/.test(key)
  );
}

/** Recognize only credential-free GitHub origin URLs for the configured repository. */
export function matchesRepository(remote: string, repository: string): boolean {
  return [
    `https://github.com/${repository}`,
    `https://github.com/${repository}.git`,
    `git@github.com:${repository}`,
    `git@github.com:${repository}.git`,
    `ssh://git@github.com/${repository}`,
    `ssh://git@github.com/${repository}.git`
  ].some((value) => value.toLowerCase() === remote.trim().toLowerCase());
}
