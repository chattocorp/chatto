import { SvelteMap } from 'svelte/reactivity';
import type {
  AttachmentUploadUpdate,
  CreateMessageInput,
  CreateMessageResult,
  UpdateMessageInput
} from '$lib/api-client/messages';
import type { TimelineEventView } from '$lib/render/timelineEvents';
import type { MentionRolesStatus } from '$lib/state/server/mentionRoles.svelte';
import { extractMentions, hasRoleOrVirtualMention } from '$lib/mentions';
import { toast } from '$lib/ui/toast';
import { m } from '$lib/i18n/messages';

export type AttachmentSubmissionStatus =
  | { phase: 'preparing' }
  | { phase: 'uploading'; committedBytes: number; totalBytes: number }
  | { phase: 'uploaded' }
  | { phase: 'failed' };

export type PreparedPost = {
  draftKey: string;
  roomId: string;
  bodyToSend: string;
  filesToSend: File[] | null;
  attachmentAssetIds?: string[];
  threadRootEventId: string | null;
  inReplyTo: string | null;
  linkPreviewToken: string | null;
  alsoSendToChannel: boolean;
  createThread: boolean;
};

type MessageSubmissionAPI = {
  createMessage(input: CreateMessageInput): Promise<CreateMessageResult>;
  updateMessage(input: UpdateMessageInput): Promise<unknown>;
};

type ComposerSubmissionDependencies = {
  getAPI: () => MessageSubmissionAPI;
  getMentionRoleStatus: () => MentionRolesStatus;
  loadMentionRoles: () => Promise<boolean>;
  getMentionRoleNames: () => string[];
  onPostSuccess: (post: PreparedPost, event: TimelineEventView | null) => void;
  onEditSuccess: () => void;
};

/**
 * Owns the asynchronous create/edit lifecycle for one mounted composer.
 *
 * Draft and editor cleanup remain callbacks because they belong to the active
 * composer instance and must compare the submitted draft with the currently
 * visible room before clearing anything.
 */
export class ComposerSubmissionState {
  loading = $state(false);
  roleMentionCheckLoading = $state(false);
  roleMentionConfirmationLoading = $state(false);
  pendingRoleMentionConfirmation = $state<PreparedPost | null>(null);
  readonly attachmentStatuses = new SvelteMap<File, AttachmentSubmissionStatus>();

  readonly #dependencies: ComposerSubmissionDependencies;

  constructor(dependencies: ComposerSubmissionDependencies) {
    this.#dependencies = dependencies;
  }

  attachmentStatus(file: File): AttachmentSubmissionStatus | null {
    return this.attachmentStatuses.get(file) ?? null;
  }

  updateAttachmentStatus(update: AttachmentUploadUpdate): void {
    const status: AttachmentSubmissionStatus =
      update.phase === 'uploading'
        ? {
            phase: 'uploading',
            committedBytes: update.committedBytes,
            totalBytes: update.totalBytes
          }
        : { phase: update.phase };
    this.attachmentStatuses.set(update.file, status);
  }

  async requestPost(post: PreparedPost): Promise<void> {
    let rolesAvailable = this.#dependencies.getMentionRoleStatus() === 'ready';
    const roleStatus = this.#dependencies.getMentionRoleStatus();

    if (post.bodyToSend.includes('@') && roleStatus !== 'ready' && roleStatus !== 'failed') {
      this.roleMentionCheckLoading = true;
      try {
        rolesAvailable = await this.#dependencies.loadMentionRoles();
      } finally {
        this.roleMentionCheckLoading = false;
      }
    }

    if (post.bodyToSend && this.#mentionsRoleOrVirtualTarget(post, rolesAvailable)) {
      this.pendingRoleMentionConfirmation = post;
      return;
    }

    await this.#submitPost(post);
  }

  cancelRoleMentionConfirmation(): void {
    this.pendingRoleMentionConfirmation = null;
  }

  async confirmRoleMentionSend(): Promise<void> {
    const pendingPost = this.pendingRoleMentionConfirmation;
    if (!pendingPost || this.roleMentionConfirmationLoading) return;

    this.roleMentionConfirmationLoading = true;
    try {
      await this.#submitPost(pendingPost);
      this.pendingRoleMentionConfirmation = null;
    } finally {
      this.roleMentionConfirmationLoading = false;
    }
  }

  async editMessage(input: UpdateMessageInput): Promise<void> {
    this.loading = true;
    try {
      await this.#dependencies.getAPI().updateMessage(input);
      this.#dependencies.onEditSuccess();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : m('composer.edit_failed'));
    } finally {
      this.loading = false;
    }
  }

  #mentionsRoleOrVirtualTarget(post: PreparedPost, rolesAvailable: boolean): boolean {
    const hasKnownRoleOrVirtualMention = hasRoleOrVirtualMention(
      post.bodyToSend,
      this.#dependencies.getMentionRoleNames()
    );
    if (hasKnownRoleOrVirtualMention) return true;
    if (rolesAvailable) return false;

    return extractMentions(post.bodyToSend).length > 0;
  }

  async #submitPost(post: PreparedPost): Promise<void> {
    this.attachmentStatuses.clear();
    for (const file of post.filesToSend ?? []) {
      this.attachmentStatuses.set(file, { phase: 'preparing' });
    }
    this.loading = true;

    try {
      let response: CreateMessageResult;
      try {
        response = await this.#dependencies.getAPI().createMessage({
          roomId: post.roomId,
          body: post.bodyToSend,
          attachmentAssetIds: post.attachmentAssetIds,
          attachments: post.attachmentAssetIds?.length ? null : post.filesToSend,
          onAttachmentUploadUpdate: (update) => this.updateAttachmentStatus(update),
          threadRootEventId: post.threadRootEventId,
          inReplyTo: post.inReplyTo,
          linkPreviewToken: post.linkPreviewToken,
          alsoSendToChannel: post.alsoSendToChannel,
          createThread: post.createThread
        });
      } catch (error) {
        if (![...this.attachmentStatuses.values()].some((status) => status.phase === 'failed')) {
          this.attachmentStatuses.clear();
        }
        toast.error(m('composer.send_failed'));
        console.error('Error creating message:', error);
        return;
      }

      this.attachmentStatuses.clear();
      this.#dependencies.onPostSuccess(post, response.event);
    } finally {
      this.loading = false;
    }
  }
}

export function uploadPercentage(status: AttachmentSubmissionStatus): number | null {
  if (status.phase === 'uploaded') return 100;
  if (status.phase !== 'uploading' || status.totalBytes <= 0) return null;
  return Math.min(100, Math.round((status.committedBytes / status.totalBytes) * 100));
}
