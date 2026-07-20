import { mkdir, readFile, readdir, unlink, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, '..');
const rawReferencePaths = [
  path.join(repoRoot, 'apps/docs-website/src/generated/connectrpc-api/auth.raw.mdx'),
  path.join(repoRoot, 'apps/docs-website/src/generated/connectrpc-api/discovery.raw.mdx'),
  path.join(repoRoot, 'apps/docs-website/src/generated/connectrpc-api/api.raw.mdx'),
  path.join(repoRoot, 'apps/docs-website/src/generated/connectrpc-api/admin.raw.mdx'),
  path.join(repoRoot, 'apps/docs-website/src/generated/connectrpc-api/realtime.raw.mdx')
];
const legacyRawReferencePath = path.join(
  repoRoot,
  'apps/docs-website/src/generated/connectrpc-api/index.raw.mdx'
);
const staleRawReferencePaths = [
  legacyRawReferencePath
];
const outputDir = path.join(
  repoRoot,
  'apps/docs-website/src/content/docs/reference/connectrpc-api'
);

const categories = [
  {
    title: 'chatto.auth.v1',
    services: [
      {
        name: 'ExternalIdentityAuthService',
        slug: 'external-identity-auth',
        title: 'External Identity Auth',
        description: 'Public external-identity confirmation and capability-token auth-flow RPCs.'
      }
    ]
  },
  {
    title: 'chatto.discovery.v1',
    services: [
      {
        name: 'ServerDiscoveryService',
        slug: 'server-discovery',
        title: 'Server Discovery',
        description: 'Unauthenticated server metadata, branding, and login discovery RPCs.'
      }
    ]
  },
  {
    title: 'chatto.api.v1',
    services: [
      {
        name: 'AssetService',
        slug: 'assets',
        title: 'Assets',
        description: 'Room-scoped asset metadata and signed URL read RPCs.'
      },
      {
        name: 'AssetUploadService',
        slug: 'asset-uploads',
        title: 'Asset Uploads',
        description: 'Chunked room-scoped attachment upload RPCs.'
      },
      {
        name: 'MessageService',
        slug: 'messages',
        title: 'Messages',
        description: 'Message creation, editing, deletion, composer link-preview, reaction, and attachment RPCs.'
      },
      {
        name: 'MessageSearchService',
        slug: 'message-search',
        title: 'Message Search',
        description: 'Authorized full-text message search and provider availability RPCs.'
      },
      {
        name: 'MyAccountService',
        slug: 'account',
        title: 'My Account',
        description: 'Self-service account, profile, avatar, presence, status, external identity, and settings RPCs for the authenticated user.'
      },
      {
        name: 'NotificationPreferencesService',
        slug: 'notification-preferences',
        title: 'Notification Preferences',
        description: 'Server and room notification preference RPCs.'
      },
      {
        name: 'NotificationService',
        slug: 'notifications',
        title: 'Notifications',
        description: 'Notification listing, counts, checks, and dismissal RPCs.'
      },
      {
        name: 'PushNotificationService',
        slug: 'push-notifications',
        title: 'Push Notifications',
        description: 'Web Push subscription RPCs.'
      },
      {
        name: 'RoleService',
        slug: 'roles',
        title: 'Roles',
        description: 'Authenticated role catalog read RPCs.'
      },
      {
        name: 'RoomDirectoryService',
        slug: 'room-directory',
        title: 'Room Directory',
        description: 'Room navigation, room group, and room viewer-state RPCs.'
      },
      {
        name: 'RoomService',
        slug: 'rooms',
        title: 'Rooms',
        description: 'Room lifecycle, timeline, read-state, membership, direct-message, typing indicator, and moderation RPCs.'
      },
      {
        name: 'ServerService',
        slug: 'server',
        title: 'Server',
        description: 'Authenticated server MOTD and runtime configuration RPCs.'
      },
      {
        name: 'ThreadService',
        slug: 'threads',
        title: 'Threads',
        description: 'Thread timeline, read-state, follow, and followed-thread listing RPCs.'
      },
      {
        name: 'UserService',
        slug: 'users',
        title: 'Users',
        description: 'Authenticated server-wide user directory RPCs.'
      },
      {
        name: 'ViewerService',
        slug: 'viewer',
        title: 'Viewer',
        description: 'Authenticated viewer profile, preferences, and capability RPCs.'
      },
      {
        name: 'VoiceCallService',
        slug: 'calls',
        title: 'Calls',
        description: 'Voice and video call state and token RPCs.'
      }
    ]
  },
  {
    title: 'chatto.admin.v1',
    services: [
      {
        name: 'AdminDiagnosticsService',
        slug: 'admin-diagnostics',
        title: 'Admin Diagnostics',
        description: 'System diagnostics RPCs.'
      },
      {
        name: 'AdminEventLogService',
        slug: 'admin-event-log',
        title: 'Admin Event Log',
        description: 'Audit event log read RPCs.'
      },
      {
        name: 'AdminPermissionService',
        slug: 'admin-permissions',
        title: 'Admin Permissions',
        description: 'Permission matrix, explanation, and override administration RPCs.'
      },
      {
        name: 'AdminRoleService',
        slug: 'admin-roles',
        title: 'Admin Roles',
        description: 'Role catalog and role definition administration RPCs.'
      },
      {
        name: 'AdminRoomLayoutService',
        slug: 'admin-room-layout',
        title: 'Admin Room Layout',
        description: 'Room group, sidebar layout, and sidebar link administration RPCs.'
      },
      {
        name: 'AdminServerService',
        slug: 'admin-server',
        title: 'Admin Server',
        description: 'Server profile, branding, and security administration RPCs.'
      },
      {
        name: 'AdminUserService',
        slug: 'admin-users',
        title: 'Admin Users',
        description: 'User identity, member detail, role assignment, and username-cooldown RPCs.'
      }
    ]
  }
];

