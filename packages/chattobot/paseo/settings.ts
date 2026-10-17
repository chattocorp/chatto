/** Optional connection to one operator-selected Paseo daemon and repository. */
import { posix, win32 } from 'node:path';
import { ConfigurationError, setting } from '../settings.ts';

export interface PaseoSettings {
  url: string;
  password?: string;
  /** Absolute repository path on the Paseo host, not necessarily the bot host. */
  repositoryPath: string;
  /** Host-selected coding model, permission mode, and thinking level. Omitted options use provider defaults. */
  agent?: { model: string; modeId?: string; thinkingOptionId?: string };
}

/** Validate configuration without contacting Paseo or inspecting the remote filesystem. */
export function paseoSettings(): PaseoSettings | undefined {
  const url = setting('CHATTO_PASEO_URL');
  const repositoryPath = setting('CHATTO_PASEO_REPOSITORY_PATH');
  const password = setting('CHATTO_PASEO_PASSWORD');
  const model = setting('CHATTO_PASEO_AGENT_MODEL');
  const modeId = setting('CHATTO_PASEO_AGENT_MODE');
  const thinkingOptionId = setting('CHATTO_PASEO_AGENT_THINKING');
  if (!url && !repositoryPath && !password && !model && !modeId && !thinkingOptionId)
    return undefined;
  if (!url || !repositoryPath)
    throw new ConfigurationError('Set CHATTO_PASEO_URL and CHATTO_PASEO_REPOSITORY_PATH');
  try {
    const parsed = new URL(url);
    if (
      !['ws:', 'wss:'].includes(parsed.protocol) ||
      parsed.username ||
      parsed.password ||
      parsed.search ||
      parsed.hash ||
      !parsed.pathname.endsWith('/ws')
    )
      throw new Error('Invalid endpoint');
  } catch {
    throw new ConfigurationError(
      'CHATTO_PASEO_URL must be a ws:// or wss:// endpoint ending in /ws, without credentials, query, or fragment'
    );
  }
  if (!posix.isAbsolute(repositoryPath) && !win32.isAbsolute(repositoryPath))
    throw new ConfigurationError('CHATTO_PASEO_REPOSITORY_PATH must be absolute on the Paseo host');
  if ((model && !/^[^\s/]+\/\S+$/.test(model)) || ((modeId || thinkingOptionId) && !model))
    throw new ConfigurationError(
      'Set CHATTO_PASEO_AGENT_MODEL to provider/model before setting CHATTO_PASEO_AGENT_MODE or CHATTO_PASEO_AGENT_THINKING'
    );
  return {
    url,
    repositoryPath,
    ...(password ? { password } : {}),
    ...(model
      ? {
          agent: {
            model,
            ...(modeId ? { modeId } : {}),
            ...(thinkingOptionId ? { thinkingOptionId } : {})
          }
        }
      : {})
  };
}
