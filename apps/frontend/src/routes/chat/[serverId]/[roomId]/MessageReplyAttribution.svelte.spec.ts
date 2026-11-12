import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import { q } from '$lib/test-utils';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import MessageReplyAttribution from './MessageReplyAttribution.svelte';
import type { MessageReplyAttachmentSummary } from './messageEventModel';

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBotOwnerUserId: (_userId: string, fallback: string | null) => fallback,
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

const actor = {
  id: 'user-1',
  login: 'ada',
  displayName: 'Ada',
  avatarUrl: '',
  deleted: false,
  presenceStatus: PresenceStatus.ONLINE
};

describe('MessageReplyAttribution', () => {
  it('renders the active author, body, and call presence', async () => {
    const { container } = render(MessageReplyAttribution, {
      props: {
        preview: {
          name: 'Ada',
          body: 'Original message',
          attachmentSummary: null,
          actor,
          deleted: false
        },
        callPresence: 'video',
        onJump: vi.fn()
      }
    });

    await expect
      .element(q(container, '[data-testid="reply-attribution-author"]'))
      .toHaveTextContent('Ada');
    await expect
      .element(q(container, '[aria-label="in reply to Original message"]'))
      .toBeInTheDocument();
    await expect
      .element(q(container, '[data-testid="user-call-presence-video"]'))
      .toBeInTheDocument();
  });

  it('keeps author clicks separate from reply-target navigation', () => {
    const onJump = vi.fn();
    const onAuthorClick = vi.fn();
    const { container } = render(MessageReplyAttribution, {
      props: {
        preview: {
          name: 'Ada',
          body: 'Original message',
          attachmentSummary: null,
          actor,
          deleted: false
        },
        onJump,
        onAuthorClick
      }
    });

    q(container, '[data-testid="reply-attribution-author"]')!.click();

    expect(onAuthorClick).toHaveBeenCalledOnce();
    expect(onJump).not.toHaveBeenCalled();
  });

  it('uses the fallback body and deleted-user label when content is unavailable', async () => {
    const { container } = render(MessageReplyAttribution, {
      props: {
        preview: {
          name: 'Deleted user',
          body: null,
          attachmentSummary: null,
          actor: null,
          deleted: true
        },
        onJump: vi.fn()
      }
    });

    await expect.element(q(container, '[aria-label="in reply to Message"]')).toBeInTheDocument();
    expect(container.textContent).toContain('[deleted user]');
  });

  it.each([
    ['image', 'Image', 'icon-[uil--image]'],
    ['video', 'Video', 'icon-[uil--video]'],
    ['audio', 'Audio', 'icon-[uil--volume]'],
    ['file', 'File', 'icon-[uil--file]'],
    ['attachments', '3 attachments', 'icon-[uil--paperclip]']
  ] as const)('shows the %s label with a decorative icon', async (kind, label, iconClass) => {
    const { container } = render(MessageReplyAttribution, {
      props: {
        preview: {
          name: 'Ada',
          body: null,
          attachmentSummary: { kind, count: kind === 'attachments' ? 3 : 1 },
          actor,
          deleted: false
        },
        onJump: vi.fn()
      }
    });
    const button = q(container, `button[aria-label="in reply to ${label}"]`)!;
    await expect.element(button).toHaveTextContent(label);
    expect(button.getAttribute('title')).toBe(`in reply to ${label}`);
    expect(button.querySelector('[aria-hidden="true"]')?.classList.contains(iconClass)).toBe(true);
  });

  it.each([false, true])(
    'keeps image reply navigation accessible with compact=%s',
    async (compact) => {
      const onJump = vi.fn();
      const onAuthorClick = vi.fn();
      const { container } = render(MessageReplyAttribution, {
        props: {
          compact,
          preview: {
            name: 'Ada',
            body: null,
            attachmentSummary: { kind: 'image', count: 1 },
            actor,
            deleted: false
          },
          onJump,
          onAuthorClick
        }
      });
      const button = q(container, 'button[aria-label="in reply to Image"]')!;
      await userEvent.click(button);
      expect(onJump).toHaveBeenCalledTimes(1);
      button.focus();
      await userEvent.keyboard('{Enter}');
      expect(onJump).toHaveBeenCalledTimes(2);
      await userEvent.keyboard(' ');
      expect(onJump).toHaveBeenCalledTimes(3);
      await userEvent.click(q(container, '[data-testid="reply-attribution-author"]')!);
      expect(onAuthorClick).toHaveBeenCalledOnce();
      expect(onJump).toHaveBeenCalledTimes(3);
    }
  );

  it('updates the preview when attachment metadata or text changes', async () => {
    const preview = {
      name: 'Ada',
      body: null as string | null,
      attachmentSummary: { kind: 'image', count: 1 } as MessageReplyAttachmentSummary | null,
      actor,
      deleted: false
    };
    const rendered = render(MessageReplyAttribution, { props: { preview, onJump: vi.fn() } });
    await expect
      .element(q(rendered.container, '[aria-label="in reply to Image"]'))
      .toBeInTheDocument();
    await rendered.rerender({
      preview: { ...preview, attachmentSummary: { kind: 'attachments', count: 2 } }
    });
    await expect
      .element(q(rendered.container, '[aria-label="in reply to 2 attachments"]'))
      .toBeInTheDocument();
    await rendered.rerender({ preview: { ...preview, body: 'Caption', attachmentSummary: null } });
    const button = q(rendered.container, '[aria-label="in reply to Caption"]')!;
    await expect.element(button).toHaveTextContent('Caption');
    expect(button.querySelector('[aria-hidden="true"]')).toBeNull();
  });

  it('shows an image label even when its author is deleted', async () => {
    const { container } = render(MessageReplyAttribution, {
      props: {
        preview: {
          name: 'Deleted user',
          body: null,
          attachmentSummary: { kind: 'image', count: 1 },
          actor: null,
          deleted: true
        },
        onJump: vi.fn()
      }
    });
    await expect.element(q(container, '[aria-label="in reply to Image"]')).toBeInTheDocument();
    expect(container.textContent).toContain('[deleted user]');
  });

  it('keeps missing-target navigation available', async () => {
    const { container } = render(MessageReplyAttribution, {
      props: {
        preview: {
          name: 'a message',
          body: null,
          attachmentSummary: null,
          actor: null,
          deleted: false
        },
        onJump: vi.fn()
      }
    });
    await expect.element(q(container, '[aria-label="in reply to a message"]')).toBeInTheDocument();
  });
});
