/** Validate operator-installed MCP connections without including credentials in errors. */
import { dirname, resolve } from 'node:path';
import type { McpServerEntry } from '@earendil-works/pi-coding-agent';

/** Custom servers belong to the main agent. Built-in namespaces remain reserved. */
export function parseMcpServers(
  value: unknown,
  configPath: string,
  env: NodeJS.ProcessEnv
): McpServerEntry[] {
  const fail = (): never => {
    throw new Error(
      'Invalid mcp.servers configuration. Check server names, transport fields, and environment references.'
    );
  };
  const table = (input: unknown): Record<string, unknown> => {
    if (!input || typeof input !== 'object' || Array.isArray(input) || input instanceof Date)
      return fail();
    return input as Record<string, unknown>;
  };
  const text = (input: unknown): string => {
    if (typeof input !== 'string' || !input.trim() || input.includes('\0')) return fail();
    return input;
  };
  // Pi resolves dollar templates and leading !commands. These are already resolved data.
  const literal = (input: string) => input.replaceAll('$', () => '$$').replace(/^!/, '$!');
  const references = (input: unknown, headers: boolean) =>
    Object.fromEntries(
      Object.entries(table(input ?? {})).map(([key, raw]) => {
        if (!(headers ? /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/ : /^[A-Za-z_][A-Za-z0-9_]*$/).test(key))
          return fail();
        const name = text(raw);
        if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name) || !Object.hasOwn(env, name)) return fail();
        const resolved = text(env[name]);
        if (headers && /[\r\n]/.test(resolved)) return fail();
        return [key, literal(resolved)];
      })
    );
  const root = table(value ?? {});
  if (Object.keys(root).some((key) => key !== 'servers')) return fail();
  return Object.entries(table(root.servers ?? {})).map(([name, raw]) => {
    if (!/^[a-z][a-z0-9_]*$/.test(name) || ['chatto', 'exa', 'memory'].includes(name))
      return fail();
    const server = table(raw);
    const remote = server.url !== undefined;
    const allowed = remote ? ['url', 'headers_from_env'] : ['command', 'args', 'env_from_env'];
    if (Object.keys(server).some((key) => !allowed.includes(key))) return fail();
    const base = { name, source: configPath, scope: 'extension' as const };
    if (remote) {
      const url = text(server.url);
      try {
        const parsed = new URL(url);
        if (
          parsed.username ||
          parsed.password ||
          parsed.hash ||
          (parsed.protocol !== 'https:' &&
            !(
              parsed.protocol === 'http:' &&
              ['localhost', '127.0.0.1', '[::1]'].includes(parsed.hostname)
            ))
        )
          return fail();
      } catch {
        return fail();
      }
      return {
        ...base,
        config: {
          url,
          headers: references(server.headers_from_env, true),
          exposure: 'direct' as const
        }
      };
    }
    const args = server.args ?? [];
    if (!Array.isArray(args) || args.some((arg) => typeof arg !== 'string' || arg.includes('\0')))
      return fail();
    return {
      ...base,
      config: {
        command: text(server.command),
        args: args as string[],
        cwd: dirname(resolve(configPath)),
        env: references(server.env_from_env, false),
        exposure: 'direct' as const
      }
    };
  });
}
