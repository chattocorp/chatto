/** Validate operator-installed MCP connections without including credentials in errors. */
import { expandEnvironment } from './environment.ts';
import { dirname, resolve } from 'node:path';
import type { McpServerEntry } from '@earendil-works/pi-coding-agent';

/** Exact remote tool names; omitted lists grant no tool access. */
export interface AuthorizedMcpServer extends McpServerEntry {
  tools: string[];
  adminTools: string[];
}

/** Custom servers belong to the main agent. Built-in namespaces remain reserved. */
export function parseMcpServers(
  value: unknown,
  configPath: string,
  env: NodeJS.ProcessEnv
): AuthorizedMcpServer[] {
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
  const text = (input: unknown, allowEmpty = false): string => {
    if (typeof input !== 'string') return fail();
    let expanded: string;
    try {
      expanded = expandEnvironment(input, env, 'mcp.servers');
    } catch {
      return fail();
    }
    if ((!allowEmpty && !expanded.trim()) || expanded.includes('\0')) return fail();
    return expanded;
  };
  // Pi resolves dollar templates and leading !commands. These are already resolved data.
  const literal = (input: string) => input.replaceAll('$', () => '$$').replace(/^!/, '$!');
  const values = (input: unknown, headers: boolean) =>
    Object.fromEntries(
      Object.entries(table(input ?? {})).map(([key, raw]) => {
        if (!(headers ? /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/ : /^[A-Za-z_][A-Za-z0-9_]*$/).test(key))
          return fail();
        const resolved = text(raw);
        if (headers && /[\r\n]/.test(resolved)) return fail();
        return [key, literal(resolved)];
      })
    );
  const root = table(value ?? {});
  if (Object.keys(root).some((key) => key !== 'servers')) return fail();
  return Object.entries(table(root.servers ?? {})).map(([name, raw]) => {
    if (
      !/^[a-z][a-z0-9_]*$/.test(name) ||
      name.includes('__') ||
      ['chatto', 'exa', 'memory'].includes(name)
    )
      return fail();
    const server = table(raw);
    const remote = server.url !== undefined;
    const allowed = [
      ...(remote ? ['url', 'headers'] : ['command', 'args', 'env']),
      'tools',
      'admin_tools'
    ];
    if (Object.keys(server).some((key) => !allowed.includes(key))) return fail();
    const names = (value: unknown): string[] => {
      if (value === undefined) return [];
      if (
        !Array.isArray(value) ||
        value.some((item) => typeof item !== 'string' || !/^[A-Za-z0-9_-]+$/.test(item))
      )
        return fail();
      return value;
    };
    const tools = names(server.tools);
    const adminTools = names(server.admin_tools);
    // Pi normalizes hyphens in tool names. Reject ambiguous grants across both lists.
    const normalized = [...tools, ...adminTools].map((tool) => tool.replaceAll('-', '_'));
    if (new Set(normalized).size !== normalized.length) return fail();
    const base = { name, source: configPath, scope: 'extension' as const, tools, adminTools };
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
          headers: values(server.headers, true),
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
        args: args.map((arg) => text(arg, true)),
        cwd: dirname(resolve(configPath)),
        env: values(server.env, false),
        exposure: 'direct' as const
      }
    };
  });
}
