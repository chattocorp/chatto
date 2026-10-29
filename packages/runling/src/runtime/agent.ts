import type { TaskContext } from './context.ts';
import { usePrivateTempDirectory } from './private-temp.ts';
import { requireDirectory } from './directory.ts';
import { executionServices } from './execution.ts';
import { stripVTControlCharacters } from 'node:util';
import { dirname } from 'node:path';
import { rm } from 'node:fs/promises';
import {
  createAgentSession as createPiAgentSession,
  convertToLlm,
  DefaultResourceLoader,
  defineTool,
  getAgentDir,
  type AgentSessionEvent,
  type ExtensionAPI,
  type InlineExtension,
  type ToolDefinition,
  ModelRuntime,
  SessionManager,
  SettingsManager
} from '@earendil-works/pi-coding-agent';
import { Type, type Static } from 'typebox';
import webFetchExtension from '../../extensions/web-fetch.ts';
import {
  codemodeExtension,
  DEFAULT_SCRIPT_MAX_CALLS,
  DEFAULT_SCRIPT_TIMEOUT_MS,
  validateScriptMaxCalls,
  validateScriptTimeout
} from './codemode.ts';
import { createTrustExtension, type TrustPolicy } from '../../extensions/trust.ts';
import { bindRunlingContext, emitRunlingEvent } from './events.ts';
import { randomId } from './id.ts';
import { log, logMarkdown, withLogSource } from './log.ts';
import { parseModelReference } from './model.ts';
import { displayPath, displayText } from './paths.ts';
import { RUNLING_SYSTEM_PROMPT, formatAgentInstructions } from './system-prompt.ts';
import { containsMalformedToolCall, toSingleLine } from './text.ts';
import {
  accumulateTokenUsage,
  emptyTokenUsage,
  formatTokenUsage,
  type TokenUsage
} from './usage.ts';

const reportSchema = Type.Object({
  outcome: Type.Union([Type.Literal('completed'), Type.Literal('blocked'), Type.Literal('failed')]),
  summary: Type.String({
    description: 'A concise, single-line summary of the outcome',
    minLength: 1,
    maxLength: 500,
    pattern: '^[^\\r\\n]+$'
  }),
  details: Type.Optional(
    Type.String({
      description: 'Optional detailed Markdown supporting the summary',
      minLength: 1,
      maxLength: 20_000
    })
  )
});

const AGENT_COLORS = [
  '#f59f00',
  '#40c057',
  '#15aabf',
  '#4c6ef5',
  '#ae3ec9',
  '#e64980',
  '#f76707',
  '#12b886'
] as const;
const MIN_AGENT_RETRIES = 5;
const TOOL_ACTION_COLORS: Record<string, string> = {
  read: '#40c057',
  bash: '#ae3ec9',
  edit: '#f76707',
  write: '#15aabf'
};
let nextAgentColor = 0;

function takeAgentColor(): string {
  const color = AGENT_COLORS[nextAgentColor % AGENT_COLORS.length]!;
  nextAgentColor++;
  return color;
}

export type AgentReport = Static<typeof reportSchema>;
/** A report enriched with the token usage of the agent interaction. */
export type AgentResult = AgentReport & {
  usage: TokenUsage;
  /** Host-detected failure, distinct from a model's own blocked/failed assessment. */
  failureReason?: 'provider_error' | 'missing_outcome';
};
export type CompletedAgentReport = AgentResult & { outcome: 'completed' };
/** A named or anonymous pi extension instantiated for one agent session. */
export type AgentExtension = InlineExtension;
/** A tool that an agent extension registers. */
export type AgentTool = ToolDefinition;
/** Define an agent tool and keep the type of its parameters for `execute`. */
export const defineAgentTool = defineTool;
/** The API available while configuring an agent-local extension. */
export type AgentExtensionAPI = ExtensionAPI;

/** Preserve contextual typing when declaring a reusable agent extension. */
export function defineAgentExtension(extension: AgentExtension): AgentExtension {
  return extension;
}

export interface AgentResourceOptions {
  /** Directory containing global pi configuration and resources. */
  agentDir?: string;
  extensions?: boolean;
  /** Default discovery unless false. An array loads only these paths, relative to cwd. */
  skills?: boolean | string[];
  promptTemplates?: boolean;
  themes?: boolean;
  contextFiles?: boolean;
}

/**
 * Reasoning effort for models that support extended thinking. "xhigh" and
 * "max" are only supported by selected model families.
 */
export type ThinkingLevel = 'off' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max';

