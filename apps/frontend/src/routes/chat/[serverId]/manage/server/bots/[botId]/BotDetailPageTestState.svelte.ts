class BotDetailPageTestState {
  botId = $state('bot-user-id');

  reset(): void {
    this.botId = 'bot-user-id';
  }
}

export const botDetailPageTestState = new BotDetailPageTestState();

export const botDetailTestPage = {
  get params() {
    return { botId: botDetailPageTestState.botId };
  }
};
