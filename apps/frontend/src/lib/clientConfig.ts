const CLIENT_CONFIG_PATH = '/client-config.json';

export type AuthlingClientConfiguration = {
  issuer: string;
  clientId: string;
};

export type ClientConfiguration = {
  version: 1;
  authling: AuthlingClientConfiguration | null;
};

type RawClientConfiguration = {
  version?: unknown;
  authling?: {
    issuer?: unknown;
    client_id?: unknown;
  } | null;
};

let configurationPromise: Promise<ClientConfiguration> | null = null;

/** Load the trusted configuration published by the current frontend origin. */
export function getClientConfiguration(): Promise<ClientConfiguration> {
  configurationPromise ??= loadClientConfiguration();
  return configurationPromise;
}

async function loadClientConfiguration(): Promise<ClientConfiguration> {
  const response = await fetch(CLIENT_CONFIG_PATH, {
    credentials: 'same-origin',
    cache: 'no-store',
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(10000)
  });
  if (!response.ok) throw new Error('The client configuration is unavailable.');

  const raw = (await response.json()) as RawClientConfiguration;
  if (raw.version !== 1) throw new Error('The client configuration version is not supported.');
  if (raw.authling === undefined || raw.authling === null) {
    return { version: 1, authling: null };
  }
  if (typeof raw.authling.issuer !== 'string' || typeof raw.authling.client_id !== 'string') {
    throw new Error('The Authling client configuration is incomplete.');
  }

  const issuer = validateIssuer(raw.authling.issuer);
  const clientId = raw.authling.client_id.trim();
  if (clientId.length === 0 || clientId.length > 2048) {
    throw new Error('The Authling client ID is invalid.');
  }
  return { version: 1, authling: { issuer, clientId } };
}

function validateIssuer(raw: string): string {
  const issuer = new URL(raw);
  if (
    (issuer.protocol !== 'https:' &&
      !(issuer.protocol === 'http:' && isLoopbackHost(issuer.hostname))) ||
    issuer.username ||
    issuer.password ||
    (issuer.pathname !== '/' && issuer.pathname !== '') ||
    issuer.search ||
    issuer.hash
  ) {
    throw new Error('The Authling issuer URL is invalid.');
  }
  return issuer.origin;
}

function isLoopbackHost(hostname: string): boolean {
  const normalized = hostname.toLowerCase().replace(/\.$/, '');
  return (
    normalized === 'localhost' ||
    normalized === '127.0.0.1' ||
    normalized === '::1' ||
    normalized === '[::1]'
  );
}
