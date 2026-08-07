import type {
  BotAPI,
  BotAccount,
  CreatedBot,
  CreateBotInput,
  UpdateBotInput
} from '$lib/api-client/bots';
import type { UserAPI, UserSummary } from '$lib/api-client/users';
import { SvelteSet } from 'svelte/reactivity';

const PAGE_SIZE = 20;
export type BotListScope = 'owned' | 'manageable';

export class BotManagementStore {
  bots = $state.raw<BotAccount[]>([]);
  owners = $state.raw<Record<string, UserSummary>>({});
  totalCount = $state(0);
  hasMore = $state(false);
  loading = $state(true);
  loadingMore = $state(false);
  error = $state<string | null>(null);

  #requestId = 0;

  constructor(
    private readonly getScope: () => BotListScope,
    private readonly getBotAPI: () => BotAPI,
    private readonly getUserAPI: () => UserAPI
  ) {}

  async load(): Promise<void> {
    const requestId = ++this.#requestId;
    this.loading = true;
    this.error = null;
    try {
      const page = await this.getBotAPI().listBots({
        limit: PAGE_SIZE,
        ownedByCallerOnly: this.getScope() === 'owned'
      });
      if (requestId !== this.#requestId) return;
      this.bots = page.bots;
      this.totalCount = page.totalCount;
      this.hasMore = page.hasMore;
      await this.#hydrateOwners(page.bots, requestId);
    } catch (error) {
      if (requestId === this.#requestId) this.error = message(error);
    } finally {
      if (requestId === this.#requestId) this.loading = false;
    }
  }

  async loadMore(): Promise<void> {
    if (this.loading || this.loadingMore || !this.hasMore) return;
    const requestId = ++this.#requestId;
    this.loadingMore = true;
    this.error = null;
    try {
      const page = await this.getBotAPI().listBots({
        limit: PAGE_SIZE,
        offset: this.bots.length,
        ownedByCallerOnly: this.getScope() === 'owned'
      });
      if (requestId !== this.#requestId) return;
      const seen = new SvelteSet(this.bots.map((bot) => bot.id));
      const additions = page.bots.filter((bot) => !seen.has(bot.id));
      this.bots = [...this.bots, ...additions];
      this.totalCount = page.totalCount;
      this.hasMore = page.hasMore;
      await this.#hydrateOwners(additions, requestId);
    } catch (error) {
      if (requestId === this.#requestId) this.error = message(error);
    } finally {
      if (requestId === this.#requestId) this.loadingMore = false;
    }
  }

  async create(input: CreateBotInput): Promise<CreatedBot> {
    const created = await this.getBotAPI().createBot(input);
    const bot = created.bot;
    this.bots = [bot, ...this.bots];
    this.totalCount += 1;
    await this.#hydrateOwners([bot], this.#requestId);
    return created;
  }

  async update(input: UpdateBotInput): Promise<BotAccount> {
    const bot = await this.getBotAPI().updateBot(input);
    this.bots = this.bots.map((item) => (item.id === bot.id ? bot : item));
    return bot;
  }

  replace(bot: BotAccount): void {
    this.bots = this.bots.map((item) => (item.id === bot.id ? bot : item));
  }

  owner(bot: BotAccount): UserSummary | null {
    return this.owners[bot.ownerId] ?? null;
  }

  async #hydrateOwners(bots: BotAccount[], requestId: number): Promise<void> {
    const ownerIds = [
      ...new SvelteSet(bots.map((bot) => bot.ownerId).filter((id) => !this.owners[id]))
    ];
    if (ownerIds.length === 0) return;
    const owners = await this.getUserAPI().batchGetUsers(ownerIds);
    if (requestId !== this.#requestId) return;
    this.owners = {
      ...this.owners,
      ...Object.fromEntries(owners.map((owner) => [owner.id, owner]))
    };
  }
}

function message(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
