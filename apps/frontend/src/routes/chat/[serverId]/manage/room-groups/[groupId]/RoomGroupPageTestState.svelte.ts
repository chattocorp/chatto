class RoomGroupPageTestState {
  serverId = $state('server-a');
  groupId = $state('group-a');

  reset(): void {
    this.serverId = 'server-a';
    this.groupId = 'group-a';
  }
}

export const roomGroupPageTestState = new RoomGroupPageTestState();

export const roomGroupTestPage = {
  get params() {
    return {
      serverId: roomGroupPageTestState.serverId,
      groupId: roomGroupPageTestState.groupId
    };
  }
};
