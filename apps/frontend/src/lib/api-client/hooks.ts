export type ApiClientHooks = {
  onAuthenticationRequired?: (serverId: string) => void;
};

let configuredHooks: ApiClientHooks = {};

export function configureApiClientHooks(hooks: ApiClientHooks): void {
  configuredHooks = hooks;
}

export function notifyAuthenticationRequired(
  serverId: string | undefined,
  localHook?: (serverId: string) => void
): void {
  if (!serverId) return;
  localHook?.(serverId);
  configuredHooks.onAuthenticationRequired?.(serverId);
}
