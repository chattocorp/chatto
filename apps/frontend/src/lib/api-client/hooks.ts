export type ApiClientHooks = {
  onAuthenticationRequired?: (serverId: string) => void;
};

let configuredHooks: ApiClientHooks = {};

export function configureApiClientHooks(hooks: ApiClientHooks): void {
  configuredHooks = hooks;
}

export function notifyAuthenticationRequired(serverId: string): void {
  configuredHooks.onAuthenticationRequired?.(serverId);
}
