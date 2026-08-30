import { Code, ConnectError } from '@connectrpc/connect';
import {
  getPublicNeighbors,
  getPublicServerInfo,
  type PublicNeighbor,
  type PublicServerInfo
} from '$lib/api-client/server';

/** Hard page-session limits for public Neighbor discovery. */
export const SERVER_DIRECTORY_LIMITS = {
  concurrency: 6,
  timeoutMs: 10_000,
  mutualHopDepth: 2,
  candidateBatchSize: 12,
  directoryRequests: 48,
  profileRequests: 24,
  totalRequests: 72,
  candidates: 120,
  neighborsPerSource: 100,
  testimonialLength: 500
} as const;

export type ServerProfileEntry = {
  origin: string;
  profile: PublicServerInfo | null;
};

export type ServerDirectoryEntry = ServerProfileEntry & {
  /** Canonical origins of the servers whose recommendations are shown. */
  sourceOrigins: string[];
  /** Ordered recommendations, including source-specific testimonials. */
  recommendations: ServerDirectoryRecommendation[];
};

export type ServerDirectoryRecommendation = {
  sourceOrigin: string;
  testimonial: string | null;
};

/** Progressive state published by one page-session discovery crawl. */
export type ServerDirectorySnapshot = {
  entries: ServerDirectoryEntry[];
  failedSourceCount: number;
  failedCandidateCount: number;
  nonMutualCandidateCount: number;
  sourceCount: number;
  activeRequestCount: number;
  queuedCandidateCount: number;
  directoryRequestCount: number;
  profileRequestCount: number;
  totalRequestCount: number;
  isStarted: boolean;
  isLoading: boolean;
  isInitialLoading: boolean;
  isPaused: boolean;
  canLoadMore: boolean;
  sessionLimitReached: boolean;
};

type DirectoryLoadOptions = {
  signal?: AbortSignal;
  initiallyVisible?: boolean;
  /** Test override. Production sessions use the fixed ten-second limit. */
  requestTimeoutMs?: number;
  listNeighbors?: typeof getPublicNeighbors;
  getServerInfo?: typeof getPublicServerInfo;
};

type ProfileLoadOptions = Pick<DirectoryLoadOptions, 'signal' | 'getServerInfo'>;
type DirectoryListener = (snapshot: ServerDirectorySnapshot) => void;
type DirectoryStatus = 'queued' | 'loaded' | 'failed' | 'unsupported';
type ProfileStatus = 'queued' | 'loaded' | 'failed';

type OrderedRecommendation = ServerDirectoryRecommendation & {
  targetOrigin: string;
  order: number;
};

type Candidate = {
  origin: string;
  sourceOrigin: string;
};

type ScheduledRequest<T> = {
  operation: () => Promise<T>;
  resolve: (value: T) => void;
  reject: (reason: unknown) => void;
};

/** Convert an advertised URL to a canonical HTTP(S) origin. */
export function canonicalServerOrigin(value: string): string | null {
  try {
    const url = new URL(value);
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || url.username || url.password) {
      return null;
    }
    return url.origin;
  } catch {
    return null;
  }
}