/** A tool operation category for journal events. */
type ToolOperation = 'read' | 'search' | 'edit' | 'command' | 'other';
/** A safe tool failure category. Unknown means the tool did not provide a recognized cause. */
type ToolFailure = 'not_found' | 'permission_denied' | 'invalid_arguments' | 'timeout' | 'unknown';

/** Categorize tool errors for logs without their raw output. */
export function toolFailureCategory(result: unknown): ToolFailure {
  const value = result as
    { content?: Array<{ type?: string; text?: string }>; details?: { code?: string } } | undefined;
  const message = [
    typeof value?.details?.code === 'string' ? value.details.code : '',
    ...(Array.isArray(value?.content)
      ? value.content
          .slice(0, 8)
          .map((part) =>
            part?.type === 'text' && typeof part.text === 'string' ? part.text.slice(0, 2000) : ''
          )
      : [])
  ].join(' ');
  if (/\bENOENT\b|no such file or directory/i.test(message)) return 'not_found';
  if (/\bEACCES\b|\bEPERM\b|permission denied/i.test(message)) return 'permission_denied';
  if (/\bETIMEDOUT\b|timed out/i.test(message)) return 'timeout';
  if (
    /invalid arguments|validation failed|missing required (?:argument|parameter|property)/i.test(
      message
    )
  )
    return 'invalid_arguments';
  return 'unknown';
}

/** Convert provider diagnostics to fixed messages without exposing their raw content. */
function providerFailureSummary(error: string): string {
  if (/refresh_token_expired|refresh_token_reused|session has expired/i.test(error)) {
    return 'The model provider login has expired. Sign in again on the agent host before retrying.';
  }
  return 'Agent provider could not finish the response';
}

/** Refuse selected tools once untrusted content has entered an agent's context.
 * See `extensions/trust.ts`. */
export type AgentTrustPolicy = TrustPolicy;

export interface RunAgentOptions {
  model: string;
  /** Static operational role shown in server logs. Never use user data or model output. */
  label?: string;
  /** Reasoning effort for the model. Defaults to pi's own settings default. */
  thinkingLevel?: ThinkingLevel;
  instructions?: readonly string[];
  /** Replace Pi's default coding-agent prompt. Instructions and Runling outcome rules still apply. */
  systemPrompt?: string;
  cwd: string;
  /** Text agents finish naturally and return their final text; report agents must call
   * report_outcome (default). */
  output?: 'text' | 'report';
  /** In text mode, permit silence without an explicit terminating tool result.
   * Defaults to false. Provider errors still fail the turn. */
  allowEmptyText?: boolean;
  /** Tool names to expose, or `extensions` for all extension tools without built-in tools.
   * `report_outcome` is added in report mode. */
  tools?: readonly string[] | 'extensions';
  /** Execute tool calls sequentially when extensions share a lock or other serial resource. */
  toolExecution?: 'parallel' | 'sequential';
  /** Include model text, reasoning, tool arguments, and provider diagnostics in logs and
   * progress events. Default: true. False does not redact private session transcripts. */
  logContent?: boolean;
  resources?: AgentResourceOptions;
  /** Extensions instantiated only for this agent session. */
  extensions?: readonly AgentExtension[];
  /** Observe the raw pi event stream for this agent session. */
  onEvent?: (event: AgentSessionEvent) => void;
  /** Abort an active model turn. */
  signal?: AbortSignal;
  /** Block selected tools after untrusted content enters this agent's context. */
  trust?: AgentTrustPolicy;
  /**
   * Add Pi's `codemode` tool: the model writes a JavaScript script that calls the tools in `tools`,
   * for example several reads in parallel, and only the script's output reaches the model.
   * Scripts run in a QuickJS sandbox without Node APIs, files, network, or timers. Every call from
   * a script passes the same `tool_call` and `tool_result` hooks as a model call, so trust
   * policies and extension gates still apply. With `mode: 'only'`, the other tools are hidden
   * from the model, which reaches them through scripts. Scripts cannot run classifier or image
   * models, and they cannot call `report_outcome`. A script stops after `timeoutMs` (default ten
   * minutes) and can make at most `maxCalls` tool calls (default 100). See FDR-008.
   */
  codemode?: boolean | { mode?: 'on' | 'only'; timeoutMs?: number; maxCalls?: number };
  /** Keep the agent's conversation in this JSONL file. When the file exists, the agent
   * continues that conversation, for example after a cancellation or restart. The file holds
   * the complete model context, so keep it private. A fork does not use it. It cannot be
   * combined with `trust`, because the untrusted mark is not stored in the file. */
  sessionFile?: string;
}

export type AgentOptions = Omit<RunAgentOptions, 'signal'>;

