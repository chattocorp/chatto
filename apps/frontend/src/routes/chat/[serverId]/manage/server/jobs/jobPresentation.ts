/** Localized labels and tones that the job list and job detail pages share. */
import { JobStatus, type AdminJob } from '$lib/api/adminJobs';
import { m } from '$lib/i18n/messages';
/** Refresh interval of visible job pages. Job state has no realtime events. */
export const JOBS_POLL_INTERVAL_MS = 3000;

/**
 * Returns the attempt count, followed by the automatic attempt limit when the
 * job has one. A job without a limit retries until it succeeds or fails
 * permanently, so only its count is shown.
 */
export function jobAttemptsLabel(job: AdminJob, locale: string): string {
  const attempts = job.attempts.toLocaleString(locale);
  return job.maxAttempts === undefined
    ? attempts
    : `${attempts} / ${job.maxAttempts.toLocaleString(locale)}`;
}

/** Pill tones that job statuses use. */
export type JobStatusTone = 'neutral' | 'action' | 'success' | 'danger';

/** Returns the label of a registered job type, or null for a future type. */
export function knownJobTypeLabel(type: string): string | null {
  switch (type) {
    case 'webhook':
      return m('admin.jobs.webhook');
    case 'media':
      return m('admin.jobs.media');
    case 'asset_cleanup':
      return m('admin.jobs.asset_cleanup');
    case 'call_key_cleanup':
      return m('admin.jobs.call_key_cleanup');
    case 'push_subscription_cleanup':
      return m('admin.jobs.push_subscription_cleanup');
    case 'user_key_shredding':
      return m('admin.jobs.user_key_shredding');
    case 'notification_alert':
      return m('admin.jobs.notification_alert');
    case 'email':
      return m('admin.jobs.email');
    default:
      return null;
  }
}

/** Returns the label of the type of a job. Future types show as unknown. */
export function jobTypeLabel(job: AdminJob): string {
  return knownJobTypeLabel(job.type) ?? m('common.unknown');
}

/** Returns the label of a job execution status. */
export function jobStatusLabel(status: JobStatus): string {
  switch (status) {
    case JobStatus.QUEUED:
      return m('admin.jobs.queued');
    case JobStatus.RUNNING:
      return m('admin.jobs.running');
    case JobStatus.SUCCEEDED:
      return m('admin.jobs.succeeded');
    case JobStatus.FAILED:
      return m('admin.jobs.failed');
    default:
      return m('common.unknown');
  }
}

/** Returns the pill tone of a job execution status. */
export function jobStatusTone(status: JobStatus): JobStatusTone {
  switch (status) {
    case JobStatus.RUNNING:
      return 'action';
    case JobStatus.SUCCEEDED:
      return 'success';
    case JobStatus.FAILED:
      return 'danger';
    default:
      return 'neutral';
  }
}
