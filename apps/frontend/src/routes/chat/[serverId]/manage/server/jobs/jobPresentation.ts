/** Localized labels and tones that the job list and job detail pages share. */
import { JobStatus, type AdminJob } from '$lib/api/adminJobs';
import { m } from '$lib/i18n/messages';
/** Refresh interval of visible job pages. Job state has no realtime events. */
export const JOBS_POLL_INTERVAL_MS = 3000;

/** Pill tones that job statuses use. */
export type JobStatusTone = 'neutral' | 'action' | 'success' | 'danger';

/** Returns the label of a registered job type. Future types show as unknown. */
export function jobTypeLabel(job: AdminJob): string {
  switch (job.type) {
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
    default:
      return m('common.unknown');
  }
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
