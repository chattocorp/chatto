/** Per-message attachment limit from CreateMessageRequest in messages.proto. */
export const MAX_MESSAGE_ATTACHMENTS = 10;

/** Server policy for user attachment uploads. Byte limits do not depend on file type. */
export type AttachmentUploadLimits = {
  videoProcessingEnabled: boolean;
  maxUploadSize: number;
};

/**
 * Return the reason that a file cannot be uploaded, or null when it is allowed.
 * Hosts supply browser files or equivalent metadata and render their own messages.
 * The server must still validate the upload.
 */
export function attachmentUploadRejection(
  file: { type: string; size: number },
  limits: AttachmentUploadLimits
): 'video_disabled' | 'too_large' | null {
  if (file.type.startsWith('video/') && !limits.videoProcessingEnabled) {
    return 'video_disabled';
  }
  return file.size > limits.maxUploadSize ? 'too_large' : null;
}
