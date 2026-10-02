import type { RegisteredServer } from '@chatto/client/server/registry';
import { startRemoteReauthentication } from '$lib/auth/reauth';
import { m } from '$lib/i18n/messages';
import { toast } from '$lib/ui/toast';

/**
 * The **Log in to this server** action for registered remote servers. Each
 * control that offers the action owns one instance. The instance runs at most
 * one sign-in at a time and shows a toast when sign-in fails.
 */
export class RemoteSignIn {
  /** ID of the server whose sign-in runs, or `null`. */
  pendingServerId = $state<string | null>(null);

  /** Whether a sign-in runs. */
  get pending(): boolean {
    return this.pendingServerId !== null;
  }

  /**
   * Start sign-in to `server`. Call this synchronously from the user's action,
   * so the browser allows the sign-in window. Does nothing while a sign-in
   * runs.
   */
  async start(server: RegisteredServer): Promise<void> {
    if (this.pending) return;
    this.pendingServerId = server.id;
    try {
      await startRemoteReauthentication(server);
    } catch {
      toast.error(m('add_server.start_failed'));
    } finally {
      this.pendingServerId = null;
    }
  }
}
