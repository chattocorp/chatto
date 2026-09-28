/**
 * Block selected tools once untrusted content has entered an agent's context.
 *
 * Runling agents install this through the `trust` option. Plain Pi users can install it
 * directly: `export default createTrustExtension({ untrusted: ['web_fetch'],
 * blockAfterUntrusted: ['bash'] }).extension;`
 */
import type { ExtensionAPI } from '@earendil-works/pi-coding-agent';

/** Tool names that add untrusted content, and tool names that are blocked afterwards. */
export interface TrustPolicy {
  /** Tools whose results can contain untrusted content, such as web pages. Any result
   * from these tools, including an error, marks the context untrusted. */
  untrusted: readonly string[];
  /** Tools that cannot run after the context is untrusted. The mark is never cleared,
   * because the content remains in the model history. */
  blockAfterUntrusted: readonly string[];
  /** Runs before the model receives the refusal, for example to show a host-written message.
   * A failure is reported through `log` with the tool name only; the block still applies. */
  onBlocked?: (toolName: string) => void | Promise<void>;
}

export interface TrustExtensionOptions {
  /** Start with an untrusted context, for example in a fork of an untrusted agent. */
  untrusted?: boolean;
  /** Receive fixed log messages. They contain tool names, never tool input or content. */
  log?: (level: 'info' | 'error', message: string) => void;
}

/** Create the extension and a view of its mark. One instance belongs to one agent session. */
export function createTrustExtension(policy: TrustPolicy, options: TrustExtensionOptions = {}) {
  const sources = new Set(policy.untrusted);
  const blocked = new Set(policy.blockAfterUntrusted);
  let untrusted = options.untrusted ?? false;

  const extension = (pi: ExtensionAPI) => {
    pi.on('tool_result', (event) => {
      if (sources.has(event.toolName)) untrusted = true;
    });
    pi.on('tool_call', async (event) => {
      if (!untrusted || !blocked.has(event.toolName)) return;
      options.log?.('info', `Blocked ${event.toolName}: untrusted content in context`);
      await Promise.resolve()
        .then(() => policy.onBlocked?.(event.toolName))
        .catch(() => {
          // Report a fixed category only; host errors can contain user data.
          options.log?.('error', `Blocked-tool callback failed for ${event.toolName}`);
        });
      return {
        block: true,
        reason: `${event.toolName} is blocked because this agent's context contains untrusted content.`
      };
    });
  };

  return {
    extension,
    /** True after a tool in `policy.untrusted` returned a result. */
    get untrusted() {
      return untrusted;
    }
  };
}
