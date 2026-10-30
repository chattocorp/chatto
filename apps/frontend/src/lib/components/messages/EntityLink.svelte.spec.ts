/** Exercise Markdown replacement, live projection boundaries, and native link actions together. */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import { createClient } from '@chatto/client';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import MessageContent from '../MessageContent.svelte';
import TipTapEditor from '../composer/TipTapEditor.svelte';
import type { ComposerEditorApi } from '../composer/editorTypes';
import { ServerInfoState } from '@chatto/client/server/state';
import { TimelineEventKind } from '@chatto/client/timeline/timelineEvents';
import { createRoomTimelineAPI } from '@chatto/client/api/roomTimeline';
import '../../../app.css';

const mocks = vi.hoisted(() => ({
  registry: undefined as ReturnType<typeof createClient>['registry'] | undefined,
  goto: vi.fn()
}));
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    get servers() {
      return mocks.registry!.servers;
    },
    get originServer() {
      return mocks.registry!.originServer;
    },
    getServer: (id: string) => mocks.registry!.getServer(id),
    tryGetStore: (id: string) => mocks.registry!.tryGetStore(id),
    getStore: (id: string) => mocks.registry!.getStore(id),
    isOriginServer: (id: string) => mocks.registry!.isOriginServer(id)
  }
}));
vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  afterNavigate: vi.fn()
}));

const origin = 'https://known.example.test';
const roomId = 'R123456789abcde';
const userId = 'U123456789abcde';
const avatarUrl =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==';
let client: ReturnType<typeof createClient>;

function putUser(displayName = 'Alice') {
  client.registry.getStore('known').projection.users.set(
    userId,
    new DirectoryMember({
      user: { id: userId, login: 'alice', displayName, avatarUrl }
    })
  );
}

beforeEach(() => {
  vi.spyOn(ServerInfoState.prototype, 'init').mockResolvedValue();
  client = createClient();
  mocks.registry = client.registry;
  mocks.goto.mockClear();
  client.registry.addServer({
    id: 'known',
    url: origin,
    name: 'Known server',
    iconUrl: null,
    token: null,
    userId: null,
    userLogin: null,
    userDisplayName: null,
    userAvatarUrl: null,
    reauthRequiredAt: null,
    addedAt: 1
  });
  const store = client.registry.getStore('known');
  store.realtimeSync.phase = 'ready';
  store.projection.rooms.set(
    roomId,
    new RoomWithViewerState({
      room: { id: roomId, name: 'General', kind: RoomKind.CHANNEL }
    })
  );
  putUser();
});
afterEach(() => {
  client.close();
  vi.restoreAllMocks();
});

