/** Operator authorization for one GitHub repository/ref and one fixed Chatto room. */
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { TunnelSettings } from './tunnel.ts';
import { expandEnvironment } from './environment.ts';

export interface AnnouncementSettings {
  repository: string;
  branch: string;
  roomId: string;
  secret: string;
  host: string;
  port: number;
  /** Optional connector, owned by the webhook listener. */
  tunnel?: TunnelSettings;
  /** Operator-owned writing instructions, loaded from prompt_file at configuration time. */
  prompt?: string;
  /** Explicit MCP connections for this session; admin tools are never enabled. */
  mcpServers?: string[];
}

/** Disabled configuration can keep secret placeholders until the operator enables it. */
export function parseAnnouncements(
  value: unknown,
  env: NodeJS.ProcessEnv,
  directory = process.cwd()
): AnnouncementSettings | undefined {
  const fail = (): never => {
    throw new Error(
      'Invalid announcements.github settings. Check repository, branch, room_id, webhook_secret, host, port, cloudflared_command, cloudflare_tunnel_token, prompt_file, and mcp_servers.'
    );
  };
  const table = (v: unknown): Record<string, unknown> => {
    if (!v || typeof v !== 'object' || Array.isArray(v)) return fail();
    return v as Record<string, unknown>;
  };
  if (value === undefined) return;
  const root = table(value);
  if (Object.keys(root).some((k) => k !== 'github')) return fail();
  const config = table(root.github ?? {});
  if (
    Object.keys(config).some(
      (k) =>
        ![
          'enabled',
          'repository',
          'branch',
          'room_id',
          'webhook_secret',
          'host',
          'port',
          'cloudflared_command',
          'cloudflare_tunnel_token',
          'prompt_file',
          'mcp_servers'
        ].includes(k)
    )
  )
    return fail();
  if (config.enabled !== undefined && typeof config.enabled !== 'boolean') return fail();
  if (config.enabled !== true) return;
  const text = (key: string, fallback?: string) => {
    const raw = config[key] ?? fallback;
    if (typeof raw !== 'string') return fail();
    let result: string;
    try {
      result = expandEnvironment(raw, env, 'announcements.github').trim();
    } catch {
      return fail();
    }
    if (!result || /[\r\n\0]/.test(result)) return fail();
    return result;
  };
  if (config.cloudflared_command !== undefined && config.cloudflare_tunnel_token === undefined)
    return fail();
  const mcpServers = config.mcp_servers;
  if (
    mcpServers !== undefined &&
    (!Array.isArray(mcpServers) ||
      mcpServers.some((name) => typeof name !== 'string' || !/^[a-z][a-z0-9_]*$/.test(name)))
  )
    return fail();
  let prompt: string | undefined;
  if (config.prompt_file !== undefined) {
    try {
      prompt = readFileSync(resolve(directory, text('prompt_file')), 'utf8').trim();
    } catch {
      return fail();
    }
    if (!prompt) return fail();
  }
  const repository = text('repository');
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repository)) return fail();
  const branch = text('branch', 'main');
  if (/\s/.test(branch) || branch.startsWith('refs/')) return fail();
  const port = config.port ?? 5152;
  if (typeof port !== 'number' || !Number.isInteger(port) || port < 1 || port > 65535)
    return fail();
  const secret = text('webhook_secret');
  if (secret.length < 32) return fail();
  return {
    ...(prompt ? { prompt } : {}),
    ...(mcpServers ? { mcpServers: [...new Set(mcpServers as string[])] } : {}),
    repository,
    branch,
    roomId: text('room_id'),
    secret,
    host: text('host', '127.0.0.1'),
    port,
    ...(config.cloudflare_tunnel_token !== undefined
      ? {
          tunnel: {
            command: text('cloudflared_command', 'cloudflared'),
            token: text('cloudflare_tunnel_token')
          }
        }
      : {})
  };
}
