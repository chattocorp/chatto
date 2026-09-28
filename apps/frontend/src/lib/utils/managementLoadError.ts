import { Code, ConnectError } from '@connectrpc/connect';
import { errorMessage } from './errorMessage';

export type ManagementLoadError = { kind: 'access-denied' } | { kind: 'failure'; message: string };

export function classifyManagementLoadError(error: unknown): ManagementLoadError {
  const connectError = ConnectError.from(error);
  if (connectError.code === Code.PermissionDenied || connectError.code === Code.NotFound) {
    return { kind: 'access-denied' };
  }
  return {
    kind: 'failure',
    message: errorMessage(error)
  };
}