describe('entity links in messages', () => {
  it('opens a known destination from its hover card using local navigation', async () => {
    const { container } = render(MessageContent, { body: `${origin}/chat/-/${roomId}` });
    const pill = await vi.waitFor(() => {
      const link = container.querySelector<HTMLAnchorElement>('.entity-link');
      expect(link).not.toBeNull();
      return link!;
    });
    await userEvent.hover(pill);
    const card = await vi.waitFor(() => {
      const link = container.querySelector<HTMLAnchorElement>('[data-testid="link-hover-card"]');
      expect(link).not.toBeNull();
      return link!;
    });
    expect(card.getAttribute('href')).toBe(`/chat/known.example.test/${roomId}`);
    card.click();
    expect(mocks.goto).toHaveBeenCalledExactlyOnceWith(`/chat/known.example.test/${roomId}`);
  });
  it('describes message destinations from local data, including thread replies, without loading a timeline', async () => {
    const store = client.registry.getStore('known');
    const messageId = 'E123456789abcde';
    const threadId = 'Eabcdefghijklm0';
    const href = `${origin}/chat/-/${roomId}/${threadId}/m/${messageId}`;
    const createTimeline = vi.spyOn(store.rooms, 'messages');
    const { container } = render(MessageContent, { body: `${href} [the original post](${href})` });
    await vi.waitFor(() => expect(container.querySelector('[data-message-link]')).not.toBeNull());
    expect(container.querySelector('[data-message-link]')?.textContent).toContain('General');
    expect(container.querySelector('[data-message-link]')?.getAttribute('data-content-href')).toBe(
      href
    );
    expect(createTimeline).not.toHaveBeenCalled();
    expect(container.querySelectorAll('[data-message-link]')).toHaveLength(1);
    expect(container.textContent).toContain('the original post');
    container.querySelector<HTMLAnchorElement>('[data-message-link]')!.click();
    expect(mocks.goto).toHaveBeenCalledWith(
      `/chat/known.example.test/${roomId}/${threadId}/m/${messageId}`
    );
  });

  it('uses a cached author and follows edits and message access loss', async () => {
    const store = client.registry.getStore('known');
    const messageId = 'E123456789abcde';
    vi.spyOn(store.connection.getAPI(createRoomTimelineAPI), 'getRoomEvents').mockResolvedValue({
      events: [],
      hasOlder: false,
      hasNewer: false,
      startCursor: null,
      endCursor: null
    });
    const timeline = store.rooms.messages(roomId);
    await vi.waitFor(() => expect(timeline.isInitialLoading).toBe(false));
    timeline.events = [
      {
        id: messageId,
        createdAt: '2026-10-30T12:00:00Z',
        actor: store.projection.users.view(userId),
        event: {
          kind: TimelineEventKind.MessagePosted,
          roomId,
          body: 'An existing message',
          attachments: [],
          replyCount: 0,
          threadParticipants: [],
          reactions: []
        }
      }
    ];
    const { container } = render(MessageContent, {
      body: `${origin}/chat/-/${roomId}/m/${messageId}`
    });
    await vi.waitFor(() =>
      expect(container.querySelector('[data-message-link]')?.textContent).toContain('Alice')
    );
    putUser('Alicia');
    await vi.waitFor(() =>
      expect(container.querySelector('[data-message-link]')?.textContent).toContain('Alicia')
    );
    timeline.clearForAccessRevocation();
    await vi.waitFor(() =>
      expect(container.querySelector('[data-message-link]')?.textContent).not.toContain('Alicia')
    );
    expect(container.querySelector('[data-message-link]')?.textContent).toContain('General');
  });

  it('uses the entity identifier when local profile data is unavailable', async () => {
    const { container } = render(MessageContent, {
      body: `${origin}/chat/-/user/missing`
    });
    await vi.waitFor(() => expect(container.querySelector('.url-link')).not.toBeNull());
    const link = container.querySelector('a')!;
    await userEvent.hover(link);
    await vi.waitFor(() =>
      expect(container.querySelector('[data-testid="link-hover-card"]')).not.toBeNull()
    );
    const heading = container.querySelector('[data-testid="link-hover-card"] .font-semibold');
    expect(heading?.textContent).toBe('missing');
    await userEvent.unhover(link);
  });

  it('replaces bare and angle-bracket URLs with known names and images', async () => {
    const { container } = render(MessageContent, {
      body: `${origin}\n\n<${origin}/chat/-/${roomId}> and ${origin}/chat/-/user/${userId}`
    });
    await vi.waitFor(() => expect(container.querySelectorAll('.entity-link')).toHaveLength(3));
    expect(container.querySelector('[data-entity-kind="server"]')?.textContent).toContain(
      'Known server'
    );
    expect(container.querySelector('[data-entity-kind="room"] bdi')?.textContent?.trim()).toBe(
      'General'
    );
    const user = container.querySelector('[data-entity-kind="user"]')!;
    expect(user.textContent).toContain('Alice');
    expect(user.querySelector('img')?.getAttribute('src')).toBe(avatarUrl);
    expect(user.getAttribute('href')).toBe(`/chat/known.example.test/user/${userId}`);
  });

  it('preserves explicit labels, including formatted labels and labels equal to their URL', async () => {
    const { container } = render(MessageContent, {
      body: `Please join [the **wonderful** development channel](${origin}/chat/-/${roomId}). [${origin}](${origin})`
    });
    await vi.waitFor(() => expect(container.querySelectorAll('a')).toHaveLength(2));
    expect(container.textContent).toContain('Please join the wonderful development channel.');
    expect(container.querySelector('strong')?.textContent).toBe('wonderful');
    expect(container.querySelectorAll('.entity-link')).toHaveLength(0);
  });

  it('previews authored links without replacing their text, formatting, or title', async () => {
    const { container } = render(MessageContent, {
      body: `Please join [the **wonderful** channel](${origin}/chat/-/${roomId} "Our development room").`
    });
    await vi.waitFor(() => expect(container.querySelector('a strong')).not.toBeNull());
    const link = container.querySelector('a')!;
    expect(link.title).toBe('Our development room');
    expect(container.querySelector('.entity-link, .url-link')).toBeNull();
    await userEvent.hover(link);
    await vi.waitFor(() =>
      expect(container.querySelector('[data-testid="link-hover-card"]')?.textContent).toContain(
        'General'
      )
    );
    expect(link.textContent).toBe('the wonderful channel');
    expect(link.querySelector('strong')?.textContent).toBe('wonderful');
    expect(link.getAttribute('aria-describedby')).toBeTruthy();
    await userEvent.unhover(link);
    await vi.waitFor(() =>
      expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull()
    );
    expect(link.title).toBe('Our development room');
    expect(link.hasAttribute('aria-describedby')).toBe(false);
  });

  it('uses neutral chips for unknown destinations and non-entity routes, preserving full URLs', async () => {
    const links = [
      'https://unknown.example.test/chat/-/user/' + userId,
      'https://unknown.example.test/chat/known.example.test/user/' + userId,
      origin + '/chat/-/user/missing',
      origin + '/chat/-/R00000000000000',
      origin + `/chat/-/${roomId}/E123456789abcde`,
      origin + '/chat/-/manage/server',
      origin + '/?token=example',
      origin + '/#section'
    ];
    const { container } = render(MessageContent, {
      body: links.join(' ') + ` \`${origin}\``
    });
    await vi.waitFor(() =>
      expect(container.querySelectorAll('.url-link')).toHaveLength(links.length)
    );
    expect(container.querySelectorAll('.entity-link')).toHaveLength(0);
    expect(
      Array.from(container.querySelectorAll('.url-link'), (link) =>
        link.getAttribute('data-content-href')
      )
    ).toEqual(links);
    expect(
      Array.from(container.querySelectorAll('.url-link'), (link) => link.getAttribute('title'))
    ).toEqual(links);
    expect(container.querySelector('code')?.textContent).toBe(origin);
  });

  it('updates names and restores the URL after deletion, then resolves new data', async () => {
    const { container } = render(MessageContent, {
      body: `${origin}/chat/-/user/${userId}`
    });
    await vi.waitFor(() =>
      expect(container.querySelector('.entity-link')?.textContent).toContain('Alice')
    );
    putUser('<img onerror=alert(1)>');
    await vi.waitFor(() =>
      expect(container.querySelector('.entity-link')?.textContent).toContain(
        '<img onerror=alert(1)>'
      )
    );
    expect(container.querySelector('[onerror]')).toBeNull();
    client.registry.getStore('known').projection.users.delete(userId);
    await vi.waitFor(() =>
      expect(container.querySelector('.url-link')?.getAttribute('title')).toBe(
        `${origin}/chat/-/user/${userId}`
      )
    );
    expect(container.querySelector('.entity-link')).toBeNull();
    putUser('Restored');
    await vi.waitFor(() =>
      expect(container.querySelector('.entity-link')?.textContent).toContain('Restored')
    );
  });

  it('bounds long web links, preserves the full destination, and leaves authored labels and email alone', async () => {
    const href = `https://docs.example.test:8443/${'long-path/'.repeat(30)}?q=value#section`;
    const opened = vi.spyOn(window, 'open').mockReturnValue(null);
    const { container } = render(MessageContent, {
      body: `${href} [read the docs](${href}) <reader@example.test>`
    });
    await vi.waitFor(() => expect(container.querySelector('.url-link')).not.toBeNull());
    const chip = container.querySelector<HTMLAnchorElement>('.url-link')!;
    expect(chip.textContent).toBe('docs.example.test:8443');
    expect(chip.href).toBe(href);
    expect(chip.title).toBe(href);
    expect(document.getElementById(chip.getAttribute('aria-describedby')!)?.textContent).toBe(href);
    expect(chip.textContent).not.toContain('?q=');
    expect(chip.textContent).not.toContain('#section');
    expect(chip.textContent).not.toContain('long-path');
    expect(chip.getBoundingClientRect().width).toBeLessThan(400);
    chip.click();
    expect(opened).toHaveBeenCalledExactlyOnceWith(href, '_blank', 'noopener,noreferrer');
    expect(container.querySelectorAll('.url-link')).toHaveLength(1);
    expect(container.textContent).toContain('read the docs');
    expect(container.querySelector('a[href="mailto:reader@example.test"]')).not.toBeNull();
  });

  it('keeps the hostname ending and port visible without a second truncated path', async () => {
    const href = 'http://chatto.ws55110.localhost:55110/chat/-/user/UT9SQd0NQaXj3pc';
    const { container } = render(MessageContent, { body: href });
    await vi.waitFor(() => expect(container.querySelector('.url-link')).not.toBeNull());
    const link = container.querySelector<HTMLAnchorElement>('.url-link')!;
    expect(link.textContent).toMatch(/^chatt….*localhost:55110$/);
    expect(link.textContent?.match(/…/g)).toHaveLength(1);
    expect(link.href).toBe(href);
    expect(link.title).toBe(href);
  });

  it('upgrades an unknown URL after server registration without rendering the message again', async () => {
    const href = 'https://new.example.test';
    const { container } = render(MessageContent, { body: href });
    await vi.waitFor(() => expect(container.querySelector('.url-link')).not.toBeNull());
    client.registry.addServer({
      ...client.registry.getServer('known')!,
      id: 'new',
      url: href,
      name: 'New server'
    });
    await vi.waitFor(() =>
      expect(container.querySelector('.entity-link')?.textContent).toContain('New server')
    );
    expect(container.querySelector('.url-link')).toBeNull();
  });

  it('navigates once, supports modifier clicks, and replaces links when a message is edited', async () => {
    const opened = vi.spyOn(window, 'open').mockReturnValue(null);
    const rendered = render(MessageContent, { body: origin });
    await vi.waitFor(() => expect(rendered.container.querySelector('.entity-link')).not.toBeNull());
    const link = rendered.container.querySelector<HTMLAnchorElement>('.entity-link')!;
    link.click();
    expect(mocks.goto).toHaveBeenCalledExactlyOnceWith('/chat/known.example.test');
    link.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, ctrlKey: true }));
    expect(opened).toHaveBeenCalledExactlyOnceWith(
      '/chat/known.example.test',
      '_blank',
      'noopener,noreferrer'
    );
    await rendered.rerender({ body: `${origin}/chat/-/${roomId}` });
    await vi.waitFor(() =>
      expect(rendered.container.querySelector('.entity-link bdi')?.textContent?.trim()).toBe(
        'General'
      )
    );
    expect(rendered.container.querySelectorAll('a')).toHaveLength(1);
    await rendered.rerender({ body: 'No links' });
    await vi.waitFor(() => expect(rendered.container.querySelectorAll('a')).toHaveLength(0));
  });

  it('opens the room menu instead of the message menu and clears it on access loss', async () => {
    const rendered = render(MessageContent, { body: `${origin}/chat/-/${roomId}` });
    const parentMenu = vi.fn();
    rendered.container.addEventListener('contextmenu', parentMenu);
    await vi.waitFor(() => expect(rendered.container.querySelector('.entity-link')).not.toBeNull());
    rendered.container
      .querySelector('.entity-link')!
      .dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() =>
      expect(document.querySelector('[data-testid="copy-room-id"]')).not.toBeNull()
    );
    expect(parentMenu).not.toHaveBeenCalled();
    client.registry.getStore('known').projection.rooms.delete(roomId);
    await vi.waitFor(() =>
      expect(document.querySelector('[data-testid="copy-room-id"]')).toBeNull()
    );
  });

  it.each([
    ['server', origin, 'Server information'],
    ['user', `${origin}/chat/-/user/${userId}`, 'Copy user ID']
  ])('opens the %s menu in the linked server scope', async (_kind, href, action) => {
    const { container } = render(MessageContent, { body: href });
    await vi.waitFor(() => expect(container.querySelector('.entity-link')).not.toBeNull());
    container
      .querySelector('.entity-link')!
      .dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(container.textContent).toContain(action));
  });

  it('keeps server menu links out of prose styling and copies the public server URL', async () => {
    const write = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
    const { container } = render(MessageContent, { body: origin });
    await vi.waitFor(() => expect(container.querySelector('.entity-link')).not.toBeNull());
    container.querySelector('.markdown-html')!.classList.add('prose');
    container
      .querySelector('.entity-link')!
      .dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(container.querySelector('a.menu-entry')).not.toBeNull());
    expect(getComputedStyle(container.querySelector('a.menu-entry')!).textDecorationLine).toBe(
      'none'
    );
    const copy = container.querySelector<HTMLButtonElement>('[data-testid="copy-server-link"]')!;
    expect(copy.textContent).toContain('Copy server link');
    copy.click();
    await vi.waitFor(() => expect(write).toHaveBeenCalledWith(`${origin}/`));
  });

  it.each([true, false])('respects the linked server DM permission (%s)', async (allowed) => {
    const store = client.registry.getStore('known');
    vi.spyOn(store, 'permissions', 'get').mockReturnValue({
      ...store.permissions,
      canStartDMs: allowed
    });
    const { container } = render(MessageContent, {
      body: `${origin}/chat/-/user/${userId}`
    });
    await vi.waitFor(() => expect(container.querySelector('.entity-link')).not.toBeNull());
    container
      .querySelector('.entity-link')!
      .dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(container.textContent).toContain('Copy user ID'));
    expect(container.textContent?.includes('Open conversation')).toBe(allowed);
  });

  it('falls back to a neutral chip when its server is removed', async () => {
    const { container } = render(MessageContent, {
      body: `${origin}/chat/-/user/${userId}`
    });
    await vi.waitFor(() => expect(container.querySelector('.entity-link')).not.toBeNull());
    client.registry.removeServer('known');
    await vi.waitFor(() =>
      expect(container.querySelector('.url-link')?.getAttribute('title')).toBe(
        `${origin}/chat/-/user/${userId}`
      )
    );
    expect(container.querySelector('.entity-link')).toBeNull();
  });
});

