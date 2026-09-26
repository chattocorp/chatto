export type ApiClientHooks = {
  /** Called when a request is rejected as unauthenticated. `source` names the request. */
  onAuthenticationRequired?: (serverId: string, source: string) => void;
};

let configuredHooks: ApiClientHooks = {};

export function configureApiClientHooks(hooks: ApiClientHooks): void {
  configuredHooks = hooks;
}

export function notifyAuthenticationRequired(serverId: string, source: string): void {
  configuredHooks.onAuthenticationRequired?.(serverId, source);
}
