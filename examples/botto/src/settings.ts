/** Load and validate TOML settings; secrets stay in the process environment. */
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { parse } from 'smol-toml';
import { parseServerUrl } from '@chatto/client/util/serverUrl';
import type { ThinkingLevel } from 'runling';
import {
  ADDRESSING_REASONS,
  DEFAULT_ADDRESSING_REASONS,
  type AddressingReason
} from '@chatto/client';

/** Queue limits and the quiet period shared by the inbox and its logs. */
export interface InboxSettings {
  debounceMs: number;
  passiveDebounceMs: number;
  maxPendingMessages: number;
  maxThreads: number;
}

/** Defaults also used when a TOML runtime section is omitted. */
export const DEFAULT_INBOX: InboxSettings = {
  debounceMs: 2000,
  passiveDebounceMs: 5000,
  maxPendingMessages: 100,
  maxThreads: 8
};

/** All settings resolved before any connection starts. Paths are absolute. */
export interface Settings extends InboxSettings {
  serverUrl: string;
  apiKey: string;
  /** Expose all tools from this server's MCP endpoint to Pi. */
  mcp: boolean;
  /** Local Means memory is independent of Chatto MCP. */
  memory: boolean;
  /** One Means collection, resolved relative to the TOML file. */
  memoryPath: string;
  meansCommand: string;
  /** Expose web search and page fetching from Exa's hosted MCP server. */
  web: boolean;
  /** Optional Exa API key; without it, Exa applies its keyless rate limits. */
  exaApiKey?: string;
  provider: string;
  modelId: string;
  /** Explicit reasoning effort; omitted preserves Pi defaults and saved session settings. */
  reasoningLevel?: ThinkingLevel;
  /** Independent description model; defaults to the main model with reasoning off. */
  titleModel: string;
  titleReasoningLevel: ThinkingLevel;
  /** Chat-model ID for the isolated OpenRouter participation classifier. */
  participationModel: string;
  name: string;
  /** Operator instructions appended to Botto's role and capability instructions for all agents. */
  systemPrompt: string;
  directory: string;
  /** Operator-owned skills beside the configuration file, independent of runtime storage. */
  skillsDirectory: string;
  allowedUserId?: string;
  /** Operator-selected Chatto user IDs. Chat and memory cannot grant this role. */
  adminUserIds: readonly string[];
  /** Show the bot account as online while Botto runs. */
  reportPresence: boolean;
  /** The reasons that address a message to Botto; see the client's `AddressingReason`. */
  respondTo: readonly AddressingReason[];
  turnTimeoutMs: number;
  contextMessages: number;
  /** Reaction on the first message of a new conversation; empty disables it. */
  acknowledgementEmoji: string;
}

const DEFAULT_PROMPT = `You are a helpful assistant. Be friendly, clear, and concise.`;

/** Expand parsed strings once. Environment values are data, never TOML or more templates.
 * A doubled dollar sign is literal, so $${NAME} produces ${NAME} unchanged. */
function expandEnvironment(value: string, env: NodeJS.ProcessEnv, label: string): string {
  return value.replace(
    /\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g,
    (match: string, name: string | undefined) => {
      if (match === '$$') return '$';
      if (name === undefined || !Object.hasOwn(env, name) || env[name] === undefined)
        throw new Error(`${label} references missing environment variable ${name}`);
      return env[name]!;
    }
  );
}

