class MemberDetailPageTestState {
  serverId = $state('server-1');
  sessionId = $state('session-1');
  userId = $state('alice');

  reset(): void {
    this.serverId = 'server-1';
    this.sessionId = 'session-1';
    this.userId = 'alice';
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