const servicePages = categories.flatMap((category) => category.services);

function frontmatter(title, description) {
  return `---\ntitle: ${title}\ndescription: ${description}\neditUrl: false\n---\n\n`;
}

function generatedNotice() {
  return '{/* Generated from proto/chatto/{auth,discovery,api,admin,realtime}/v1/*.proto. Do not edit directly. */}\n\n';
}

function parseAnchoredSections(source, heading) {
  const pattern = new RegExp(`<a id="([^"]+)"></a>\\n\\n${heading} ([^\\n]+)\\n`, 'g');
  const matches = [...source.matchAll(pattern)];
  const sections = new Map();
  for (let i = 0; i < matches.length; i += 1) {
    const match = matches[i];
    const next = matches[i + 1];
    sections.set(match[2], {
      anchor: match[1],
      content: source.slice(match.index, next?.index ?? source.length).trimEnd()
    });
  }
  return sections;
}

function rewriteServiceTypeLinks(section) {
  return section
    .replace(
      /\]\(#(chatto-(?:auth|discovery|api|admin)-v1-[^)]+)\)/g,
      '](/reference/connectrpc-api/types/#$1)'
    )
    .replace(
      /`chatto\.(auth|discovery|api|admin)\.v1\.([A-Za-z][A-Za-z0-9_]*)`/g,
      (_match, pkg, typeName) =>
        `[\`chatto.${pkg}.v1.${typeName}\`](/reference/connectrpc-api/types/#chatto-${pkg}-v1-${typeName})`
    );
}

function rewriteRealtimeExternalLinks(section) {
  return section.replace(
    /\]\(#(chatto-(?:auth|discovery|api|admin)-v1-[^)]+)\)/g,
    '](/reference/connectrpc-api/types/#$1)'
  );
}