it('previews bare URLs in the visual composer, preserves labels, and edits the URL behind a pill', async () => {
  let api: ComposerEditorApi | undefined;
  let markdown = '';
  const { container } = render(TipTapEditor, {
    onUpdate: (value) => {
      markdown = value;
    },
    onReady: (ready) => {
      api = ready;
    }
  });
  await vi.waitFor(() => expect(api).toBeDefined());
  const source = `${origin} and [the wonderful channel](${origin}/chat/-/${roomId})`;
  api!.setContent(source);
  await vi.waitFor(() =>
    expect(container.querySelector('.entity-link')?.textContent).toContain('Known server')
  );
  expect(container.querySelectorAll('.entity-link')).toHaveLength(1);
  api!.focus('end');
  api!.insertText('!');
  expect(markdown).toBe(source + '!');
  const preview = container.querySelector<HTMLElement>('.composer-entity-preview')!;
  expect(preview.isConnected).toBe(true);
  preview.querySelector<HTMLAnchorElement>('a')!.click();
  await vi.waitFor(() => expect(container.querySelector('.composer-entity-preview')).toBeNull());
  expect(mocks.goto).not.toHaveBeenCalled();
  api!.insertText('edited');
  expect(api!.getText()).toContain('edited');
  api!.setContent(`[${origin}](${origin})`);
  await vi.waitFor(() => expect(container.querySelector('.entity-link')).toBeNull());
  api!.focus('end');
  api!.insertText('!');
  expect(markdown).toBe(`[${origin}](${origin})!`);
});

