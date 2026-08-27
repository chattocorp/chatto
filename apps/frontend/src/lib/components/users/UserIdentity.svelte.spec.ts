import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync, tick } from 'svelte';
import { q } from '$lib/test-utils';
import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
import UserIdentity from './UserIdentity.svelte';

vi.mock('$lib/navigation', () => ({
	serverIdToSegment: (serverId: string) => serverId
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
	useServerScope: () => ({
		serverId: 'server-1',
		store: {
			permissions: {
				loaded: true,
				canAdminViewUsers: false
			}
		}
	})
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
	getLiveDisplayName: (_userId: string, fallback: string) => fallback,
	getLiveLogin: (_userId: string, fallback: string) => fallback,
	getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
	getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
	getPresenceCache: () => ({ get: (_scope: unknown, fallback: unknown) => fallback })
}));

vi.mock('$lib/utils/inputCapabilities', () => ({
	prefersTouchActions: () => false,
	supportsHoverActions: () => true
}));

const user = {
	id: 'owner-1',
	login: 'alice',
	displayName: 'Alice Example',
	deleted: false,
	avatarUrl: null,
	presenceStatus: PresenceStatus.OFFLINE
};

const userContextMenuLoader = async () => ({ default: UserContextMenu });

let originalShowPopover: typeof HTMLElement.prototype.showPopover;
let originalShowModal: typeof HTMLDialogElement.prototype.showModal;

beforeAll(() => {
	originalShowPopover = HTMLElement.prototype.showPopover;
	originalShowModal = HTMLDialogElement.prototype.showModal;
	HTMLElement.prototype.showPopover = function showPopover() {
		this.setAttribute('popover-open', '');
	};
	HTMLDialogElement.prototype.showModal = function showModal() {
		this.setAttribute('open', '');
	};
});

afterAll(() => {
	HTMLElement.prototype.showPopover = originalShowPopover;
	HTMLDialogElement.prototype.showModal = originalShowModal;
});

describe('UserIdentity', () => {
	it('renders the shared avatar with the display name', () => {
		const { container } = render(UserIdentity, { props: { user, userContextMenuLoader } });

		expect(q(container, '[data-testid="user-identity"]')?.textContent).toContain('Alice Example');
		expect(q(container, '[role="img"][aria-label="alice"]')).toBeTruthy();
	});

	it('opens the shared user profile on right-click', async () => {
		const { container } = render(UserIdentity, { props: { user, userContextMenuLoader } });
		await tick();
		const identity = q(container, '[data-testid="user-identity"]')!;

		identity.dispatchEvent(
			new MouseEvent('contextmenu', { bubbles: true, clientX: 40, clientY: 60 })
		);
		await tick();

		await expect.element(q(container, '[role="dialog"]')).toBeInTheDocument();
		expect(q(container, '[role="dialog"]')?.textContent).toContain('Alice Example');
	});

	it('opens the shared user profile as a sheet after a touch long-press', async () => {
		const { container } = render(UserIdentity, { props: { user, userContextMenuLoader } });
		await tick();
		vi.useFakeTimers();
		try {
			const identity = q(container, '[data-testid="user-identity"]')!;

			identity.dispatchEvent(
				new PointerEvent('pointerdown', {
					bubbles: true,
					pointerId: 7,
					pointerType: 'touch',
					isPrimary: true,
					clientX: 40,
					clientY: 60
				})
			);
			await vi.advanceTimersByTimeAsync(500);
			flushSync();

			expect(q(container, 'dialog.bottom-sheet[open]')).toBeTruthy();
			expect(q(container, 'dialog')?.textContent).toContain('Alice Example');
		} finally {
			vi.useRealTimers();
		}
	});
});