function dedupeInlineMethodTypes(content) {
  const pattern = /<a id="([^"]+)"><\/a>\n\n(#{1,6}) ([^\n]+)\n/g;
  const matches = [...content.matchAll(pattern)];
  const seenInlineTypeAnchors = new Set();
  let output = '';
  let cursor = 0;

  for (let i = 0; i < matches.length; i += 1) {
    const match = matches[i];
    const start = match.index;
    const end = matches[i + 1]?.index ?? content.length;
    const anchor = match[1];
    const headingLevel = match[2];
    const title = match[3];
    const inlineTypeMatch = title.match(/^(Input|Result): (.+)$/);

    output += content.slice(cursor, start);
    if (headingLevel === '####' && inlineTypeMatch && seenInlineTypeAnchors.has(anchor)) {
      const [, kind, typeName] = inlineTypeMatch;
      output += `#### ${kind}: [${typeName}](#${anchor})\n\nUses the same ${kind.toLowerCase()} shape documented above.\n\n`;
    } else {
      output += content.slice(start, end);
      if (headingLevel === '####' && inlineTypeMatch) {
        seenInlineTypeAnchors.add(anchor);
      }
    }
    cursor = end;
  }

  output += content.slice(cursor);
  return output;
}

function isRealtimeType(name) {
  return name.startsWith('Realtime');
}

function renderPage(title, description, body) {
  return `${frontmatter(title, description)}${generatedNotice()}${body.trim()}\n`;
}

