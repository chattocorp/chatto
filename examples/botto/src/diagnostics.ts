/** Report host-owned diagnostics without exposing external error messages or URLs. */
export class StartupError extends Error {}

const hints: Record<string, string> = {
  EACCES: 'Permission denied.',
  EPERM: 'Operation not permitted.',
  ENOSPC: 'No disk space available.',
  ENOTDIR: 'A configured directory is not a directory.',
  ENOTFOUND: 'Host name lookup failed.',
  ECONNREFUSED: 'Connection refused.',
  ETIMEDOUT: 'Connection timed out.',
  CERT_HAS_EXPIRED: 'The TLS certificate has expired.',
  UNABLE_TO_VERIFY_LEAF_SIGNATURE: 'The TLS certificate could not be verified.',
  '4': 'Request deadline exceeded.',
  '7': 'Access denied.',
  '14': 'Service unavailable.',
  '16': 'Authentication rejected.'
};

/** Add a fixed hint from a known error code, including wrapped transport errors. */
export function errorHint(error: unknown): string {
  for (let depth = 0; depth < 5 && typeof error === 'object' && error !== null; depth++) {
    if ('code' in error && (typeof error.code === 'string' || typeof error.code === 'number')) {
      const key = String(error.code);
      if (Object.hasOwn(hints, key)) return ` ${hints[key]}`;
    }
    error = 'cause' in error ? error.cause : undefined;
  }
  return '';
}

/** Wrap a startup operation with an actionable, host-owned failure description. */
export async function startupStep<T>(description: string, work: () => Promise<T>): Promise<T> {
  try {
    return await work();
  } catch (error) {
    if (error instanceof StartupError) throw error;
    throw new StartupError(`${description}${errorHint(error)}`);
  }
}
