import { Code, ConnectError } from '@connectrpc/connect';

type MessageBearingError = {
  message?: unknown;
};

const AUTHENTICATION_REQUIRED_MESSAGE = 'authentication required';

export function isAuthenticationRequiredError(error: unknown): boolean {
  if (error instanceof ConnectError && error.code === Code.Unauthenticated) {
    return true;
  }

  if (!error || typeof error !== 'object') return false;

  const candidate = error as MessageBearingError;
  return (
    typeof candidate.message === 'string' &&
    candidate.message.toLowerCase().includes(AUTHENTICATION_REQUIRED_MESSAGE)
  );
}
