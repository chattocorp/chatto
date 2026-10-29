import { toast } from '$lib/ui/toast';
import { m } from '$lib/i18n/messages';
import { prepareFiles } from '$lib/attachments/prepareFiles';
import {
  attachmentUploadRejection,
  MAX_MESSAGE_ATTACHMENTS,
  type AttachmentUploadLimits
} from '@chatto/client/util/attachmentUpload';

export type FileWithUrl = { file: File; url: string; description: string };

export type AttachmentLimits = AttachmentUploadLimits;

export function formatFileSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(0)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${bytes} bytes`;
}

export class AttachmentsState {
  filesWithUrls = $state<FileWithUrl[]>([]);
  pendingCount = $state(0);
  // Preparation results belong to the draft in which they started.
  private preparationGeneration = 0;

  constructor(private readonly getLimits: () => AttachmentLimits) {}

  get selectedFiles(): File[] {
    return this.filesWithUrls.map((f) => f.file);
  }

  get descriptions() {
    return this.filesWithUrls.flatMap(({ file, description }) =>
      description.trim() ? [{ file, description }] : []
    );
  }

  restore(files: FileWithUrl[]): void {
    this.preparationGeneration++;
    this.pendingCount = 0;
    this.filesWithUrls = files;
  }

  validateFiles(files: File[]): File[] {
    const limits = this.getLimits();
    const accepted: File[] = [];
    for (const file of files) {
      const rejection = attachmentUploadRejection(file, limits);
      if (rejection === 'video_disabled') {
        toast.error(m('composer.upload.video_disabled'));
        continue;
      }

      const limit = limits.maxUploadSize;
      if (rejection === 'too_large') {
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
      MAX_MESSAGE_ATTACHMENTS - this.filesWithUrls.length - this.pendingCount
    );
    const eligibleFiles = this.validateFiles(files);
    const validFiles = eligibleFiles.slice(0, available);
    if (eligibleFiles.length > available) {
      toast.error(m('composer.upload.too_many', { max: MAX_MESSAGE_ATTACHMENTS }));
    }
    if (validFiles.length === 0) return;

    const generation = this.preparationGeneration;
    // Reserve slots before asynchronous conversion so concurrent selections share the cap.
    this.pendingCount += validFiles.length;
    try {
      const prepared = await prepareFiles(validFiles);
      if (generation !== this.preparationGeneration) return;
      if (prepared.length > 0) {
        this.filesWithUrls = [...this.filesWithUrls, ...this.filesToPreviewItems(prepared)];
      }
    } catch (err) {
      if (generation !== this.preparationGeneration) return;
      console.error('Error preparing attachment files:', err);
      toast.error(m('composer.upload.prepare_failed'));
    } finally {
      if (generation === this.preparationGeneration) {
        this.pendingCount -= validFiles.length;
      }
    }
  }

  removeFile(index: number): void {
    const removed = this.filesWithUrls[index];
    if (removed) URL.revokeObjectURL(removed.url);
    this.filesWithUrls = this.filesWithUrls.filter((_, i) => i !== index);
  }

  setDescription(index: number, description: string): void {
    const attachment = this.filesWithUrls[index];
    if (!attachment) return;
    attachment.description = description.trim();
  }

  clear(): void {
    this.preparationGeneration++;
    this.pendingCount = 0;
    for (const { url } of this.filesWithUrls) {
      URL.revokeObjectURL(url);
    }
    this.filesWithUrls = [];
  }
}