it('keeps typing after a known URL outside its link', async () => {
  let api: ComposerEditorApi | undefined;
  let markdown = '';
  const { container } = render(TipTapEditor, {
    onReady: (ready) => {
      api = ready;
    },
    onUpdate: (value) => {
      markdown = value;
    }
  });
  await vi.waitFor(() => expect(api).toBeDefined());
  api!.setContent(origin);
  api!.focus('end');
  await vi.waitFor(() =>
    expect(document.activeElement).toBe(container.querySelector('.ProseMirror'))
  );
  await userEvent.keyboard(' after');
  await vi.waitFor(() => expect(markdown).toBe(`${origin} after`));
  expect(api!.getText()).toBe(`${origin} after`);
});

it.each([
  [`${origin}/chat/-/${roomId}`, '.entity-link', 'General'],
  [`${origin}/chat/-/${roomId}/m/E123456789abcde`, '[data-message-link]', 'General'],
  [
    'https://unknown.example.test:8443/a/very/long/path?query=value#section',
    '.url-link',
    'unkno…xample.test:8443'
  ]
])(
  'previews pasted %s immediately and keeps the following text editable',
  async (href, selector, label) => {
    let api: ComposerEditorApi | undefined;
    let markdown = '';
    const { container } = render(TipTapEditor, {
      onReady: (ready) => {
        api = ready;
      },
      onUpdate: (value) => {
        markdown = value;
      }
    });
    await vi.waitFor(() => expect(api).toBeDefined());
    api!.focus('end');
    const editor = container.querySelector<HTMLElement>('.ProseMirror')!;
    await vi.waitFor(() => expect(document.activeElement).toBe(editor));
    const clipboardData = new DataTransfer();
    clipboardData.setData('text/plain', href);
    editor.dispatchEvent(
      new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData })
    );
    await vi.waitFor(() =>
      expect(
        container.querySelector(`.composer-entity-preview ${selector}`)?.textContent
      ).toContain(label)
    );
    expect(getComputedStyle(container.querySelector('.composer-entity-source')!).display).toBe(
      'none'
    );
    expect(markdown).toBe(href);
    await userEvent.keyboard(' after');
    await vi.waitFor(() => expect(markdown).toBe(`${href} after`));
  }
);
