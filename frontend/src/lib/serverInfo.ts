export interface PublicAuthProviderInfo {
  id: string;
  type: string;
  label: string;
  loginUrl: string;
}

export interface PublicServerInfo {
  name: string;
  authProviders: PublicAuthProviderInfo[];
  directRegistrationEnabled: boolean;
  welcomeMessage: string | null;
  description: string | null;
  iconUrl: string | null;
  bannerUrl: string | null;
}

interface RawPublicServerInfo {
  name?: unknown;
  authProviders?: unknown;
  registrationOpen?: unknown;
  directRegistrationEnabled?: unknown;
  welcomeMessage?: unknown;
  description?: unknown;
  iconUrl?: unknown;
  logoUrl?: unknown;
  bannerUrl?: unknown;
}

export async function fetchPublicServerInfo(baseUrl = ''): Promise<PublicServerInfo> {
  const response = await fetch(serverInfoUrl(baseUrl));
  if (!response.ok) {
    throw new Error(`GET /api/server failed with ${response.status}`);
  }

  const raw = (await response.json()) as RawPublicServerInfo;
  const registration =
    typeof raw.directRegistrationEnabled === 'boolean'
      ? raw.directRegistrationEnabled
      : typeof raw.registrationOpen === 'boolean'
        ? raw.registrationOpen
        : true;

  return {
    name: stringOr(raw.name, 'Chatto'),
    authProviders: authProviders(raw.authProviders),
    directRegistrationEnabled: registration,
    welcomeMessage: nullableString(raw.welcomeMessage),
    description: nullableString(raw.description),
    iconUrl: nullableString(raw.iconUrl) ?? nullableString(raw.logoUrl),
    bannerUrl: nullableString(raw.bannerUrl)
  };
}

function serverInfoUrl(baseUrl: string): string {
  if (!baseUrl) return '/api/server';
  return new URL('/api/server', baseUrl).toString();
}

function authProviders(value: unknown): PublicAuthProviderInfo[] {
  if (!Array.isArray(value)) return [];
  return value
    .map((provider) => {
      if (!provider || typeof provider !== 'object') return null;
      const record = provider as Record<string, unknown>;
      const id = nullableString(record.id);
      const type = nullableString(record.type);
      const label = nullableString(record.label);
      const loginUrl = nullableString(record.loginUrl);
      if (!id || !type || !label || !loginUrl) return null;
      return { id, type, label, loginUrl };
    })
    .filter((provider): provider is PublicAuthProviderInfo => provider !== null);
}

function stringOr(value: unknown, fallback: string): string {
  return typeof value === 'string' && value ? value : fallback;
}

function nullableString(value: unknown): string | null {
  return typeof value === 'string' && value ? value : null;
}