/** Convert a hostname or HTTP(S) URL entered by a person to its origin. */
export function serverOriginFromInput(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed) && !/^https?:\/\//i.test(trimmed)) return null;
  return canonicalServerOrigin(/^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`);
}

/** Load public profiles without making one failed server hide the others. */
export async function loadServerProfiles(
  origins: readonly string[],
  options: ProfileLoadOptions = {}
): Promise<ServerProfileEntry[]> {
  const getServerInfo = options.getServerInfo ?? getPublicServerInfo;
  const profiles = await mapWithConcurrency(
    origins,
    SERVER_DIRECTORY_LIMITS.concurrency,
    (origin) => getServerInfo(origin, { signal: discoverySignal(options.signal) }).catch(() => null)
  );
  throwIfAborted(options.signal);

  return origins.map((origin, index) => ({ origin, profile: profiles[index] ?? null }));
}

/**
 * Create one bounded, cancellable Neighbor discovery session. The session
 * fetches each canonical directory and profile at most once and publishes a
 * new immutable snapshot whenever useful progress occurs.
 */
export function createServerDirectoryDiscovery(
  registeredOrigins: readonly string[],
  options: DirectoryLoadOptions = {}
): ServerDirectoryDiscovery {
  return new ServerDirectoryDiscovery(registeredOrigins, options);
}

/** A bounded progressive crawl of mutually recommending Chatto servers. */
export class ServerDirectoryDiscovery {
  private readonly listNeighbors: typeof getPublicNeighbors;
  private readonly getServerInfo: typeof getPublicServerInfo;
  private readonly requestTimeoutMs: number;
  private readonly controller = new AbortController();
  private readonly scheduler: RequestScheduler;
  private readonly listeners = new Set<DirectoryListener>();
  private readonly sourceOrigins: string[];
  private readonly rootOrigins: Set<string>;
  private readonly directoryStatuses = new Map<string, DirectoryStatus>();
  private readonly profileStatuses = new Map<string, ProfileStatus>();
  private readonly outgoingByOrigin = new Map<string, OrderedRecommendation[]>();
  private readonly incomingByOrigin = new Map<string, Map<string, OrderedRecommendation>>();
  private readonly recommendationsByOrigin = new Map<
    string,
    Map<string, OrderedRecommendation>
  >();
  private readonly mutualOrigins = new Set<string>();
  private readonly edgeOutcomes = new Map<string, 'verified' | 'non-mutual'>();
  private readonly depths = new Map<string, number>();
  private readonly expansionSources = new Set<string>();
  private readonly sourceCandidateCursors = new Map<string, number>();
  private readonly sourcesWithQueuedCandidate = new Set<string>();
  private readonly exhaustedSources = new Set<string>();
  private readonly candidateQueue: Candidate[] = [];
  private readonly candidateOrigins = new Set<string>();
  private readonly profiles = new Map<string, PublicServerInfo>();
  private entries: ServerDirectoryEntry[] = [];
  private failedSourceCount = 0;
  private failedCandidateCount = 0;
  private nonMutualCandidateCount = 0;
  private directoryRequestCount = 0;
  private profileRequestCount = 0;
  private recommendationOrder = 0;
  private candidateCredits = 0;
  private started = false;
  private cancelled = false;
  private sessionLimitReached = false;
  private pumping = false;
  private readonly idleWaiters = new Set<{
    resolve: () => void;
    reject: (reason: unknown) => void;
  }>();

  constructor(registeredOrigins: readonly string[], options: DirectoryLoadOptions = {}) {
    this.listNeighbors = options.listNeighbors ?? getPublicNeighbors;
    this.getServerInfo = options.getServerInfo ?? getPublicServerInfo;
    this.requestTimeoutMs = options.requestTimeoutMs ?? SERVER_DIRECTORY_LIMITS.timeoutMs;
    this.sourceOrigins = [
      ...new Set(
        registeredOrigins.flatMap((value) => {
          const origin = canonicalServerOrigin(value);
          return origin ? [origin] : [];
        })
      )
    ];
    this.rootOrigins = new Set(this.sourceOrigins);
    for (const origin of this.sourceOrigins) this.depths.set(origin, 0);

    if (options.signal) {
      if (options.signal.aborted) {
        this.controller.abort(options.signal.reason);
        this.cancelled = true;
      } else {
        options.signal.addEventListener('abort', () => this.cancel(options.signal?.reason), {
          once: true
        });
      }
    }

    this.scheduler = new RequestScheduler(
      SERVER_DIRECTORY_LIMITS.concurrency,
      () => this.schedulerChanged(),
      options.initiallyVisible === false
    );
  }

  /** Receive the current snapshot immediately and after each state change. */
  subscribe(listener: DirectoryListener): () => void {
    this.listeners.add(listener);
    listener(this.snapshot());
    return () => this.listeners.delete(listener);
  }

  /** Start root discovery and grant the single automatic candidate batch. */
  start(): void {
    if (this.started || this.cancelled) return;
    this.started = true;
    this.candidateCredits = SERVER_DIRECTORY_LIMITS.candidateBatchSize;

    for (const sourceOrigin of this.sourceOrigins) {
      if (!this.scheduleDirectory(sourceOrigin)) {
        this.sessionLimitReached = true;
        break;
      }
    }
    this.pumpCandidates();
    this.emit();
    this.settleIdleWaiters();
  }

  /** Spend one more fixed-size candidate batch when the current work is idle. */
  loadMore(): void {
    if (!this.snapshot().canLoadMore || this.cancelled) return;
    this.candidateCredits += SERVER_DIRECTORY_LIMITS.candidateBatchSize;
    this.pumpCandidates();
    this.emit();
  }

  /** Pause or resume the launch of queued browser requests. */
  setVisible(visible: boolean): void {
    if (this.cancelled) return;
    this.scheduler.setPaused(!visible);
    if (visible) this.pumpCandidates();
    this.emit();
  }

  /** Abort active work and discard every queued request. */
  cancel(reason: unknown = new DOMException('Cancelled', 'AbortError')): void {
    if (this.cancelled) return;
    this.cancelled = true;
    this.controller.abort(reason);
    this.scheduler.cancel(reason);
    for (const waiter of this.idleWaiters) waiter.reject(reason);
    this.idleWaiters.clear();
  }

  /** Wait until the currently granted work and all resulting profiles settle. */
  whenIdle(): Promise<void> {
    if (this.cancelled) return Promise.reject(this.controller.signal.reason);
    if (this.isIdle()) return Promise.resolve();
    return new Promise<void>((resolve, reject) => this.idleWaiters.add({ resolve, reject }));
  }

  private scheduleDirectory(origin: string): boolean {
    if (this.cancelled || this.directoryStatuses.has(origin)) return true;
    if (!this.hasDirectoryBudget()) return false;

    this.directoryStatuses.set(origin, 'queued');
    this.directoryRequestCount += 1;
    void this.scheduler
      .schedule(async () => {
        try {
          const neighbors = await this.listNeighbors(origin, {
            signal: discoverySignal(this.controller.signal, this.requestTimeoutMs)
          });
          return { status: 'loaded' as const, neighbors };
        } catch (error) {
          if (this.controller.signal.aborted) throw this.controller.signal.reason;
          if (error instanceof ConnectError && error.code === Code.Unimplemented) {
            return { status: 'unsupported' as const, neighbors: [] };
          }
          return { status: 'failed' as const, neighbors: [] };
        }
      })
      .then((result) => {
        if (this.cancelled) return;
        if (result.status === 'failed') {
          this.directoryStatuses.set(origin, 'failed');
          if (this.rootOrigins.has(origin)) this.failedSourceCount += 1;
          else this.failedCandidateCount += 1;
        } else {
          this.directoryStatuses.set(origin, result.status);
          this.processDirectory(origin, result.neighbors);
        }
        this.emit();
      })
      .catch(() => {
        // Session cancellation owns the error and must not publish partial state.
      });
    return true;
  }

  private processDirectory(origin: string, advertisedNeighbors: readonly PublicNeighbor[]): void {
    const outgoing: OrderedRecommendation[] = [];
    const seenTargets = new Set<string>();
    for (const advertised of advertisedNeighbors.slice(
      0,
      SERVER_DIRECTORY_LIMITS.neighborsPerSource
    )) {
      const targetOrigin = canonicalServerOrigin(advertised.origin);
      if (!targetOrigin || targetOrigin === origin || seenTargets.has(targetOrigin)) continue;
      seenTargets.add(targetOrigin);
      const recommendation = {
        ...recommendationFrom(origin, advertised),
        targetOrigin,
        order: this.recommendationOrder++
      };
      outgoing.push(recommendation);
      let incoming = this.incomingByOrigin.get(targetOrigin);
      if (!incoming) {
        incoming = new Map();
        this.incomingByOrigin.set(targetOrigin, incoming);
      }
      incoming.set(origin, recommendation);
    }
    this.outgoingByOrigin.set(origin, outgoing);

    if (this.rootOrigins.has(origin)) {
      for (const recommendation of outgoing) {
        this.recordRecommendation(recommendation.targetOrigin, origin, recommendation);
      }
    }

    for (const sourceOrigin of this.incomingByOrigin.get(origin)?.keys() ?? []) {
      this.evaluateEdge(sourceOrigin, origin);
    }
    this.activateSource(origin);
  }

  private evaluateEdge(sourceOrigin: string, targetOrigin: string): void {
    if (!this.isEligibleSource(sourceOrigin)) return;
    const edgeKey = `${sourceOrigin}\n${targetOrigin}`;
    if (this.edgeOutcomes.has(edgeKey)) return;
    const targetStatus = this.directoryStatuses.get(targetOrigin);
    if (targetStatus !== 'loaded' && targetStatus !== 'unsupported') return;

    const reverseExists = this.outgoingByOrigin
      .get(targetOrigin)
      ?.some((recommendation) => recommendation.targetOrigin === sourceOrigin);
    if (!reverseExists) {
      this.edgeOutcomes.set(edgeKey, 'non-mutual');
      this.nonMutualCandidateCount += 1;
      return;
    }

    const recommendation = this.incomingByOrigin.get(targetOrigin)?.get(sourceOrigin);
    if (!recommendation) return;
    this.edgeOutcomes.set(edgeKey, 'verified');
    this.mutualOrigins.add(targetOrigin);
    this.recordRecommendation(targetOrigin, sourceOrigin, recommendation);

    const sourceDepth = this.depths.get(sourceOrigin);
    if (sourceDepth !== undefined) {
      const depth = Math.min(sourceDepth + 1, SERVER_DIRECTORY_LIMITS.mutualHopDepth);
      const priorDepth = this.depths.get(targetOrigin);
      if (priorDepth === undefined || depth < priorDepth) this.depths.set(targetOrigin, depth);
    }

    this.activateSource(targetOrigin);
  }

  private recordRecommendation(
    targetOrigin: string,
    sourceOrigin: string,
    recommendation: OrderedRecommendation
  ): void {
    let recommendations = this.recommendationsByOrigin.get(targetOrigin);
    if (!recommendations) {
      recommendations = new Map();
      this.recommendationsByOrigin.set(targetOrigin, recommendations);
    }
    recommendations.set(sourceOrigin, recommendation);
    this.scheduleProfile(targetOrigin);
    this.refreshEntry(targetOrigin);
  }

  private activateSource(origin: string): void {
    if (!this.isEligibleSource(origin)) return;
    for (const recommendation of this.outgoingByOrigin.get(origin) ?? []) {
      this.evaluateEdge(origin, recommendation.targetOrigin);
    }
    if (this.canExpand(origin)) this.addExpansionSource(origin);
  }

  private addExpansionSource(sourceOrigin: string): void {
    if (this.expansionSources.has(sourceOrigin)) return;
    this.expansionSources.add(sourceOrigin);
    this.sourceCandidateCursors.set(sourceOrigin, 0);
    this.refillSourceCandidate(sourceOrigin);
    this.pumpCandidates();
  }

  private refillSourceCandidate(sourceOrigin: string): void {
    if (
      this.sourcesWithQueuedCandidate.has(sourceOrigin) ||
      this.exhaustedSources.has(sourceOrigin) ||
      this.candidateOrigins.size >= SERVER_DIRECTORY_LIMITS.candidates
    ) {
      if (this.candidateOrigins.size >= SERVER_DIRECTORY_LIMITS.candidates) {
        this.sessionLimitReached = true;
      }
      return;
    }

    const outgoing = this.outgoingByOrigin.get(sourceOrigin) ?? [];
    let cursor = this.sourceCandidateCursors.get(sourceOrigin) ?? 0;
    while (cursor < outgoing.length) {
      const recommendation = outgoing[cursor++]!;
      const targetOrigin = recommendation.targetOrigin;
      this.sourceCandidateCursors.set(sourceOrigin, cursor);
      if (this.directoryStatuses.has(targetOrigin) || this.candidateOrigins.has(targetOrigin)) {
        this.evaluateEdge(sourceOrigin, targetOrigin);
        continue;
      }
      this.candidateOrigins.add(targetOrigin);
      this.candidateQueue.push({ origin: targetOrigin, sourceOrigin });
      this.sourcesWithQueuedCandidate.add(sourceOrigin);
      return;
    }
    this.exhaustedSources.add(sourceOrigin);
  }

  private pumpCandidates(): void {
    if (this.pumping || this.cancelled || this.scheduler.isPaused) return;
    this.pumping = true;
    try {
      while (
        this.candidateCredits > 0 &&
        this.candidateQueue.length > 0 &&
        this.scheduler.availableSlots > 0
      ) {
        if (!this.hasDirectoryBudget()) {
          this.sessionLimitReached = true;
          break;
        }
        const candidate = this.candidateQueue.shift()!;
        this.sourcesWithQueuedCandidate.delete(candidate.sourceOrigin);
        this.refillSourceCandidate(candidate.sourceOrigin);
        this.candidateCredits -= 1;
        this.scheduleDirectory(candidate.origin);
      }
    } finally {
      this.pumping = false;
    }
  }

  private scheduleProfile(origin: string): void {
    if (this.profileStatuses.has(origin) || this.cancelled) return;
    if (!this.hasProfileBudget()) {
      this.sessionLimitReached = true;
      return;
    }
    this.profileStatuses.set(origin, 'queued');
    this.profileRequestCount += 1;
    void this.scheduler
      .schedule(() =>
        this.getServerInfo(origin, {
          signal: discoverySignal(this.controller.signal, this.requestTimeoutMs)
        })
      )
      .then((profile) => {
        if (this.cancelled) return;
        this.profileStatuses.set(origin, 'loaded');
        this.profiles.set(origin, profile);
        this.refreshEntry(origin);
        this.emit();
      })
      .catch(() => {
        if (this.cancelled) return;
        this.profileStatuses.set(origin, 'failed');
        this.emit();
      });
  }

  private refreshEntry(origin: string): void {
    const profile = this.profiles.get(origin);
    const availableRecommendations = this.recommendationsByOrigin.get(origin);
    if (!profile || !availableRecommendations?.size) return;
    const recommendations = [...availableRecommendations.values()]
      .sort((left, right) => left.order - right.order)
      .map(({ sourceOrigin, testimonial }) => ({ sourceOrigin, testimonial }));
    const entry: ServerDirectoryEntry = {
      origin,
      profile,
      sourceOrigins: recommendations.map(({ sourceOrigin }) => sourceOrigin),
      recommendations
    };
    const index = this.entries.findIndex((candidate) => candidate.origin === origin);
    if (index === -1) this.entries = [...this.entries, entry];
    else this.entries = this.entries.with(index, entry);
  }

  private canExpand(origin: string): boolean {
    const depth = this.depths.get(origin);
    if (depth === undefined || depth >= SERVER_DIRECTORY_LIMITS.mutualHopDepth) return false;
    return this.isEligibleSource(origin);
  }

  private isEligibleSource(origin: string): boolean {
    return this.rootOrigins.has(origin) || this.mutualOrigins.has(origin);
  }

  private hasDirectoryBudget(): boolean {
    return (
      this.directoryRequestCount < SERVER_DIRECTORY_LIMITS.directoryRequests &&
      this.totalRequestCount() < SERVER_DIRECTORY_LIMITS.totalRequests
    );
  }

  private hasProfileBudget(): boolean {
    return (
      this.profileRequestCount < SERVER_DIRECTORY_LIMITS.profileRequests &&
      this.totalRequestCount() < SERVER_DIRECTORY_LIMITS.totalRequests
    );
  }

  private schedulerChanged(): void {
    if (this.cancelled) return;
    this.pumpCandidates();
    this.emit();
    this.settleIdleWaiters();
  }

  private isIdle(): boolean {
    if (!this.started) return true;
    const candidateCanRun =
      !this.scheduler.isPaused &&
      this.candidateCredits > 0 &&
      this.candidateQueue.length > 0 &&
      this.hasDirectoryBudget();
    return (
      this.scheduler.activeCount === 0 && this.scheduler.pendingCount === 0 && !candidateCanRun
    );
  }

  private settleIdleWaiters(): void {
    if (!this.isIdle()) return;
    for (const waiter of this.idleWaiters) waiter.resolve();
    this.idleWaiters.clear();
  }

  private totalRequestCount(): number {
    return this.directoryRequestCount + this.profileRequestCount;
  }

  private snapshot(): ServerDirectorySnapshot {
    const isLoading =
      this.started && (this.scheduler.activeCount > 0 || this.scheduler.pendingCount > 0);
    const canLoadMore =
      this.started &&
      !isLoading &&
      !this.scheduler.isPaused &&
      this.candidateCredits === 0 &&
      this.candidateQueue.length > 0 &&
      this.hasDirectoryBudget();
    return {
      entries: this.entries,
      failedSourceCount: this.failedSourceCount,
      failedCandidateCount: this.failedCandidateCount,
      nonMutualCandidateCount: this.nonMutualCandidateCount,
      sourceCount: Math.min(this.sourceOrigins.length, SERVER_DIRECTORY_LIMITS.directoryRequests),
      activeRequestCount: this.scheduler.activeCount,
      queuedCandidateCount: this.candidateQueue.length,
      directoryRequestCount: this.directoryRequestCount,
      profileRequestCount: this.profileRequestCount,
      totalRequestCount: this.totalRequestCount(),
      isStarted: this.started,
      isLoading,
      isInitialLoading: isLoading && this.entries.length === 0,
      isPaused: this.scheduler.isPaused,
      canLoadMore,
      sessionLimitReached: this.sessionLimitReached
    };
  }

  private emit(): void {
    const snapshot = this.snapshot();
    for (const listener of this.listeners) listener(snapshot);
  }
}

class RequestScheduler {
  private readonly queue: ScheduledRequest<unknown>[] = [];
  private active = 0;
  private paused: boolean;
  private cancelledReason: unknown;

  constructor(
    private readonly concurrency: number,
    private readonly onStateChange: () => void,
    initiallyPaused: boolean
  ) {
    this.paused = initiallyPaused;
  }

  get activeCount(): number {
    return this.active;
  }

  get pendingCount(): number {
    return this.queue.length;
  }

  get availableSlots(): number {
    return Math.max(0, this.concurrency - this.active - this.queue.length);
  }

  get isPaused(): boolean {
    return this.paused;
  }

  schedule<T>(operation: () => Promise<T>): Promise<T> {
    if (this.cancelledReason !== undefined) return Promise.reject(this.cancelledReason);
    const promise = new Promise<T>((resolve, reject) => {
      this.queue.push({ operation, resolve, reject } as ScheduledRequest<unknown>);
    });
    this.drain();
    this.onStateChange();
    return promise;
  }

  setPaused(paused: boolean): void {
    if (this.paused === paused) return;
    this.paused = paused;
    if (!paused) this.drain();
    this.onStateChange();
  }

  cancel(reason: unknown): void {
    this.cancelledReason = reason;
    for (const request of this.queue.splice(0)) request.reject(reason);
    this.onStateChange();
  }

  private drain(): void {
    while (!this.paused && this.active < this.concurrency && this.queue.length > 0) {
      const request = this.queue.shift()!;
      this.active += 1;
      void request
        .operation()
        .then(request.resolve, request.reject)
        .finally(() => {
          this.active -= 1;
          this.drain();
          this.onStateChange();
        });
    }
  }
}

function recommendationFrom(
  sourceOrigin: string,
  advertised: PublicNeighbor
): ServerDirectoryRecommendation {
  return { sourceOrigin, testimonial: boundedTestimonial(advertised.testimonial) };
}

function boundedTestimonial(value: string | null): string | null {
  const trimmed = value?.trim();
  if (!trimmed) return null;
  let bounded = '';
  let length = 0;
  for (const character of trimmed) {
    if (length === SERVER_DIRECTORY_LIMITS.testimonialLength) break;
    bounded += character;
    length += 1;
  }
  return bounded;
}

function discoverySignal(
  parent?: AbortSignal,
  timeoutMs: number = SERVER_DIRECTORY_LIMITS.timeoutMs
): AbortSignal {
  const timeout = AbortSignal.timeout(timeoutMs);
  return parent ? AbortSignal.any([parent, timeout]) : timeout;
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw signal.reason;
}

async function mapWithConcurrency<T, R>(
  values: readonly T[],
  concurrency: number,
  operation: (value: T) => Promise<R>
): Promise<R[]> {
  const results = new Array<R>(values.length);
  let nextIndex = 0;

  async function worker() {
    while (nextIndex < values.length) {
      const index = nextIndex++;
      results[index] = await operation(values[index]!);
    }
  }

  await Promise.all(Array.from({ length: Math.min(concurrency, values.length) }, () => worker()));
  return results;
}
