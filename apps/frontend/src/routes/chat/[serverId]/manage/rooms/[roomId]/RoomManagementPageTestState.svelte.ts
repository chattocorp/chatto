class RoomManagementPageTestState {
  serverId = $state('server-a');
  roomId = $state('shared-room');

  reset(): void {
    this.serverId = 'server-a';
    this.roomId = 'shared-room';
  }
}

export const roomManagementPageTestState = new RoomManagementPageTestState();

export const roomManagementTestPage = {
  get params() {
    return {
      serverId: roomManagementPageTestState.serverId,
      roomId: roomManagementPageTestState.roomId
    };
  }
};
