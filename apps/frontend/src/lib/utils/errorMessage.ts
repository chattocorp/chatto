import { Code, ConnectError } from '@connectrpc/connect';
import { isNetworkError, StaleResponseError } from '@chatto/client/api/connect';
import { m } from '$lib/i18n/messages';
import { toast } from '$lib/ui/toast';

/**
 * Codes whose server message tells the user what to change, such as a
 * validation failure or a limit. The UI shows that text as it is.
 */
const SERVER_MESSAGE_CODES = new Set([
  Code.InvalidArgument,
  Code.FailedPrecondition,
  Code.AlreadyExists,
  Code.OutOfRange,
  Code.ResourceExhausted
]);

/**
 * Convert a failed operation into a message for the user.
 *
 * - A `ConnectError` with an access, network, or conflict code gets a
 *   localized message. The server text for these codes is diagnostic, not
 *   guidance for the user.
 * - A `ConnectError` with a validation or limit code shows the server text,
 *   without the `[code]` prefix of `ConnectError.message`.
 * - Any other `ConnectError` shows `fallback`.
 * - A plain `Error` shows its own message. Local code and REST flows put
 *   user-facing text in it.
 * - Any other value shows `fallback`.
 */
export function errorMessage(error: unknown, fallback: string = m('common.error.generic')): string {
  if (error instanceof ConnectError) {
    if (isNetworkError(error)) return m('common.error.network');
    switch (error.code) {
      case Code.PermissionDenied:
        return m('common.error.permission_denied');
      case Code.NotFound:
        return m('common.error.not_found');
      case Code.Unauthenticated:
        return m('common.error.unauthenticated');
      case Code.Aborted:
        return m('common.error.conflict');
    }
    if (SERVER_MESSAGE_CODES.has(error.code) && error.rawMessage) return error.rawMessage;
    return fallback;
  }
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

/**
 * Convert a failed read into a message for the user. Use it for load and
 * retry states. It works like {@link errorMessage}, but always shows
 * `fallback` for validation and limit codes: the user did not submit input,
 * so the server text of a failed read is diagnostic and not guidance.
 */
export function loadErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ConnectError && SERVER_MESSAGE_CODES.has(error.code)) return fallback;
  return errorMessage(error, fallback);
}

/**
 * Show {@link errorMessage} as an error toast. A {@link StaleResponseError}
 * shows nothing: its response belongs to a context that no longer applies,
 * and a discarded mutation response can even belong to a successful change.
 */
export function toastError(error: unknown, fallback?: string): void {
  if (error instanceof StaleResponseError) return;
  toast.error(errorMessage(error, fallback));
}
