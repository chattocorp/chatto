import { mkdir, readFile, readdir, unlink, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, '..');
const rawReferencePaths = [
  path.join(repoRoot, 'apps/docs-website/src/generated/connectrpc-api/api.raw.mdx'),
  path.join(repoRoot, 'apps/docs-website/src/generated/connectrpc-api/admin.raw.mdx'),
  path.join(repoRoot, 'apps/docs-website/src/generated/connectrpc-api/realtime.raw.mdx')
];
const legacyRawReferencePath = path.join(
  repoRoot,
  'apps/docs-website/src/generated/connectrpc-api/index.raw.mdx'
);
const outputDir = path.join(
  repoRoot,
  'apps/docs-website/src/content/docs/reference/connectrpc-api'
);

const categories = [
  {
    title: 'chatto.api.v1',
    services: [
      {
        name: 'ServerDiscoveryService',
        slug: 'server-discovery',
        title: 'Server Discovery',
        description: 'Unauthenticated server metadata, branding, and login discovery RPCs.'
      },
      {
        name: 'ServerService',
        slug: 'server',
        title: 'Server',
        description: 'Authenticated server state and current-user server capability RPCs.'
      },
      {
        name: 'ViewerService',
        slug: 'viewer',
        title: 'Viewer',
        description: 'Authenticated viewer profile, preferences, and capability RPCs.'
      },
      {
        name: 'AccountService',
        slug: 'account',
        title: 'Account',
        description: 'Self-service account, profile, avatar, presence, status, and settings RPCs.'
      },
      {
        name: 'UserDirectoryService',
        slug: 'user-directory',
        title: 'User Directory',
        description: 'Authenticated public user profile lookup RPCs.'
      },
      {
        name: 'MemberDirectoryService',
        slug: 'member-directory',
        title: 'Member Directory',
        description: 'Server and room member directory RPCs.'
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
        description: 'Room lifecycle, membership, direct-message, and moderation RPCs.'
      },
      {
        name: 'RoomTimelineService',
        slug: 'room-timeline',
        title: 'Room Timeline',
        description: 'Room and thread timeline read RPCs.'
      },
      {
        name: 'MessageService',
        slug: 'messages',
        title: 'Messages',
        description: 'Message posting, editing, deletion, link-preview, attachment, and typing RPCs.'
      },
      {
        name: 'AttachmentService',
        slug: 'attachments',
        title: 'Attachments',
        description: 'Attachment listing and signed URL refresh RPCs.'
      },
      {
        name: 'ReactionService',
        slug: 'reactions',
        title: 'Reactions',
        description: 'Message reaction command RPCs.'
      },
      {
        name: 'ReadStateService',
        slug: 'read-state',
        title: 'Read State',
        description: 'Room and thread read-state command RPCs.'
      },
      {
        name: 'ThreadService',
        slug: 'threads',
        title: 'Threads',
        description: 'Thread follow and followed-thread listing RPCs.'
      },
      {
        name: 'LinkPreviewService',
        slug: 'link-previews',
        title: 'Link Previews',
        description: 'Link preview fetch RPCs.'
      },
      {
        name: 'VoiceCallService',
        slug: 'calls',
        title: 'Calls',
        description: 'Voice and video call state and token RPCs.'
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
      }
    ]
  },
  {
    title: 'chatto.admin.v1',
    services: [
      {
        name: 'AdminServerService',
        slug: 'admin-server',
        title: 'Admin Server',
        description: 'Server profile, branding, and security administration RPCs.'
      },
      {
        name: 'AdminRoomLayoutService',
        slug: 'admin-room-layout',
        title: 'Admin Room Layout',
        description: 'Room group, sidebar layout, and sidebar link administration RPCs.'
      },
      {
        name: 'AdminMemberService',
        slug: 'admin-members',
        title: 'Admin Members',
        description: 'Member identity, role assignment, and user administration RPCs.'
      },
      {
        name: 'AdminRoleService',
        slug: 'admin-roles',
        title: 'Admin Roles',
        description: 'Role catalog and role definition administration RPCs.'
      },
      {
        name: 'AdminPermissionService',
        slug: 'admin-permissions',
        title: 'Admin Permissions',
        description: 'Permission matrix, explanation, and override administration RPCs.'
      },
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
      }
    ]
  }
];

const servicePages = categories.flatMap((category) => category.services);

function frontmatter(title, description) {
  return `---\ntitle: ${title}\ndescription: ${description}\neditUrl: false\n---\n\n`;
}

function generatedNotice() {
  return '{/* Generated from proto/chatto/{api,admin,realtime}/v1/*.proto. Do not edit directly. */}\n\n';
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
  return section.replace(
    /\]\(#(chatto-(?:api|admin)-v1-[^)]+)\)/g,
    '](/reference/connectrpc-api/types/#$1)'
  );
}

function rewriteRealtimeExternalLinks(section) {
  return section.replace(
    /\]\(#(chatto-(?:api|admin)-v1-[^)]+)\)/g,
    '](/reference/connectrpc-api/types/#$1)'
  );
}

function isRealtimeType(name) {
  return name.startsWith('Realtime');
}

function renderPage(title, description, body) {
  return `${frontmatter(title, description)}${generatedNotice()}${body.trim()}\n`;
}

function renderLanding() {
  const lines = [
    'Chatto exposes a protobuf-first integration API over ConnectRPC at `/api/connect`.',
    '',
    'Endpoint paths use the Connect convention:',
    '',
    '`/api/connect/<fully-qualified-service>/<method>`',
    '',
    'Use this reference when building integrations, bots, admin tools, or alternate clients for a Chatto server. Public unary services use `chatto.api.v1`; privileged administration services use `chatto.admin.v1`.',
    '',
    'Chatto is still pre-1.0. This reference is generated from the current protobuf contract and may change between releases.',
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
    'ConnectRPC API',
    "Generated reference index for Chatto's public protobuf API.",
    lines.join('\n')
  );
}

function renderServicePage(service, serviceSections) {
  const body = [
    `Chatto exposes this service below \`/api/connect\`.`,
    '',
    'Shared message and enum definitions are documented in [Shared Types And Enums](/reference/connectrpc-api/types/).',
    '',
    rewriteServiceTypeLinks(serviceSections.get(service.name).content)
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
  if (supportingStart === -1 || enumsStart === -1 || enumsStart < supportingStart) {
    throw new Error(`Unable to find generated Supporting Types and Enums sections in ${rawReferencePath}.`);
  }

  const serviceSource = raw.slice(0, supportingStart);
  const typeSource = raw.slice(supportingStart, enumsStart);
  const enumSource = raw.slice(enumsStart);

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
try {
  await unlink(legacyRawReferencePath);
} catch (error) {
  if (error.code !== 'ENOENT') {
    throw error;
  }
}
for (const [filename, content] of generatedPages.entries()) {
  await writeFile(path.join(outputDir, filename), content);
}
