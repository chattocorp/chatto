import { mkdir, readFile, readdir, unlink, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, '..');
const rawReferencePath = path.join(
  repoRoot,
  'apps/docs-website/src/generated/connectrpc-api/index.raw.mdx'
);
const outputDir = path.join(
  repoRoot,
  'apps/docs-website/src/content/docs/reference/connectrpc-api'
);

const groups = [
  {
    slug: 'identity',
    title: 'Identity And Accounts',
    description: 'Viewer, account, profile, presence, and member directory RPCs.',
    services: [
      'ViewerService',
      'AccountService',
      'UserService',
      'MemberDirectoryService',
      'PresenceService',
      'UserStatusService'
    ]
  },
  {
    slug: 'rooms-and-messages',
    title: 'Rooms And Messages',
    description: 'Room navigation, timelines, messages, attachments, reactions, reads, threads, and calls.',
    services: [
      'RoomDirectoryService',
      'RoomService',
      'RoomTimelineService',
      'MessageService',
      'AttachmentService',
      'ReactionService',
      'ReadStateService',
      'ThreadService',
      'LinkPreviewService',
      'VoiceCallService'
    ]
  },
  {
    slug: 'notifications',
    title: 'Notifications',
    description: 'Notification listing, preferences, counts, dismissal, and web push RPCs.',
    services: [
      'NotificationPreferencesService',
      'NotificationService',
      'PushNotificationService'
    ]
  },
  {
    slug: 'administration',
    title: 'Administration',
    description: 'Server administration, room layout, users, roles, permissions, diagnostics, and audit RPCs.',
    services: [
      'ServerService',
      'ServerStateService',
      'AdminRoomLayoutService',
      'AdminUserManagementService',
      'RoleService',
      'PermissionService',
      'AdminDiagnosticsService',
      'AdminEventLogService'
    ]
  }
];

function frontmatter(title, description) {
  return `---\ntitle: ${title}\ndescription: ${description}\neditUrl: false\n---\n\n`;
}

function generatedNotice() {
  return '{/* Generated from proto/chatto/api/v1/*.proto. Do not edit directly. */}\n\n';
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
    /\]\(#(chatto-api-v1-[^)]+)\)/g,
    '](/reference/connectrpc-api/types/#$1)'
  );
}

function rewriteRealtimeExternalLinks(section) {
  return section.replace(
    /\]\(#(chatto-api-v1-(?!Realtime)[^)]+)\)/g,
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
    'Use this reference when building integrations, bots, admin tools, or alternate clients for a Chatto server. All services in this reference use the `chatto.api.v1` protobuf package.',
    '',
    'Chatto is still pre-1.0. This reference is generated from the current protobuf contract and may change between releases.',
    '',
    '## ConnectRPC Services',
    '',
    ...groups.map((group) => `- [${group.title}](/reference/connectrpc-api/${group.slug}/) - ${group.description}`),
    '',
    '## Shared References',
    '',
    '- [Shared Types And Enums](/reference/connectrpc-api/types/) - common message and enum definitions used by service responses.',
    '- [Realtime WebSocket Protocol](/reference/connectrpc-api/realtime/) - binary protobuf frames exchanged at `/api/realtime`.'
  ];
  return renderPage(
    'ConnectRPC API',
    "Generated reference index for Chatto's public protobuf API.",
    lines.join('\n')
  );
}

function renderServiceGroup(group, serviceSections) {
  const body = [
    `Chatto exposes these ${group.title.toLowerCase()} services below \`/api/connect\`.`,
    '',
    'Shared message and enum definitions are documented in [Shared Types And Enums](/reference/connectrpc-api/types/).',
    '',
    ...group.services.map((service) => rewriteServiceTypeLinks(serviceSections.get(service).content))
  ];
  return renderPage(group.title, group.description, body.join('\n\n'));
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
    'Chatto exposes realtime updates at `GET /api/realtime` using binary protobuf frames from `chatto.api.v1`.',
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

const raw = await readFile(rawReferencePath, 'utf8');
const supportingStart = raw.indexOf('\n## Supporting Types\n');
const enumsStart = raw.indexOf('\n## Enums\n');
if (supportingStart === -1 || enumsStart === -1 || enumsStart < supportingStart) {
  throw new Error('Unable to find generated Supporting Types and Enums sections.');
}

const serviceSource = raw.slice(0, supportingStart);
const typeSource = raw.slice(supportingStart, enumsStart);
const enumSource = raw.slice(enumsStart);

const serviceSections = parseAnchoredSections(serviceSource, '##');
const typeSections = parseAnchoredSections(typeSource, '###');
const enumSections = parseAnchoredSections(enumSource, '###');

const mappedServices = new Set(groups.flatMap((group) => group.services));
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
for (const group of groups) {
  generatedPages.set(`${group.slug}.mdx`, renderServiceGroup(group, serviceSections));
}
generatedPages.set('types.mdx', renderTypesPage(typeSections, enumSections));
generatedPages.set('realtime.mdx', renderRealtimePage(typeSections, enumSections));

validateGeneratedPages(generatedPages);

await mkdir(outputDir, { recursive: true });
await removeStaleGeneratedPages(new Set(generatedPages.keys()));
for (const [filename, content] of generatedPages.entries()) {
  await writeFile(path.join(outputDir, filename), content);
}
