/** Redaction and protected paths for implementation work. */
import { stripVTControlCharacters } from 'node:util';

/** Prefix of an owner question forwarded to the worker; the worker answers it with answerOwner. */
export const ownerQuestionPrefix = '[ChattoBot owner question: ';

/** Instruction, skill, and environment files that implementation must not change. */
export const protectedPath = (path: string) =>
  /(^|\/)(AGENTS\.md|CLAUDE\.md|SKILL\.md|\.env(?:\..*)?)$/i.test(path) ||
  /(^|\/)(?:\.agents|\.codex|\.claude)?\/?skills\//i.test(path);

/** Remove common private values before retaining worker or check text. */
function redactImplementationText(output: string, worktree: string): string {
  let text = stripVTControlCharacters(output);
  // An empty worktree would match between every character.
  if (worktree) text = text.split(worktree).join('<worktree>');
  for (const [key, value] of Object.entries(process.env)) {
    if (value && value.length >= 4 && /KEY|TOKEN|PASSWORD|SECRET|CREDENTIAL/i.test(key))
      text = text.split(value).join('[redacted]');
  }
  text = text
    .replace(/https?:\/\/[^\s)]+/g, '[url]')
    .replace(/file:\/\/[^\s)]+/g, '[host path]')
    .replace(/\/(?:Users|home)\/[^\s)]+/g, '[host path]')
    .replace(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/gi, '[email]')
    .replace(/\b(?:\d{1,3}\.){3}\d{1,3}\b/g, '[ip]')
    .replace(
      /(?:Bearer\s+|(?:api[_-]?key|token|password|secret)\s*[:=]\s*)[^\s,;]+/gi,
      '[credential]'
    )
    .replace(/[\x00-\x08\x0b-\x1f\x7f]/g, '');
  return text;
}

/** Private repair context, bounded and scrubbed of host credentials and common identifiers.
 * Never send this text to operational logs or copy it verbatim to chat/PR bodies. */
export function validationDiagnostic(output: string, worktree: string): string {
  const text = redactImplementationText(output, worktree);
  return text.trim().slice(-8000) || 'No diagnostic output was available.';
}

/** Bounded worker explanation for the owner; no raw output or host paths enter chat. */
export function workerStopReason(summary: string, worktree: string): string {
  return (
    redactImplementationText(summary, worktree).replace(/\s+/g, ' ').trim().slice(0, 800) ||
    'The worker did not provide a reason.'
  );
}
