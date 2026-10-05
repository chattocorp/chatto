/** Runling's codemode: Pi's codemode tool with limits that the agent's owner controls. See FDR-008. */
import { rm } from 'node:fs/promises';
import { createCodemodeExtension, type ExtensionAPI } from '@earendil-works/pi-coding-agent';

/** Longest script run unless the agent sets `codemode.timeoutMs`. */
export const DEFAULT_SCRIPT_TIMEOUT_MS = 10 * 60_000;
/** Most tool calls in one script unless the agent sets `codemode.maxCalls`. */
export const DEFAULT_SCRIPT_MAX_CALLS = 100;

const OPTIONS_PREFIX = '// @options:';

/**
 * Give a script a deadline of at most `maxMs`. Pi reads options from an optional first line, and a
 * script without a `timeout_ms` option never times out. A shorter deadline in the script stays.
 * A first line with invalid options stays unchanged: Pi then refuses the script.
 */
export function withScriptDeadline(code: string, maxMs: number): string {
  const newline = code.indexOf('\n');
  const first = (newline === -1 ? code : code.slice(0, newline)).replace(/\r$/, '').trimStart();
  if (!first.startsWith(OPTIONS_PREFIX))
    return `${OPTIONS_PREFIX} ${JSON.stringify({ timeout_ms: maxMs })}\n${code}`;
  let options: unknown;
  try {
    options = JSON.parse(first.slice(OPTIONS_PREFIX.length));
  } catch {
    return code;
  }
  if (typeof options !== 'object' || options === null || Array.isArray(options)) return code;
  const requested = (options as { timeout_ms?: unknown }).timeout_ms;
  const timeout =
    typeof requested === 'number' && requested > 0 ? Math.min(requested, maxMs) : maxMs;
  const rest = newline === -1 ? '' : code.slice(newline);
  return `${OPTIONS_PREFIX} ${JSON.stringify({ ...options, timeout_ms: timeout })}${rest}`;
}

export interface CodemodeSettings {
  mode: 'on' | 'only';
  timeoutMs: number;
  /** Most tool calls in one script. Later calls fail in the script. */
  maxCalls: number;
  /** Keep the file with the complete output of a long script until the agent ends, for agents
   * that can read it. Otherwise Runling deletes the file at once. */
  keepFullOutput: boolean;
  /** Receives the paths of kept files, which the agent deletes when it ends. */
  keptFiles: Set<string>;
  /** True after the agent ended; a script that finishes later keeps no file. */
  ended: () => boolean;
}

/** Check a script deadline: a positive whole number of milliseconds that Pi accepts. */
export function validateScriptTimeout(timeoutMs: number): number {
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 2_147_483_647)
    throw new RangeError('codemode.timeoutMs must be whole milliseconds between 1 and 2147483647');
  return timeoutMs;
}

/** Check a call limit: a positive whole number. */
export function validateScriptMaxCalls(maxCalls: number): number {
  if (!Number.isSafeInteger(maxCalls) || maxCalls <= 0)
    throw new RangeError('codemode.maxCalls must be a positive whole number');
  return maxCalls;
}

/** Pi writes the complete output of a long script to a temporary file and names it in the result. */
const FULL_OUTPUT_NOTE = /\n\n\[Full output: [^\n]*\]$/;

export function codemodeExtension(settings: CodemodeSettings) {
  // Scripts must not spend money on models that the agent's owner did not choose.
  const codemode = createCodemodeExtension({ mode: settings.mode, models: false });
  return (pi: ExtensionAPI) => {
    codemode(pi);
    // Tool calls of each running script, by the script's call ID. Scripts cannot start scripts.
    const calls = new Map<string, number>();
    pi.on('tool_call', (event) => {
      if (event.parentToolCallId) {
        const count = (calls.get(event.parentToolCallId) ?? 0) + 1;
        calls.set(event.parentToolCallId, count);
        if (count > settings.maxCalls)
          return {
            block: true,
            reason: `A script can make at most ${settings.maxCalls} tool calls. Make fewer calls, or call the tools directly.`
          };
        return;
      }
      if (event.toolName !== 'codemode') return;
      // Calls from a script carry the script's call ID. Without a unique one, they would look like
      // direct calls and pass the call limit and gates that refuse script calls.
      if (!event.toolCallId || calls.has(event.toolCallId))
        return {
          block: true,
          reason: 'codemode needs a unique tool call ID from the model provider.'
        };
      calls.set(event.toolCallId, 0);
      const input = event.input as { code?: unknown };
      if (typeof input.code === 'string')
        input.code = withScriptDeadline(input.code, settings.timeoutMs);
    });
    // Every call of a response ends before its turn ends. A refused or aborted script leaves no
    // entry behind, so a provider that reuses IDs across turns can run later scripts.
    pi.on('turn_end', () => calls.clear());
    pi.on('tool_result', async (event) => {
      if (event.toolName !== 'codemode' || event.parentToolCallId) return;
      calls.delete(event.toolCallId);
      const path = (event.details as { fullOutputPath?: unknown } | undefined)?.fullOutputPath;
      if (typeof path !== 'string') return;
      if (settings.keepFullOutput && !settings.ended()) {
        settings.keptFiles.add(path);
        return;
      }
      // Without a read tool, the file serves no purpose and can hold sensitive tool output.
      await rm(path, { force: true });
      return {
        content: event.content.map((item) =>
          item.type === 'text' ? { ...item, text: item.text.replace(FULL_OUTPUT_NOTE, '') } : item
        ),
        details: { ...(event.details as object), fullOutputPath: undefined }
      };
    });
  };
}
