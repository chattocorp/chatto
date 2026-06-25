import { getContext, setContext } from 'svelte';
import type { PublicServerInfo } from '$lib/api/server';

const AUTH_SERVER_INFO = Symbol('auth-server-info');

export type AuthServerInfoGetter = () => PublicServerInfo | null;

export function setAuthServerInfo(getServerInfo: AuthServerInfoGetter): void {
  setContext(AUTH_SERVER_INFO, getServerInfo);
}

export function getAuthServerInfo(): AuthServerInfoGetter {
  return getContext<AuthServerInfoGetter>(AUTH_SERVER_INFO) ?? (() => null);
}
