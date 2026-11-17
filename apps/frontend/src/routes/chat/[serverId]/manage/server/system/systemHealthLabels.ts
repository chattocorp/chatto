import { m } from '$lib/i18n/messages';
import type { HealthDetail } from './systemHealth';

/** Returns the localized text of a health check detail. */
export function healthDetailLabel(detail: HealthDetail): string {
  switch (detail.kind) {
    case 'unavailable':
      return m('admin.system.health.unavailable');
    case 'broker_connected':
      return m('admin.system.health.broker_connected', { version: detail.version || '-' });
    case 'broker_disconnected':
      return m('admin.system.health.broker_disconnected');
    case 'projections_failed':
      return m('admin.system.health.projections_failed_count', { count: detail.count });
    case 'projections_stalled':
      return m('admin.system.health.projections_stalled_count', { count: detail.count });
    case 'projections_catching_up':
      return m('admin.system.health.projections_catching_up_count', { count: detail.count });
    case 'projections_running':
      return m('admin.system.health.projections_running_count', { count: detail.count });
    case 'workers_failing':
      return m('admin.system.health.workers_failing_count', { count: detail.count });
    case 'workers_unconfirmed':
      return m('admin.system.health.workers_unconfirmed_count', { count: detail.count });
    case 'workers_running':
      return m('admin.system.health.workers_running_count', { count: detail.count });
    case 'backlog_waiting':
      return m('admin.system.health.backlog_waiting_count', { count: detail.count });
    case 'backlog_clear':
      return m('admin.system.health.backlog_clear');
    case 'storage_used':
      return m('admin.system.health.storage_used', { percent: detail.percent });
    case 'storage_unlimited':
      return m('admin.system.health.storage_unlimited');
    case 'livekit_connected':
      return m('admin.system.health.livekit_connected');
    case 'livekit_unreachable':
      return m('admin.system.health.livekit_unreachable');
    case 'livekit_unauthorized':
      return m('admin.system.health.livekit_unauthorized');
    case 'livekit_error':
      return m('admin.system.health.livekit_error');
    case 'livekit_insecure_url':
      return m('admin.system.health.livekit_insecure_url');
    case 'livekit_webhooks_rejected':
      return m('admin.system.health.livekit_webhooks_rejected');
    case 'search_none':
      return m('admin.system.health.search_none');
    case 'search_failed':
    case 'search_degraded':
    case 'search_indexing':
    case 'search_unknown':
    case 'search_ready':
    case 'search_index_stalled':
    case 'search_index_behind':
    case 'search_index_unknown':
    case 'search_index_current':
      return m(`admin.system.health.${detail.kind}`, {
        count: detail.count,
        total: detail.total
      });
  }
}