function renderLanding() {
  const lines = [
    'Chatto exposes a protobuf-first integration API over ConnectRPC at `/api/connect`. Use it for bots, integrations, admin tooling, and alternate clients that need the same public contract as the bundled web app.',
    '',
    'ConnectRPC lets the same generated protobuf service work with the Connect, gRPC, and gRPC-Web protocols. For simple debugging you can also call unary RPCs as JSON over HTTP.',
    '',
    '## Endpoint Shape',
    '',
    'Every ConnectRPC service method is mounted below `/api/connect`:',
    '',
    '```txt',
    'https://chat.example.com/api/connect/<fully-qualified-service>/<method>',
    '```',
    '',
    'Replace `chat.example.com` with the host of the Chatto server you want to interact with.',
    '',
    'For example, public server discovery is:',
    '',
    '```txt',
    'POST /api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer',
    '```',
    '',
    '`chatto.discovery.v1` server discovery is unauthenticated. Most other documented ConnectRPC services require an `Authorization: Bearer <token>` header or a browser session when called by the bundled web client.',
    '',
    '## Authentication And Permissions',
    '',
    '[ServerDiscoveryService.GetServer](/reference/connectrpc-api/server-discovery/#chatto-discovery-v1-ServerDiscoveryService-GetServer) is public so clients can discover branding, registration state, and login providers before a user signs in.',
    '',
    '`chatto.auth.v1` external-identity confirmation calls are public but require short-lived capability tokens produced by the browser auth flow. See [External Login Providers](/guides/integrations/external-login-providers/) for login-provider discovery and sign-in configuration.',
    '',
    'Most `chatto.api.v1` calls require an authenticated user. Non-browser clients should send `Authorization: Bearer <token>`; browser clients can use the active Chatto session.',
    '',
    '`chatto.admin.v1` calls require authentication. Mutating calls and sensitive reads require the relevant server permission; a few catalog/layout reads are intentionally available to any authenticated user so clients can render assigned roles and sidebar layout. See [Permissions & Roles](/guides/operations/permissions/) for the permission model.',
    '',
    '## Packages And Namespaces',
    '',
    'The API is split by who uses each part and how clients connect to it.',
    '',
    '**`chatto.discovery.v1`**',
    '',
    '- **Transport:** ConnectRPC unary RPCs.',
    '- **Covers:** Pre-authentication bootstrap, such as server metadata and login discovery.',
    '- **Contract:** Public discovery API for clients that do not have a normal Chatto session yet.',
    '',
    '**`chatto.auth.v1`**',
    '',
    '- **Transport:** ConnectRPC unary RPCs.',
    '- **Covers:** Public external-identity confirmation steps backed by short-lived capability tokens.',
    '- **Contract:** Narrow auth-flow API for the bundled client and compatible login integrations.',
    '',
    '**`chatto.api.v1`**',
    '',
    '- **Transport:** ConnectRPC unary RPCs.',
    '- **Covers:** Normal authenticated client and integration behavior: profile reads, room navigation, messages, reactions, notifications, calls, attachments, and preferences.',
    '- **Contract:** Public client API for integrations, bots, alternate clients, and the bundled web app.',
    '',
    '**`chatto.admin.v1`**',
    '',
    '- **Transport:** ConnectRPC unary RPCs.',
    '- **Covers:** Server administration: settings, room layout, members, roles, permissions, diagnostics, and audit reads.',
    '- **Contract:** Public administrative API for tools used by server owners and administrators. Calls require authentication; mutating calls and sensitive reads require the relevant permission.',
    '',
    '**`chatto.realtime.v1`**',
    '',
    '- **Transport:** WebSocket protobuf frames at `/api/realtime`.',
    '- **Covers:** Live event delivery and realtime client synchronization.',
    '- **Contract:** Public realtime wire protocol. It is documented separately because it is not a ConnectRPC service.',
    '',
    'This split makes it clear which calls are for ordinary client behavior, which calls are administrative, and which protocol handles live updates.',
    '',
    '## Reflection',
    '',
    'Chatto exposes unauthenticated gRPC-compatible reflection for the public ConnectRPC API:',
    '',
    '```txt',
    '/api/connect/grpc.reflection.v1.ServerReflection/ServerReflectionInfo',
    '/api/connect/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo',
    '```',
    '',
    'Reflection lets tools resolve service and message descriptors without a local copy of the `.proto` files. Chatto limits reflection to public descriptors plus required imports.',
    '',
    'Because Chatto mounts ConnectRPC under `/api/connect`, use tools that accept a full Connect URL, such as `buf curl`. gRPC tools that only dial services at the host root need a proxy or path rewrite.',
    '',
    '## Usage Examples',
    '',
    '### Public JSON request with curl',
    '',
    'The Connect protocol accepts JSON for unary requests, which makes [ServerDiscoveryService.GetServer](/reference/connectrpc-api/server-discovery/#chatto-discovery-v1-ServerDiscoveryService-GetServer) easy to test with ordinary HTTP tools:',
    '',
    '```sh',
    'curl -X POST \\',
    '  -H "Content-Type: application/json" \\',
    '  -H "Connect-Protocol-Version: 1" \\',
    "  -d '{}' \\",
    '  https://chat.example.com/api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer',
    '```',
    '',
    '### Authenticated JSON request',
    '',
    'Use bearer tokens for external clients. The exact token issuance flow depends on how your integration authenticates with the server. This example calls [ViewerService.GetViewer](/reference/connectrpc-api/viewer/#chatto-api-v1-ViewerService-GetViewer).',
    '',
    '```sh',
    'curl -X POST \\',
    '  -H "Content-Type: application/json" \\',
    '  -H "Connect-Protocol-Version: 1" \\',
    '  -H "Authorization: Bearer $CHATTO_TOKEN" \\',
    "  -d '{}' \\",
    '  https://chat.example.com/api/connect/chatto.api.v1.ViewerService/GetViewer',
    '```',
    '',
    '### Reflection-backed protobuf call with buf curl',
    '',
    '`buf curl` uses protobuf schemas and can speak the Connect, gRPC, or gRPC-Web protocols. It accepts request data as protobuf JSON for CLI ergonomics, then uses reflection to resolve the request and response types. This example calls [ServerDiscoveryService.GetServer](/reference/connectrpc-api/server-discovery/#chatto-discovery-v1-ServerDiscoveryService-GetServer) over the Connect protocol:',
    '',
    '```sh',
    'buf curl --protocol connect \\',
    "  -d '{}' \\",
    '  https://chat.example.com/api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer',
    '```',
    '',
    'For a local plaintext server, use HTTP/2 prior knowledge. You can also switch to gRPC protobuf framing with `--protocol grpc`:',
    '',
    '```sh',
    'buf curl --http2-prior-knowledge \\',
    '  --protocol grpc \\',
    "  -d '{}' \\",
    '  http://localhost:4000/api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer',
    '```',
    '',
    'Add `-v` to see the reflection request before the actual RPC. The first request resolves the schema through `/api/connect/grpc.reflection.v1.ServerReflection/ServerReflectionInfo`; the second request calls your target service.',
    '',
    '### Raw binary protobuf request',
    '',
    'Generated clients and `buf curl` are usually easier, but unary Connect calls can also use raw protobuf wire bytes. Send `Content-Type: application/proto`; the request body is the serialized protobuf request message, and the response body is the serialized protobuf response message.',
    '',
    '[ServerDiscoveryService.GetServer](/reference/connectrpc-api/server-discovery/#chatto-discovery-v1-ServerDiscoveryService-GetServer) has an empty request message, so an empty binary body is valid:',
    '',
    '```sh',
    'curl -X POST \\',
    '  -H "Content-Type: application/proto" \\',
    '  --data-binary "" \\',
    '  --output get-server.bin \\',
    '  https://chat.example.com/api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer',
    '```',
    '',
    '`get-server.bin` contains a protobuf-encoded `GetServerResponse`. Decode it with generated code or a protobuf tool that has the Chatto schema.',
    '',
    '### Generated TypeScript client',
    '',
    'Generated clients use `/api/connect` as their base URL. The client appends the service and method path. Set `useBinaryFormat: true` when you want the Connect-Web client to send and receive binary protobuf instead of JSON.',
    '',
    '```ts',
    'import { createClient } from "@connectrpc/connect";',
    'import { createConnectTransport } from "@connectrpc/connect-web";',
    'import { ServerDiscoveryService } from "./gen/chatto/discovery/v1/server_connect";',
    '',
    'const transport = createConnectTransport({',
    '  baseUrl: "https://chat.example.com/api/connect",',
    '  useBinaryFormat: true,',
    '});',
    '',
    'const discovery = createClient(ServerDiscoveryService, transport);',
    'const server = await discovery.getServer({});',
    '```',
    '',
    'For authenticated calls, pass request headers through the generated client call options:',
    '',
    '```ts',
    'const viewer = await viewerClient.getViewer({}, {',
    '  headers: { Authorization: `Bearer ${token}` },',
    '});',
    '```',
    '',
    '## Responses And Errors',
    '',
    'Successful unary JSON calls return the protobuf response message as JSON. Field names use protobuf JSON casing, such as `publicProfile` and `directRegistrationEnabled`.',
    '',
    'Successful binary protobuf calls return the serialized protobuf response message with `Content-Type: application/proto`.',
    '',
    'Failed calls return Connect errors with stable codes. Common codes include:',
    '',
    '- `unauthenticated` - the call needs a signed-in user or bearer token.',
    '- `permission_denied` - the user is authenticated but lacks the required permission.',
    '- `not_found` - a singular lookup target does not exist.',
    '- `invalid_argument` - the request message failed validation.',
    '',
    'Generated clients expose those codes through their Connect client error helpers. Plain HTTP tools receive a Connect error response with an HTTP status mapped from the Connect code.',
    '',
    '## Versioning And Stability',
    '',
    'Package names such as `chatto.auth.v1`, `chatto.discovery.v1`, `chatto.api.v1`, and `chatto.admin.v1` identify the current protobuf wire namespaces that clients integrate with.',
    '',
    'Chatto is still pre-1.0, so any release may change the public API in ways that require client changes. The `v1` suffix is part of the current wire name, not a compatibility guarantee. Pin an exact server version, test integrations against the exact upgrade candidate, and read [API Compatibility](/guides/integrations/api-compatibility/) before upgrading.',
    '',
    'Use `ServerDiscoveryService.GetServer` protocol capabilities for feature discovery. If you call the API directly, ignore unknown fields and enum values when possible. Treat documented error codes and permission requirements as part of the experimental integration contract.',
    '',
    'The realtime protocol is versioned separately as `chatto.realtime.v1` because it is a WebSocket protocol rather than a ConnectRPC service.',
    '',
    '## Reference Pages',
    '',
    'Use the service pages below for request and response fields. Shared messages and enums are collected in [Shared Types And Enums](/reference/connectrpc-api/types/).',
    '',
    '## ConnectRPC Services',
    '',
    ...categories.flatMap((category) => [
      `### ${category.title}`,
      '',
      ...category.services.map((service) => `- [${service.name}](/reference/connectrpc-api/${service.slug}/) - ${service.description}`),
      ''
    ]),
    '',
    '## Shared References',
    '',
    '- [Shared Types And Enums](/reference/connectrpc-api/types/) - common message and enum definitions used by service responses.',
    '- [Realtime WebSocket Protocol](/reference/connectrpc-api/realtime/) - `chatto.realtime.v1` binary protobuf frames exchanged at `/api/realtime`.'
  ];
  return renderPage(
    'API Overview',
    "Overview of Chatto's public protobuf API.",
    lines.join('\n')
  );
}