export interface AgentRunOptions {
  /** Receive completed assistant text (not reasoning or tool output). `hasToolCalls`
   * identifies text before tool execution; it is not a guarantee that the turn has ended. */
  onText?: (text: string, message: { hasToolCalls: boolean }) => void;
  /** Abort this model turn without disposing the agent. */
  signal?: AbortSignal;
}

export interface RunlingAgent extends AsyncDisposable {
  /** Human-friendly ID used to prefix this agent's log lines. */
  readonly id: string;
  /** Run one turn and require it to complete successfully. */
  run(
    ctx: TaskContext<any, any>,
    prompt: string,
    options?: AgentRunOptions
  ): Promise<CompletedAgentReport>;
  /** Run one turn and return any reported outcome. */
  runOutcome(
    ctx: TaskContext<any, any>,
    prompt: string,
    options?: AgentRunOptions
  ): Promise<AgentResult>;
  /** Deliver plain text during an interaction. Resolves true when inserted into its
   * conversation, false if idle or the interaction ends before delivery. */
  steer(text: string): Promise<boolean>;
  /** Create an independent in-memory agent with a copy of this conversation. */
  fork(): Promise<RunlingAgent>;
  /** Cancel active work, await extension shutdown, and release the session. Idempotent. */
  dispose(): Promise<void>;
}

export class AgentOutcomeError extends Error {
  override readonly name = 'AgentOutcomeError';

  constructor(readonly report: AgentResult) {
    super(report.summary);
  }
}

export function describeTool(name: string, args: Record<string, unknown>, directory?: string) {
  switch (name) {
    case 'read':
      return `Reading ${displayPath(String(args.path), directory)}`;
    case 'edit':
      return `Editing ${displayPath(String(args.path), directory)}`;
    case 'write':
      return `Writing ${displayPath(String(args.path), directory)}`;
    case 'bash':
      return `Running ${displayText(String(args.command).replaceAll('\n', ' '), directory)}`;
    default:
      return `Using ${name}`;
  }
}

function highlightToolAction(tool: string, description: string): string {
  const separator = description.indexOf(' ');
  const color = TOOL_ACTION_COLORS[tool] ?? 'white';
  if (separator === -1) return log.highlight(description, color);

  return `${log.highlight(description.slice(0, separator), color)}${description.slice(separator)}`;
}

export async function runAgent(
  ctx: TaskContext<any, any>,
  prompt: string,
  options: RunAgentOptions
): Promise<AgentResult> {
  const { signal, ...createOptions } = options;
  ctx.signal.throwIfAborted();
  signal?.throwIfAborted();
  const instance = await createAgentSession(createOptions);

  try {
    return await instance.runOutcome(ctx, prompt, { signal });
  } finally {
    await instance.dispose();
  }
}

/** The phase of a text part, when the provider marks it. OpenAI's Responses API separates
 * preliminary `commentary` from the `final_answer`; Pi keeps the phase in `textSignature`. */
function textPhase(part: { textSignature?: string }): string | undefined {
  if (!part.textSignature?.startsWith('{')) return undefined;
  try {
    const phase: unknown = JSON.parse(part.textSignature).phase;
    return typeof phase === 'string' ? phase : undefined;
  } catch {
    return undefined;
  }
}

type ContentPart = { type: string; text?: string; textSignature?: string };
const textParts = (content: readonly ContentPart[]) =>
  content.filter((part): part is ContentPart & { text: string } => part.type === 'text');

/** A message's answer text. When the provider marks a final answer, preliminary commentary is
 * left out, so the answer is not delivered twice in different words. */
export function answerText(content: readonly ContentPart[]): string {
  const parts = textParts(content);
  const final = parts.filter((part) => textPhase(part) === 'final_answer');
  return (final.length ? final : parts).map((part) => part.text).join('\n');
}

/** Preliminary commentary that `answerText` leaves out, for debug logs. */
function commentaryText(content: readonly ContentPart[]): string {
  const parts = textParts(content);
  if (!parts.some((part) => textPhase(part) === 'final_answer')) return '';
  return parts
    .filter((part) => textPhase(part) !== 'final_answer')
    .map((part) => part.text)
    .join('\n');
}

/** Create a live agent session for a custom conversation loop. The caller owns disposal. */
export async function createAgentSession(options: AgentOptions): Promise<RunlingAgent> {
  await usePrivateTempDirectory();
  return createRunlingAgent(options);
}

