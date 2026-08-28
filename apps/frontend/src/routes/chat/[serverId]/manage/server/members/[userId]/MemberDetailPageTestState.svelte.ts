class MemberDetailPageTestState {
  serverId = $state('server-1');
  sessionId = $state('session-1');
  userId = $state('alice');
  /** Authenticated viewer id used by pages reading the current user. */
  viewerId = $state('viewer');

  reset(): void {
    this.serverId = 'server-1';
    this.sessionId = 'session-1';
    this.userId = 'alice';
    this.viewerId = 'viewer';
  }
}

export const memberDetailPageTestState = new MemberDetailPageTestState();

export const memberDetailTestPage = {
  get params() {
    return {
      serverId: memberDetailPageTestState.serverId,
      userId: memberDetailPageTestState.userId
    };
  }
};