function renderServicePage(service, serviceSections) {
  const serviceContent = dedupeInlineMethodTypes(
    rewriteServiceTypeLinks(serviceSections.get(service.name).content)
  );
  const body = [
    `Chatto exposes this service below \`/api/connect\`.`,
    '',
    'Shared message and enum definitions are documented in [Shared Types And Enums](/reference/connectrpc-api/types/).',
    '',
    serviceContent
  ];
  return renderPage(service.name, service.description, body.join('\n\n'));
}

function renderTypesPage(typeSections, enumSections) {
  const normalTypes = [...typeSections.entries()]
    .filter(([name]) => !isRealtimeType(name))
    .map(([, section]) => section.content);
  const normalEnums = [...enumSections.entries()]
    .filter(([name]) => !isRealtimeType(name))
    .map(([, section]) => section.content);

  const body = [
    'Shared message and enum definitions used by the ConnectRPC service pages.',
    '',
    '## Supporting Types',
    '',
    ...normalTypes,
    '',
    '## Enums',
    '',
    ...normalEnums
  ];

  return renderPage(
    'Shared Types And Enums',
    'Generated shared message and enum reference for Chatto ConnectRPC services.',
    body.join('\n\n')
  );
}

function renderRealtimePage(typeSections, enumSections) {
  const realtimeTypes = [...typeSections.entries()]
    .filter(([name]) => isRealtimeType(name))
    .map(([, section]) => rewriteRealtimeExternalLinks(section.content));
  const realtimeEnums = [...enumSections.entries()]
    .filter(([name]) => isRealtimeType(name))
    .map(([, section]) => rewriteRealtimeExternalLinks(section.content));

  const body = [
    'Chatto exposes realtime updates at `GET /api/realtime` using binary protobuf frames from `chatto.realtime.v1`.',
    '',
    'Realtime frames are documented separately from ConnectRPC services because they are exchanged over a long-lived WebSocket session rather than `/api/connect` RPC methods.',
    '',
    '## Protocol Types',
    '',
    ...realtimeTypes,
    '',
    '## Protocol Enums',
    '',
    ...realtimeEnums
  ];

  return renderPage(
    'Realtime WebSocket Protocol',
    'Generated protobuf frame reference for the Chatto realtime WebSocket API.',
    body.join('\n\n')
  );
}