async function createRunlingAgent(
  options: AgentOptions,
  history: Parameters<typeof convertToLlm>[0] = [],
  inheritedUntrusted = false
): Promise<RunlingAgent> {
  const inheritedMessages = convertToLlm(structuredClone(history));
  const agentId = randomId();
  const color = takeAgentColor();
  const diagnostic = (message?: string) =>
    options.logContent === false ? 'Details omitted' : toSingleLine(message ?? 'Unknown error');
  const progress = (text: string) => {
    const line = stripVTControlCharacters(text).replace(/\s+/g, ' ').trim();
    if (line) {
      emitRunlingEvent({ type: 'agent.progress', agentId, text: line.slice(-500) });
    }
  };

  const prefixLog = (message: string) => `${log.colorize(`[${agentId}]`)} ${message}`;
  const writeAgentLog = (
    level: 'debug' | 'error' | 'info' | 'success',
    message: string,
    showProgress = true,
    markdown = false
  ) => {
    if (showProgress && level !== 'debug') progress(message);
    if (level !== 'debug' || log.level === 'debug') {
      emitRunlingEvent({
        type: 'agent.action',
        agentId,
        action: stripVTControlCharacters(message)
      });
    }
    withLogSource({ type: 'agent', id: agentId }, () =>
      log.withColor(color, () =>
        markdown ? logMarkdown(prefixLog('').trimEnd(), message) : log[level](prefixLog(message))
      )
    );
  };
  const agentLog = {
    debug: (message: string) => writeAgentLog('debug', message),
    error: (message: string) => writeAgentLog('error', message),
    info: (message: string) => writeAgentLog('info', message),
    markdown: (message: string) => writeAgentLog('info', message, true, true),
    success: (message: string) => writeAgentLog('success', message)
  };
  // Preserve reply events for journals and observers even when the host owns their display.
  const logReply = (message: string, markdown = false) => {
    if (options.logContent === false) return;
    const write = () => writeAgentLog('info', message, true, markdown);
    if (executionServices()?.printAgentReplies === false) log.withDestination('silent', write);
    else write();
  };
  let activeReport: AgentReport | undefined;
  let reports: AgentReport[] = [];

  const reportOutcome = defineTool({
    name: 'report_outcome',
    label: 'Report outcome',
    description: 'Report the final outcome of the task and terminate the run.',
    promptSnippet: 'Report the task outcome as structured data',
    promptGuidelines: [
      'Always call report_outcome as your final action.',
      'Do not finish with a plain-text assistant response.',
      'Every report must contain the complete current result. If new messages arrive after a report, incorporate them and report the full updated result again. Never refer to a preceding report or replace findings with a meta-summary.'
    ],
    parameters: reportSchema,
    // Only the model reports the outcome; codemode scripts cannot call it.
    exposure: 'model-only',
    async execute(_toolCallId, params) {
      activeReport = params;
      reports.push({ ...params });
      return {
        content: [{ type: 'text' as const, text: 'Outcome recorded.' }],
        details: params,
        terminate: true
      };
    }
  });

  const textOutput = options.output === 'text';
  const cwd = requireDirectory(options.cwd);
  const agentDir = options.resources?.agentDir ?? getAgentDir();
  const modelRuntime = await ModelRuntime.create();
  const modelReference = parseModelReference(options.model);
  let model = modelRuntime.getModel(modelReference.provider, modelReference.id);
  // Refresh only the missing model's provider. Known models need no network lookup.
  if (model === undefined && process.env.PI_OFFLINE === undefined) {
    await modelRuntime.refresh({
      providers: [modelReference.provider],
      allowNetwork: true,
      force: true,
      signal: AbortSignal.timeout(5_000)
    });
    model = modelRuntime.getModel(modelReference.provider, modelReference.id);
  }

  if (model === undefined) {
    throw new Error(`Model ${options.model} is unavailable`);
  }

  const additionalInstructions = formatAgentInstructions(options.instructions ?? []);

  // Untrusted content stays in the model history, so forks inherit the mark.
  const trust = options.trust
    ? createTrustExtension(options.trust, {
        untrusted: inheritedUntrusted,
        log: (level, message) => writeAgentLog(level, message)
      })
    : undefined;

  const resources = options.resources;
  // Files with the complete output of long scripts, which the agent deletes when it ends.
  const codemodeFiles = new Set<string>();
  let disposed = false;
  const extensionsEnabled = resources?.extensions !== false;
  const settingsManager = SettingsManager.create(cwd, agentDir);
  const retrySettings = settingsManager.getRetrySettings();
  settingsManager.applyOverrides({
    retry: {
      ...retrySettings,
      maxRetries: Math.max(retrySettings.maxRetries, MIN_AGENT_RETRIES)
    }
  });
  const resourceLoader = new DefaultResourceLoader({
    cwd,
    agentDir,
    settingsManager,
    extensionFactories: [
      ...(trust ? [{ name: 'runling-trust', factory: trust.extension }] : []),
      ...(extensionsEnabled ? [{ name: 'runling-web-fetch', factory: webFetchExtension }] : []),
      ...(options.codemode
        ? [
            {
              name: 'runling-codemode',
              factory: codemodeExtension({
                mode: options.codemode === true ? 'on' : (options.codemode.mode ?? 'on'),
                timeoutMs: validateScriptTimeout(
                  (options.codemode === true ? undefined : options.codemode.timeoutMs) ??
                    DEFAULT_SCRIPT_TIMEOUT_MS
                ),
                maxCalls: validateScriptMaxCalls(
                  (options.codemode === true ? undefined : options.codemode.maxCalls) ??
                    DEFAULT_SCRIPT_MAX_CALLS
                ),
                keepFullOutput:
                  options.tools !== 'extensions' && (options.tools ?? ['read']).includes('read'),
                keptFiles: codemodeFiles,
                ended: () => disposed
              })
            }
          ]
        : []),
      ...(options.extensions ?? [])
    ],
    noExtensions: !extensionsEnabled,
    noSkills: resources?.skills === false || Array.isArray(resources?.skills),
    ...(Array.isArray(resources?.skills) ? { additionalSkillPaths: resources.skills } : {}),
    noPromptTemplates: resources?.promptTemplates === false,
    noThemes: resources?.themes === false,
    noContextFiles: resources?.contextFiles === false,
    ...(options.systemPrompt === undefined
      ? {}
      : { systemPromptOverride: () => options.systemPrompt }),
    appendSystemPromptOverride: (base) => [
      ...base,
      ...(textOutput ? [] : [RUNLING_SYSTEM_PROMPT]),
      ...(additionalInstructions === undefined ? [] : [additionalInstructions])
    ]
  });
  await resourceLoader.reload();

  if (options.sessionFile && options.trust)
    throw new Error('An agent session file cannot be combined with a trust policy');
  // A session file continues its conversation when it exists and starts a new one otherwise.
  const sessionManager = options.sessionFile
    ? SessionManager.open(options.sessionFile, dirname(options.sessionFile), cwd)
    : SessionManager.inMemory(cwd);
  // Persist the inherited model context so Pi can compact and restore it.
  // Pi converts existing compaction/branch summaries into normal messages.
  for (const message of inheritedMessages) {
    sessionManager.appendMessage(message);
  }
  const { session } = await createPiAgentSession({
    cwd,
    model,
    thinkingLevel: options.thinkingLevel,
    modelRuntime,
    resourceLoader,
    sessionManager,
    settingsManager,
    customTools: textOutput ? [] : [reportOutcome],
    ...(options.tools === 'extensions'
      ? { noTools: 'builtin' as const }
      : {
          tools: [
            ...new Set([
              ...(options.tools ?? [
                'read',
                'bash',
                'edit',
                'write',
                ...(extensionsEnabled ? ['web_fetch'] : [])
              ]),
              ...(options.codemode ? ['codemode'] : []),
              ...(textOutput ? [] : ['report_outcome'])
            ])
          ]
        })
  });

  if (options.toolExecution) session.agent.toolExecution = options.toolExecution;
  // Native extensions start their connections on session_start. Pair even a
  // partially failed startup with shutdown before releasing the Pi session.
  let shutdown: Promise<void> | undefined;
  const shutdownExtensions = () =>
    (shutdown ??= session.extensionRunner.emit({ type: 'session_shutdown', reason: 'quit' }));
  try {
    await session.bindExtensions({});
  } catch (error) {
    try {
      await shutdownExtensions();
    } finally {
      session.dispose();
    }
    throw error;
  }

  let running = false;
  let acceptingSteering = false;
  let aborting: Promise<void> | undefined;
  const abortModel = () => (aborting ??= session.abort());
  let interactionSignal: AbortSignal | undefined;
  const steering = new Map<object, (delivered: boolean) => void>();
  const finishSteering = () => {
    acceptingSteering = false;
    if (steering.size) session.agent.clearSteeringQueue();
    for (const resolve of steering.values()) resolve(false);
    steering.clear();
  };
  let disposal: Promise<void> | undefined;
  const dispose = (): Promise<void> => {
    if (disposal) return disposal;
    disposed = true;
    finishSteering();
    return (disposal = (async () => {
      try {
        // Stop MCP transports as well as model work; tool calls can be waiting
        // on a connection that only the extension can close.
        const results = await Promise.allSettled([
          running ? abortModel() : (aborting ?? Promise.resolve()),
          shutdownExtensions()
        ]);
        for (const result of results) if (result.status === 'rejected') throw result.reason;
      } finally {
        session.dispose();
        await Promise.all(
          [...codemodeFiles].map((file) => rm(file, { force: true }).catch(() => {}))
        );
      }
    })());
  };

  const runOutcome: RunlingAgent['runOutcome'] = async (
    ctx,
    prompt,
    { signal: externalSignal, onText } = {}
  ) => {
    const signal = externalSignal ? AbortSignal.any([ctx.signal, externalSignal]) : ctx.signal;
    if (disposed) {
      throw new Error(`Agent ${agentId} has been disposed`);
    }
    if (running) {
      throw new Error(`Agent ${agentId} is already running`);
    }

    signal?.throwIfAborted();
    running = true;
    aborting = undefined;
    interactionSignal = signal;
    acceptingSteering = true;
    activeReport = undefined;
    reports = [];
    let finalText: string | undefined;
    let finalTextError: string | undefined;
    // Match Pi's batch termination rule, but only successful results count as completion.
    let toolsTerminated: boolean | undefined;
    const usage = emptyTokenUsage();
    let turn = 0;
    // Pi starts its agent loop again after a provider retry. It is still the same interaction.
    let started = false;
    let lastTextUpdate = -Infinity;
    let preparingReport = false;
    const toolStartedAt = new Map<string, number>();
    const activity = (tool: string, phase: 'started' | 'succeeded' | 'failed') => {
      const operation: ToolOperation =
        tool === 'read'
          ? 'read'
          : ['grep', 'find', 'ls'].includes(tool)
            ? 'search'
            : ['edit', 'write', 'apply_patch'].includes(tool)
              ? 'edit'
              : tool === 'bash'
                ? 'command'
                : 'other';
      const toolName = /^[a-zA-Z][a-zA-Z0-9_]{0,63}$/.test(tool) ? tool : undefined;
      emitRunlingEvent({ type: 'agent.tool', agentId, operation, phase, toolName });
    };

    const unsubscribe = session.subscribe(
      bindRunlingContext((event) => {
        if (event.type === 'agent_start') acceptingSteering = !disposed && !signal.aborted;
        if (event.type === 'agent_end') acceptingSteering = false;
        if (event.type === 'message_start' && steering.has(event.message)) {
          const delivered = steering.get(event.message)!;
          steering.delete(event.message);
          activeReport = undefined;
          finalText = undefined;
          preparingReport = false;
          delivered(true);
        }
        options.onEvent?.(event);

        if (event.type === 'message_update') {
          const update = event.assistantMessageEvent;
          if (
            !preparingReport &&
            (update.type === 'toolcall_start' ||
              update.type === 'toolcall_delta' ||
              update.type === 'toolcall_end')
          ) {
            const part = update.partial.content[update.contentIndex];
            if (part?.type === 'toolCall' && part.name === 'report_outcome') {
              progress('Preparing result…');
              preparingReport = true;
            }
          }
          if (update.type === 'text_start') lastTextUpdate = -Infinity;
          if (
            options.logContent !== false &&
            (update.type === 'text_delta' || update.type === 'text_end')
          ) {
            const now = performance.now();
            if (update.type === 'text_end' || now - lastTextUpdate >= 100) {
              const part = update.partial.content[update.contentIndex];
              if (part?.type === 'text') progress(part.text);
              lastTextUpdate = now;
            }
          }
        }

        if (event.type === 'agent_start' && !started) {
          started = true;
          emitRunlingEvent({
            type: 'agent.started',
            agentId,
            model: `${model.provider}/${model.id}`,
            ...(options.label ? { label: options.label } : {}),
            color
          });
          agentLog.info(`Agent started (model: ${model.provider}/${model.id})`);
        }

        if (event.type === 'turn_start') {
          toolsTerminated = undefined;
          turn++;
          agentLog.debug(`Turn ${turn} started`);
        }

        if (event.type === 'turn_end') {
          agentLog.debug(
            `Turn ${turn} finished (${formatCount(event.toolResults.length, 'tool result')})`
          );
        }

        if (event.type === 'tool_execution_start' && event.toolName !== 'report_outcome') {
          toolStartedAt.set(event.toolCallId, performance.now());
          // A codemode script reports as one call; the calls in it appear only in the log.
          if (!event.parentToolCallId) activity(event.toolName, 'started');
          const action =
            options.logContent === false
              ? `Using ${event.toolName}`
              : describeTool(event.toolName, event.args, cwd);
          agentLog.info(highlightToolAction(event.toolName, action));
        }

        if (event.type === 'tool_execution_end' && !event.parentToolCallId) {
          toolsTerminated =
            (toolsTerminated ?? true) && !event.isError && event.result?.terminate === true;
        }

        if (event.type === 'tool_execution_end' && event.toolName !== 'report_outcome') {
          const startedAt = toolStartedAt.get(event.toolCallId);
          toolStartedAt.delete(event.toolCallId);
          if (!event.parentToolCallId)
            activity(event.toolName, event.isError ? 'failed' : 'succeeded');
          const duration =
            startedAt === undefined ? '' : ` in ${formatDuration(performance.now() - startedAt)}`;

          if (event.isError) {
            agentLog.error(
              `${event.toolName} failed${duration} (${toolFailureCategory(event.result)})`
            );
          } else {
            agentLog.debug(`${event.toolName} finished${duration}`);
          }
        }

        if (event.type === 'compaction_start') {
          agentLog.info(`Compacting context (${event.reason})`);
        }

        if (event.type === 'compaction_end') {
          if (event.aborted) {
            agentLog.info('Context compaction aborted');
          } else if (event.result === undefined) {
            agentLog.error(`Context compaction failed: ${diagnostic(event.errorMessage)}`);
          } else {
            const estimatedAfter = event.result.estimatedTokensAfter;
            agentLog.success(
              estimatedAfter === undefined
                ? `Context compacted (${event.result.tokensBefore.toLocaleString()} tokens before compaction)`
                : `Context compacted from ${event.result.tokensBefore.toLocaleString()} to about ${estimatedAfter.toLocaleString()} tokens`
            );
          }
        }

        if (event.type === 'summarization_retry_scheduled') {
          agentLog.info(
            `Retrying context summary in ${formatDelay(event.delayMs)} ` +
              `(attempt ${event.attempt}/${event.maxAttempts}): ${diagnostic(event.errorMessage)}`
          );
        }

        if (event.type === 'summarization_retry_attempt_start') {
          agentLog.debug(`Retrying ${formatSummarySource(event.source)} summary`);
        }

        if (event.type === 'summarization_retry_finished') {
          agentLog.debug('Context summary retry finished');
        }

        if (event.type === 'agent_end') {
          agentLog.debug(`Agent finished (${formatCount(event.messages.length, 'message')})`);
        }

        if (event.type === 'auto_retry_start') {
          agentLog.info(
            `Retrying agent in ${formatDelay(event.delayMs)} ` +
              `(attempt ${event.attempt}/${event.maxAttempts}): ${diagnostic(event.errorMessage)}`
          );
        }

        if (event.type === 'auto_retry_end') {
          if (event.success) {
            finalTextError = undefined;
            agentLog.success(`Agent recovered after ${formatAttempts(event.attempt)}`);
          } else {
            finalTextError = 'Agent provider failed after retries';
            agentLog.error(
              `Agent retry failed after ${formatAttempts(event.attempt)}: ${diagnostic(event.finalError)}`
            );
          }
        }

        if (event.type === 'message_end' && event.message.role === 'assistant') {
          finalTextError =
            event.message.stopReason === 'error' || event.message.stopReason === 'aborted'
              ? event.message.errorMessage || 'Agent response failed'
              : undefined;
          finalText = answerText(event.message.content);
          const commentary = commentaryText(event.message.content);
          if (commentary && options.logContent !== false)
            agentLog.debug(`Commentary: ${commentary}`);

          // Reasoning summaries are often the only account of what the agent is doing.
          for (const part of event.message.content)
            if (
              options.logContent !== false &&
              part.type === 'thinking' &&
              !part.redacted &&
              part.thinking.trim()
            )
              agentLog.markdown(`**Thinking**\n\n${part.thinking.trim()}`);
          if (finalText.trim()) logReply(finalText, true);

          accumulateTokenUsage(usage, event.message.usage);
          ctx.recordUsage(event.message.usage);
          emitRunlingEvent({ type: 'agent.usage', agentId, usage: { ...usage } });
          agentLog.debug(`Tokens: ${formatTokenUsage(usage)}`);
          // A message that failed or was aborted can hold partial text; a retry follows.
          if (finalText.trim() && !finalTextError)
            onText?.(finalText, {
              hasToolCalls: event.message.content.some((part) => part.type === 'toolCall')
            });
        }
      })
    );

    const abort = () => {
      void abortModel().catch(() => agentLog.error('Agent cancellation failed'));
    };

    signal?.addEventListener('abort', abort, { once: true });
    let result: AgentResult | undefined;

    try {
      result = await withLogSource({ type: 'agent', id: agentId }, () =>
        log.withColor(color, async (): Promise<AgentResult> => {
          signal?.throwIfAborted();
          await session.prompt(prompt);
          signal?.throwIfAborted();

          // A transport failure is not a report-format error. Do not start a new
          // model request after the provider has exhausted its retry budget.
          if (finalTextError) {
            return {
              outcome: 'failed',
              failureReason: 'provider_error',
              summary: providerFailureSummary(finalTextError),
              usage
            };
          }
          if (textOutput) {
            // The final text is the result. onText already delivered it during the turn.
            if (!finalText?.trim() && !options.allowEmptyText && !toolsTerminated)
              return {
                outcome: 'failed',
                summary: 'Agent finished without a text response',
                usage
              };
            return { outcome: 'completed', summary: finalText ?? '', usage };
          }

          if (activeReport === undefined) {
            agentLog.info(
              finalText !== undefined && containsMalformedToolCall(finalText)
                ? 'Retrying malformed outcome report'
                : 'Retrying missing outcome report'
            );
            finalText = undefined;
            acceptingSteering = true;
            await session.prompt(
              'Finish the original task by calling report_outcome with the truthful outcome. Use native tool calling; do not respond with plain text.'
            );
            signal?.throwIfAborted();
            if (finalTextError) {
              return {
                outcome: 'failed',
                failureReason: 'provider_error',
                summary: providerFailureSummary(finalTextError),
                usage
              };
            }
          }

          if (activeReport !== undefined) {
            // Steering can reopen an interaction after a valid report. Preserve
            // that evidence even if the next report only refers back to it.
            let details = activeReport.details;
            if (reports.length > 1) {
              details = reports
                .map((report, index) => {
                  const label =
                    index === reports.length - 1
                      ? 'latest; supersedes earlier conclusions'
                      : 'earlier findings';
                  const body = report.details
                    ? `${report.summary}\n\n${report.details}`
                    : report.summary;

                  return `Report ${index + 1} (${label}):\n${body}`;
                })
                .join('\n\n');
            }

            logReply(details?.trim() || activeReport.summary);
            return { ...activeReport, ...(details !== undefined ? { details } : {}), usage };
          }

          if (options.logContent !== false && finalText !== undefined && finalText.trim() !== '') {
            agentLog.debug(`Discarding unreported final text: ${toSingleLine(finalText)}`);
          }

          return {
            outcome: 'failed',
            summary: 'Agent finished without a valid outcome report',
            failureReason: 'missing_outcome',
            usage
          };
        })
      );
      return result;
    } finally {
      signal?.removeEventListener('abort', abort);
      finishSteering();
      interactionSignal = undefined;
      unsubscribe();
      running = false;
      writeAgentLog('info', `Token usage: ${formatTokenUsage(usage)}`, false);
      emitRunlingEvent({
        type: 'agent.finished',
        agentId,
        outcome: result?.outcome ?? 'failed',
        usage: { ...usage }
      });
    }
  };

  return {
    id: agentId,

    async run(ctx, prompt, runOptions) {
      const report = await runOutcome(ctx, prompt, runOptions);
      if (report.outcome !== 'completed') {
        throw new AgentOutcomeError(report);
      }
      return report as CompletedAgentReport;
    },

    runOutcome,

    steer(text) {
      if (disposed || !running || !acceptingSteering || interactionSignal?.aborted)
        return Promise.resolve(false);
      // Use Pi's plain-message API: steering must not expand slash commands or templates.
      const message = {
        role: 'user' as const,
        content: [{ type: 'text' as const, text }],
        timestamp: Date.now()
      };
      return new Promise<boolean>((resolve, reject) => {
        steering.set(message, resolve);
        try {
          session.agent.steer(message);
        } catch (error) {
          steering.delete(message);
          reject(error);
        }
      });
    },

    async fork() {
      if (disposed) {
        throw new Error(`Agent ${agentId} has been disposed`);
      }
      if (running) {
        throw new Error(`Agent ${agentId} is already running`);
      }

      // A fork has its own history; it must not write into the parent's session file.
      return createRunlingAgent(
        { ...options, sessionFile: undefined },
        session.agent.state.messages,
        trust?.untrusted ?? false
      );
    },

    dispose,

    async [Symbol.asyncDispose]() {
      await dispose();
    }
  };
}

function formatDelay(delayMs: number): string {
  return delayMs % 1000 === 0 ? `${delayMs / 1000}s` : `${delayMs}ms`;
}

function formatAttempts(attempts: number): string {
  return `${attempts} retry ${attempts === 1 ? 'attempt' : 'attempts'}`;
}

function formatCount(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`;
}

function formatDuration(durationMs: number): string {
  return durationMs < 1000 ? `${Math.round(durationMs)}ms` : `${(durationMs / 1000).toFixed(1)}s`;
}

function formatSummarySource(source: 'branchSummary' | 'compaction'): string {
  return source === 'branchSummary' ? 'branch' : 'context';
}
