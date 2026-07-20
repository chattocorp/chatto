import { Code, ConnectError } from '@connectrpc/connect';

export type ManagementLoadError = { kind: 'access-denied' } | { kind: 'failure'; message: string };

export function classifyManagementLoadError(error: unknown): ManagementLoadError {
  const connectError = ConnectError.from(error);
  if (connectError.code === Code.PermissionDenied || connectError.code === Code.NotFound) {
    return { kind: 'access-denied' };
  }
  return {
    kind: 'failure',
    message: error instanceof Error ? error.message : String(error)
  };
}