function collectAnchors(content) {
  return new Set([...content.matchAll(/<a id="([^"]+)"><\/a>/g)].map((match) => match[1]));
}

function collectDuplicateAnchors(content) {
  const seen = new Set();
  const duplicates = new Set();
  for (const match of content.matchAll(/<a id="([^"]+)"><\/a>/g)) {
    const anchor = match[1];
    if (seen.has(anchor)) {
      duplicates.add(anchor);
    } else {
      seen.add(anchor);
    }
  }
  return duplicates;
}

function collectLocalLinks(content) {
  return [...content.matchAll(/\]\(#([^)]+)\)/g)].map((match) => match[1]);
}

function collectTypePageLinks(content) {
  return [...content.matchAll(/\]\(\/reference\/connectrpc-api\/types\/#([^)]+)\)/g)].map(
    (match) => match[1]
  );
}

function validateGeneratedPages(pages) {
  const typeAnchors = collectAnchors(pages.get('types.mdx') ?? '');
  const problems = [];
  for (const [filename, content] of pages.entries()) {
    const anchors = collectAnchors(content);
    for (const anchor of collectDuplicateAnchors(content)) {
      problems.push(`${filename} contains duplicate local anchor #${anchor}`);
    }
    for (const anchor of collectLocalLinks(content)) {
      if (!anchors.has(anchor)) {
        problems.push(`${filename} links to missing local anchor #${anchor}`);
      }
    }
    for (const anchor of collectTypePageLinks(content)) {
      if (!typeAnchors.has(anchor)) {
        problems.push(`${filename} links to missing shared type anchor #${anchor}`);
      }
    }
  }
  if (problems.length > 0) {
    throw new Error(`Generated API docs contain broken links:\n${problems.join('\n')}`);
  }
}

async function removeStaleGeneratedPages(expectedFilenames) {
  let entries = [];
  try {
    entries = await readdir(outputDir, { withFileTypes: true });
  } catch (error) {
    if (error.code !== 'ENOENT') {
      throw error;
    }
  }

  for (const entry of entries) {
    if (!entry.isFile() || !entry.name.endsWith('.mdx') || expectedFilenames.has(entry.name)) {
      continue;
    }
    const fullPath = path.join(outputDir, entry.name);
    const content = await readFile(fullPath, 'utf8');
    if (content.includes(generatedNotice().trim())) {
      await unlink(fullPath);
    }
  }
}

const serviceSections = new Map();
const typeSections = new Map();
const enumSections = new Map();
for (const rawReferencePath of rawReferencePaths) {
  const raw = await readFile(rawReferencePath, 'utf8');
  const supportingStart = raw.indexOf('\n## Supporting Types\n');
  const enumsStart = raw.indexOf('\n## Enums\n');
  if (enumsStart !== -1 && supportingStart !== -1 && enumsStart < supportingStart) {
    throw new Error(`Generated Enums section appears before Supporting Types in ${rawReferencePath}.`);
  }

  const serviceEnd =
    supportingStart === -1 ? (enumsStart === -1 ? raw.length : enumsStart) : supportingStart;
  const typeEnd = enumsStart === -1 ? raw.length : enumsStart;
  const serviceSource = raw.slice(0, serviceEnd);
  const typeSource = supportingStart === -1 ? '' : raw.slice(supportingStart, typeEnd);
  const enumSource = enumsStart === -1 ? '' : raw.slice(enumsStart);

  for (const [name, section] of parseAnchoredSections(serviceSource, '##')) {
    serviceSections.set(name, section);
  }
  for (const [name, section] of parseAnchoredSections(typeSource, '###')) {
    typeSections.set(name, section);
  }
  for (const [name, section] of parseAnchoredSections(enumSource, '###')) {
    enumSections.set(name, section);
  }
}

const mappedServices = new Set(servicePages.map((service) => service.name));
const generatedServices = new Set(serviceSections.keys());
const missing = [...mappedServices].filter((service) => !generatedServices.has(service));
const unmapped = [...generatedServices].filter((service) => !mappedServices.has(service));
if (missing.length > 0 || unmapped.length > 0) {
  throw new Error(
    [
      missing.length > 0 ? `Missing generated services: ${missing.join(', ')}` : '',
      unmapped.length > 0 ? `Unmapped generated services: ${unmapped.join(', ')}` : ''
    ]
      .filter(Boolean)
      .join('\n')
  );
}

const generatedPages = new Map([['index.mdx', renderLanding()]]);
for (const service of servicePages) {
  generatedPages.set(`${service.slug}.mdx`, renderServicePage(service, serviceSections));
}
generatedPages.set('types.mdx', renderTypesPage(typeSections, enumSections));
generatedPages.set('realtime.mdx', renderRealtimePage(typeSections, enumSections));

validateGeneratedPages(generatedPages);

await mkdir(outputDir, { recursive: true });
await removeStaleGeneratedPages(new Set(generatedPages.keys()));
for (const staleRawReferencePath of staleRawReferencePaths) {
  try {
    await unlink(staleRawReferencePath);
  } catch (error) {
    if (error.code !== 'ENOENT') {
      throw error;
    }
  }
}
for (const [filename, content] of generatedPages.entries()) {
  await writeFile(path.join(outputDir, filename), content);
}
