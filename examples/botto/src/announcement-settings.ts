/** Operator authorization for one GitHub repository/ref and one fixed Chatto room. */
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { TunnelSettings } from './tunnel.ts';
import type { ChangeFilter } from './announcement-filter.ts';
import { expandEnvironment } from './environment.ts';

export interface AnnouncementSettings {
  /** Optional deterministic push filter, evaluated before agent startup. */
  changeFilter?: ChangeFilter;
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
  /** Opt in to published releases independently of the push branch filter. */
  releases?: boolean;
  /** Separate writing instructions for release events. */
  releasePrompt?: string;
  /** Explicit MCP connections for this session; admin tools are never enabled. */
  mcpServers?: string[];
  /** Permit push sessions to notify existing threads, including pushes without release notes. */
  threadReplies?: boolean;
}

/** Disabled configuration can keep secret placeholders until the operator enables it. */
export function parseAnnouncements(
  value: unknown,
  env: NodeJS.ProcessEnv,
  directory = process.cwd()
): AnnouncementSettings | undefined {
  const fail = (): never => {
    throw new Error(
      'Invalid announcements.github settings. Check repository, branch, room_id, webhook_secret, host, port, cloudflared_command, cloudflare_tunnel_token, prompt_file, releases, release_prompt_file, mcp_servers, thread_replies, required_changed_path, required_changed_content, added_files_only, and GitHub App credentials.'
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
          'releases',
          'release_prompt_file',
          'mcp_servers',
          'thread_replies',
          'required_changed_path',
          'required_changed_content',
          'added_files_only',
          'github_app_id',
          'github_installation_id',
          'github_private_key_path'
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
  if (config.releases !== undefined && typeof config.releases !== 'boolean') return fail();
  if (config.thread_replies !== undefined && typeof config.thread_replies !== 'boolean')
    return fail();
  let releasePrompt: string | undefined;
  if (config.release_prompt_file !== undefined) {
    try {
      releasePrompt = readFileSync(resolve(directory, text('release_prompt_file')), 'utf8').trim();
    } catch {
      return fail();
    }
    if (!releasePrompt) return fail();
  }
  let prompt: string | undefined;
  if (config.prompt_file !== undefined) {
    try {
      prompt = readFileSync(resolve(directory, text('prompt_file')), 'utf8').trim();
    } catch {
      return fail();
    }
    if (!prompt) return fail();
  }
  if (config.added_files_only !== undefined && typeof config.added_files_only !== 'boolean')
    return fail();
  if (
    config.required_changed_path === undefined &&
    (config.required_changed_content !== undefined || config.added_files_only !== undefined)
  )
    return fail();
  let changeFilter: ChangeFilter | undefined;
  if (config.required_changed_path !== undefined) {
    const pattern = text('required_changed_path');
    const contentPattern =
      config.required_changed_content === undefined ? undefined : text('required_changed_content');
    try {
      new RegExp(pattern);
      if (contentPattern) new RegExp(contentPattern);
    } catch {
      return fail();
    }
    const appId = text('github_app_id');
    const installationId = text('github_installation_id');
    if (!/^[0-9]+$/.test(appId) || !/^[0-9]+$/.test(installationId)) return fail();
    changeFilter = {
      pattern,
      ...(contentPattern ? { contentPattern } : {}),
      ...(config.added_files_only !== undefined
        ? { addedOnly: config.added_files_only as boolean }
        : {}),
      appId,
      installationId,
      privateKeyPath: resolve(directory, text('github_private_key_path'))
    };
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
    ...(changeFilter ? { changeFilter } : {}),
    ...(prompt ? { prompt } : {}),
    ...(config.releases !== undefined ? { releases: config.releases as boolean } : {}),
    ...(releasePrompt ? { releasePrompt } : {}),
    ...(mcpServers ? { mcpServers: [...new Set(mcpServers as string[])] } : {}),
    ...(config.thread_replies !== undefined
      ? { threadReplies: config.thread_replies as boolean }
      : {}),
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
