import { toast } from '$lib/ui/toast';
import { m } from '$lib/i18n/messages';
import { compressVideo } from '$lib/attachments/compressVideo';
import { prepareFiles } from '$lib/attachments/prepareFiles';
import {
  attachmentUploadRejection,
  MAX_MESSAGE_ATTACHMENTS,
  type AttachmentUploadLimits
} from '@chatto/client/util/attachmentUpload';

/** Preparation shares the attachment tile and must finish before sending. */
export type VideoPreparation = {
  phase: 'queued' | 'compressing' | 'failed';
  progress: number;
  controller: AbortController;
};

export type FileWithUrl = {
  file: File;
  url: string;
  description: string;
  /** Absent once the file is ready for upload. Failed preparations remain removable. */
  preparation?: VideoPreparation;
};

export type AttachmentLimits = AttachmentUploadLimits;

export function formatFileSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(0)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${bytes} bytes`;
}

export class AttachmentsState {
  filesWithUrls = $state<FileWithUrl[]>([]);
  private preparingCount = $state(0);
  // Prepared tiles already occupy slots. Failed tiles also block send until removed.
  pendingCount = $derived(
    this.preparingCount + this.filesWithUrls.filter((item) => item.preparation).length
  );
  private videoQueue: Promise<void> = Promise.resolve();
  // Preparation results belong to the draft in which they started.
  private preparationGeneration = 0;

  constructor(private readonly getLimits: () => AttachmentLimits) {}

  get selectedFiles(): File[] {
    return this.filesWithUrls.filter((item) => !item.preparation).map((item) => item.file);
  }

  get descriptions() {
    return this.filesWithUrls.flatMap(({ file, description, preparation }) =>
      !preparation && description.trim() ? [{ file, description }] : []
    );
  }

  restore(files: FileWithUrl[]): void {
    this.preparationGeneration++;
    this.cancelVideoPreparation();
    this.preparingCount = 0;
    this.filesWithUrls = files;
  }

  validateFiles(files: File[], allowPreparation = false): File[] {
    const limits = this.getLimits();
    const accepted: File[] = [];
    for (const file of files) {
      const rejection = attachmentUploadRejection(file, limits);
      if (rejection === 'video_disabled') {
        toast.error(m('composer.upload.video_disabled'));
        continue;
      }

      const limit = limits.maxUploadSize;
      if (rejection === 'too_large' && !(allowPreparation && file.type.startsWith('video/'))) {
        toast.error(
          m('composer.upload.too_large', {
            filename: file.name,
            size: formatFileSize(file.size),
            limit: formatFileSize(limit)
          })
        );
      } else {
        accepted.push(file);
      }
    }
    return accepted;
  }

  filesToPreviewItems(files: File[]): FileWithUrl[] {
    return files.map((file) => ({
      file,
      url: URL.createObjectURL(file),
      description: ''
    }));
  }

  async stageFiles(files: File[]): Promise<void> {
    const available = Math.max(
      0,
      MAX_MESSAGE_ATTACHMENTS - this.filesWithUrls.length - this.preparingCount
    );
    const eligibleFiles = this.validateFiles(files, true);
    const selected = eligibleFiles.slice(0, available);
    if (eligibleFiles.length > available) {
      toast.error(m('composer.upload.too_many', { max: MAX_MESSAGE_ATTACHMENTS }));
    }
    if (selected.length === 0) return;

    const generation = this.preparationGeneration;
    const ordinaryFiles: File[] = [];
    const work: Promise<void>[] = [];
    for (const file of selected) {
      if (file.type.startsWith('video/') && file.size > this.getLimits().maxUploadSize) {
        this.filesWithUrls.push({
          file,
          url: URL.createObjectURL(file),
          description: '',
          preparation: { phase: 'queued', progress: 0, controller: new AbortController() }
        });
        // Use the reactive item so progress and the replacement file update the same tile.
        const item = this.filesWithUrls[this.filesWithUrls.length - 1];
        const preparation = item.preparation!;
        this.videoQueue = this.videoQueue.then(() =>
          this.prepareVideo(item, preparation, generation)
        );
        work.push(this.videoQueue);
      } else {
        ordinaryFiles.push(file);
      }
    }
    // Reserve slots before awaiting HEIC conversion or queued video work.
    this.preparingCount += ordinaryFiles.length;
    if (ordinaryFiles.length) work.push(this.prepareOrdinaryFiles(ordinaryFiles, generation));
    await Promise.all(work);
  }

  private async prepareOrdinaryFiles(files: File[], generation: number): Promise<void> {
    try {
      const prepared = await prepareFiles(files);
      if (generation !== this.preparationGeneration) return;
      this.filesWithUrls = [...this.filesWithUrls, ...this.filesToPreviewItems(prepared)];
    } catch {
      if (generation === this.preparationGeneration)
        toast.error(m('composer.upload.prepare_failed'));
    } finally {
      if (generation === this.preparationGeneration) this.preparingCount -= files.length;
    }
  }

  /** Release codec work when the composer unmounts or changes drafts. Stashed tiles show failure. */
  cancelVideoPreparation(): void {
    for (const item of this.filesWithUrls) {
      if (item.preparation) {
        item.preparation.controller.abort();
        item.preparation.phase = 'failed';
      }
    }
  }

  /** Serialize local encoding and replace only the upload file, preserving tile identity and description. */
  private async prepareVideo(
    item: FileWithUrl,
    preparation: VideoPreparation,
    generation: number
  ): Promise<void> {
    const signal = preparation.controller.signal;
    if (signal.aborted || generation !== this.preparationGeneration) return;
    preparation.phase = 'compressing';
    try {
      const limits = this.getLimits();
      if (!limits.videoProcessingEnabled) {
        toast.error(m('composer.upload.video_disabled'));
        preparation.phase = 'failed';
        return;
      }
      const file = await compressVideo(item.file, {
        maxBytes: limits.maxUploadSize,
        signal,
        onProgress: (fraction) => {
          if (!signal.aborted) preparation.progress = fraction;
        }
      });
      if (signal.aborted || generation !== this.preparationGeneration) return;
      if (this.validateFiles([file]).length === 0) {
        preparation.phase = 'failed';
        return;
      }
      item.file = file;
      // Keep the source preview URL so the tile and video element do not remount.
      item.preparation = undefined;
    } catch {
      if (!signal.aborted && generation === this.preparationGeneration) {
        preparation.phase = 'failed';
        toast.error(m('composer.upload.compress_failed'));
      }
    }
  }

  removeFile(index: number): void {
    const removed = this.filesWithUrls[index];
    if (removed) {
      removed.preparation?.controller.abort();
      URL.revokeObjectURL(removed.url);
    }
    this.filesWithUrls = this.filesWithUrls.filter((_, i) => i !== index);
  }

  setDescription(index: number, description: string): void {
    const attachment = this.filesWithUrls[index];
    if (!attachment) return;
    attachment.description = description.trim();
  }

  clear(): void {
    this.preparationGeneration++;
    this.cancelVideoPreparation();
    this.preparingCount = 0;
    for (const { url } of this.filesWithUrls) {
      URL.revokeObjectURL(url);
    }
    this.filesWithUrls = [];
  }
}