/** Parse without exposing TOML source text in syntax or validation errors. */
export function parseSettings(
  source: string,
  configPath: string,
  env: NodeJS.ProcessEnv = process.env
): Settings {
  let root: Record<string, unknown>;
  try {
    root = parse(source);
  } catch {
    throw new Error('Invalid TOML configuration. Check TOML syntax.');
  }
  const knownKeys = (table: Record<string, unknown>, keys: string[], label: string) => {
    if (Object.keys(table).some((key) => !keys.includes(key)))
      throw new Error(
        `Unsupported setting in ${label}. Check botto.example.toml; keep secrets in the environment.`
      );
  };
  knownKeys(
    root,
    ['chatto', 'agent', 'runtime', 'memory', 'web', 'titles', 'participation'],
    'the root table'
  );
  const section = (name: string, keys: string[]): Record<string, unknown> => {
    const value = root[name] ?? {};
    if (
      typeof value !== 'object' ||
      value === null ||
      Array.isArray(value) ||
      value instanceof Date
    )
      throw new Error(`${name} must be a TOML table`);
    const table = value as Record<string, unknown>;
    knownKeys(table, keys, name);
    return table;
  };
  const chatto = section('chatto', [
    'url',
    'api_key',
    'allowed_user_id',
    'admin_user_ids',
    'mcp',
    'report_presence',
    'respond_to'
  ]);
  const adminUserIds = chatto.admin_user_ids ?? [];
  if (
    !Array.isArray(adminUserIds) ||
    adminUserIds.some((id) => typeof id !== 'string' || !id.trim())
  )
    throw new Error('chatto.admin_user_ids must be an array of non-empty user IDs');
  if (chatto.mcp !== undefined && typeof chatto.mcp !== 'boolean')
    throw new Error('chatto.mcp must be a boolean');
  if (chatto.report_presence !== undefined && typeof chatto.report_presence !== 'boolean')
    throw new Error('chatto.report_presence must be a boolean');
  const respondTo = chatto.respond_to ?? DEFAULT_ADDRESSING_REASONS;
  if (
    !Array.isArray(respondTo) ||
    !respondTo.length ||
    respondTo.some((reason) => !ADDRESSING_REASONS.includes(reason as AddressingReason))
  )
    throw new Error(
      `chatto.respond_to must be a non-empty array of: ${ADDRESSING_REASONS.join(', ')}`
    );
  const agent = section('agent', ['name', 'model', 'system_prompt', 'reasoning_level']);
  const titles = section('titles', ['model', 'reasoning_level']);
  const participation = section('participation', ['model']);
  const memory = section('memory', ['enabled', 'command', 'path']);
  if (memory.enabled !== undefined && typeof memory.enabled !== 'boolean')
    throw new Error('memory.enabled must be a boolean');
  const web = section('web', ['enabled', 'exa_api_key']);
  if (web.enabled !== undefined && typeof web.enabled !== 'boolean')
    throw new Error('web.enabled must be a boolean');
  const runtime = section('runtime', [
    'data_directory',
    'acknowledgement_emoji',
    'debounce_ms',
    'passive_debounce_ms',
    'turn_timeout_ms',
    'context_messages',
    'max_pending_messages',
    'max_threads'
  ]);
  const string = (
    table: Record<string, unknown>,
    key: string,
    label: string,
    fallback?: string,
    allowEmpty = false
  ): string => {
    const value = table[key] ?? fallback;
    if (typeof value !== 'string') throw new Error(`${label} must be a non-empty string`);
    const expanded = expandEnvironment(value, env, label).trim();
    if (!expanded && !allowEmpty) throw new Error(`${label} must be a non-empty string`);
    return expanded;
  };
  const number = (key: string, fallback: number, min: number, max: number) => {
    const value = runtime[key] ?? fallback;
    if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < min || value > max)
      throw new Error(`runtime.${key} must be an integer between ${min} and ${max}`);
    return value;
  };
  const serverUrl = string(chatto, 'url', 'chatto.url');
  try {
    parseServerUrl(serverUrl);
  } catch {
    throw new Error('chatto.url must be an HTTP or HTTPS URL without credentials');
  }
  const model = string(agent, 'model', 'agent.model');
  const reasoningLevels: readonly ThinkingLevel[] = [
    'off',
    'minimal',
    'low',
    'medium',
    'high',
    'xhigh',
    'max'
  ];
  const reasoningLevel =
    agent.reasoning_level === undefined
      ? undefined
      : string(agent, 'reasoning_level', 'agent.reasoning_level');
  if (reasoningLevel !== undefined && !reasoningLevels.includes(reasoningLevel as ThinkingLevel))
    throw new Error('agent.reasoning_level must be off, minimal, low, medium, high, xhigh, or max');
  const separator = model.indexOf('/');
  if (separator < 1 || separator === model.length - 1)
    throw new Error('agent.model must use provider/model format');
  const titleModel = string(titles, 'model', 'titles.model', model);
  if (titleModel.indexOf('/') < 1 || titleModel.endsWith('/'))
    throw new Error('titles.model must use provider/model format');
  const titleReasoningLevel = string(titles, 'reasoning_level', 'titles.reasoning_level', 'off');
  if (!reasoningLevels.includes(titleReasoningLevel as ThinkingLevel))
    throw new Error(
      'titles.reasoning_level must be off, minimal, low, medium, high, xhigh, or max'
    );
  const apiKey = string(chatto, 'api_key', 'chatto.api_key');
  const participationModel = string(
    participation,
    'model',
    'participation.model',
    'z-ai/glm-5.3-flash'
  );
  if (!/^[^/\s]+\/[^\s]+$/.test(participationModel) || participationModel.startsWith('openrouter/'))
    throw new Error(
      'participation.model must be an OpenRouter model ID, without an openrouter/ prefix'
    );
  return {
    serverUrl,
    apiKey,
    adminUserIds: [
      ...new Set(
        adminUserIds.map((id: string) => {
          const expanded = expandEnvironment(id, env, 'chatto.admin_user_ids').trim();
          if (!expanded) throw new Error('chatto.admin_user_ids must contain non-empty user IDs');
          return expanded;
        })
      )
    ],
    mcp: chatto.mcp === true,
    reportPresence: chatto.report_presence !== false,
    respondTo: [...new Set(respondTo as AddressingReason[])],
    memory: memory.enabled === true,
    memoryPath: resolve(
      dirname(resolve(configPath)),
      string(memory, 'path', 'memory.path', 'memory')
    ),
    meansCommand: string(memory, 'command', 'memory.command', 'means'),
    web: web.enabled === true,
    exaApiKey:
      web.exa_api_key === undefined ? undefined : string(web, 'exa_api_key', 'web.exa_api_key'),
    provider: model.slice(0, separator),
    modelId: model.slice(separator + 1),
    reasoningLevel: reasoningLevel as ThinkingLevel | undefined,
    titleModel,
    participationModel,
    titleReasoningLevel: titleReasoningLevel as ThinkingLevel,
    name: string(agent, 'name', 'agent.name', 'Botto'),
    skillsDirectory: resolve(dirname(resolve(configPath)), '.agents/skills'),
    systemPrompt: string(agent, 'system_prompt', 'agent.system_prompt', DEFAULT_PROMPT),
    directory: resolve(
      dirname(resolve(configPath)),
      string(runtime, 'data_directory', 'runtime.data_directory', '.botto')
    ),
    allowedUserId:
      chatto.allowed_user_id === undefined
        ? undefined
        : string(chatto, 'allowed_user_id', 'chatto.allowed_user_id'),
    debounceMs: number('debounce_ms', DEFAULT_INBOX.debounceMs, 2000, 60_000),
    passiveDebounceMs: number(
      'passive_debounce_ms',
      DEFAULT_INBOX.passiveDebounceMs,
      2000,
      600_000
    ),
    maxPendingMessages: number('max_pending_messages', DEFAULT_INBOX.maxPendingMessages, 1, 10_000),
    maxThreads: number('max_threads', DEFAULT_INBOX.maxThreads, 1, 100),
    turnTimeoutMs: number('turn_timeout_ms', 900_000, 1000, 3_600_000),
    contextMessages: number('context_messages', 30, 1, 50),
    acknowledgementEmoji: string(
      runtime,
      'acknowledgement_emoji',
      'runtime.acknowledgement_emoji',
      'eyes',
      true
    )
  };
}

/** Read the chosen config file once. Relative data paths are based on that file. */
export function readSettings(
  configPath = 'botto.toml',
  env: NodeJS.ProcessEnv = process.env
): Settings {
  let source: string;
  try {
    source = readFileSync(configPath, 'utf8');
  } catch {
    throw new Error(
      'Cannot read Botto configuration. Copy botto.example.toml to botto.toml, or set BOTTO_CONFIG to its path.'
    );
  }
  return parseSettings(source, configPath, env);
}
