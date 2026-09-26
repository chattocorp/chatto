export type ApiClientHooks = {
  onAuthenticationRequired?: (serverId: string) => void;
};

let configuredHooks: ApiClientHooks = {};

export function configureApiClientHooks(hooks: ApiClientHooks): void {
  configuredHooks = hooks;
}

export function notifyAuthenticationRequired(serverId: string | undefined): void {
  if (!serverId) return;
  configuredHooks.onAuthenticationRequired?.(serverId);
}
