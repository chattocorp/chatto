/** Expand parsed strings once. Environment values are data, never TOML or more templates.
 * A doubled dollar sign is literal, so $${NAME} produces ${NAME} unchanged. */
export function expandEnvironment(value: string, env: NodeJS.ProcessEnv, label: string): string {
  return value.replace(
    /\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g,
    (match: string, name: string | undefined) => {
      if (match === '$$') return '$';
      if (name === undefined || !Object.hasOwn(env, name) || env[name] === undefined)
        throw new Error(`${label} references missing environment variable ${name}`);
      return env[name]!;
    }
  );
}
