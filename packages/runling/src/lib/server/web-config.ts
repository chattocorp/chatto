import { ConfigReloader } from "runling/config-reloader";
let reloader: ConfigReloader | undefined;
export function getConfigReloader(): ConfigReloader {
  const path = process.env.RUNLING_WEB_CONFIG;
  if (!path) throw new Error("RUNLING_WEB_CONFIG is not set");
  return (reloader ??= new ConfigReloader(path));
}
export const loadWebConfig = () => getConfigReloader().load();
if (import.meta.hot) import.meta.hot.dispose(() => reloader?.close());
