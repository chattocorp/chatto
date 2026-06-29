import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AdminDiagnosticsService } from "@chatto/api-types/admin/v1/diagnostics_connect";

export type AdminDiagnosticsAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type AdminSystemInfo = {
  connection: AdminConnectionInfo;
  account: AdminAccountInfo;
  nats: AdminNatsStats;
  stats: AdminServerStats;
  projections: AdminProjectionState[];
};

export type AdminConnectionInfo = {
  connected: boolean;
  serverId: string;
  serverName: string;
  version: string;
  maxPayload: number;
  rtt: string;
};

export type AdminAccountInfo = {
  memory: number;
  memoryUsed: number;
  storage: number;
  storageUsed: number;
  streams: number;
  streamsUsed: number;
  consumers: number;
  consumersUsed: number;
};

export type AdminServerStats = {
  userCount: number;
  channelRoomCount: number;
  dmRoomCount: number;
};

export type AdminNatsStats = {
  totalMessages: number;
  totalBytes: number;
  totalConsumerPending: number;
  totalAckPending: number;
  streams: AdminNatsStreamInfo[];
  consumers: AdminNatsConsumerInfo[];
};

export type AdminNatsStreamInfo = {
  name: string;
  description: string;
  subjects: string[];
  storage: string;
  messages: number;
  bytes: number;
  firstSequence: string;
  lastSequence: string;
  consumerCount: number;
  replicas: number;
  clusterLeader: string;
};

export type AdminNatsConsumerInfo = {
  stream: string;
  name: string;
  durable: string;
  filterSubject: string;
  filterSubjects: string[];
  ackPolicy: string;
  pullBased: boolean;
  pushBound: boolean;
  pending: number;
  ackPending: number;
  redelivered: number;
  waiting: number;
  deliveredConsumerSequence: string;
  deliveredStreamSequence: string;
  ackFloorConsumerSequence: string;
  ackFloorStreamSequence: string;
};

export type AdminProjectionState = {
  key: string;
  name: string;
  subjects: string[];
  started: boolean;
  startupDurationSeconds: number | null;
  lastAppliedSequence: string;
  matchingStreamSequence: string;
  streamLastSequence: string;
  lag: number;
  failed: boolean;
  failedSequence: string;
  failure: string;
  entryCount: number;
  estimatedBytes: number;
  averageEntryBytes: number;
  metrics: AdminProjectionMetric[];
};

export type AdminProjectionMetric = {
  name: string;
  value: number;
  bytes: number;
};

function adminDiagnosticsClient(config: AdminDiagnosticsAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const client = createClient(AdminDiagnosticsService, transport);
  const headers = config.bearerToken
    ? { Authorization: `Bearer ${config.bearerToken}` }
    : undefined;
  return { client, headers };
}

export async function getAdminSystemInfo(
  config: AdminDiagnosticsAPIConfig,
): Promise<AdminSystemInfo> {
  const { client, headers } = adminDiagnosticsClient(config);
  const response = await client.getSystemInfo({}, { headers });
  const systemInfo = response.systemInfo;

  return {
    connection: {
      connected: systemInfo?.connection?.connected ?? false,
      serverId: systemInfo?.connection?.serverId ?? "",
      serverName: systemInfo?.connection?.serverName ?? "",
      version: systemInfo?.connection?.version ?? "",
      maxPayload: Number(systemInfo?.connection?.maxPayload ?? 0),
      rtt: systemInfo?.connection?.rtt ?? "",
    },
    account: {
      memory: Number(systemInfo?.account?.memory ?? 0),
      memoryUsed: Number(systemInfo?.account?.memoryUsed ?? 0),
      storage: Number(systemInfo?.account?.storage ?? 0),
      storageUsed: Number(systemInfo?.account?.storageUsed ?? 0),
      streams: systemInfo?.account?.streams ?? 0,
      streamsUsed: systemInfo?.account?.streamsUsed ?? 0,
      consumers: systemInfo?.account?.consumers ?? 0,
      consumersUsed: systemInfo?.account?.consumersUsed ?? 0,
    },
    nats: {
      totalMessages: Number(systemInfo?.nats?.totalMessages ?? 0),
      totalBytes: Number(systemInfo?.nats?.totalBytes ?? 0),
      totalConsumerPending: Number(systemInfo?.nats?.totalConsumerPending ?? 0),
      totalAckPending: systemInfo?.nats?.totalAckPending ?? 0,
      streams: (systemInfo?.nats?.streams ?? []).map((stream) => ({
        name: stream.name,
        description: stream.description,
        subjects: [...stream.subjects],
        storage: stream.storage,
        messages: Number(stream.messages),
        bytes: Number(stream.bytes),
        firstSequence: stream.firstSequence,
        lastSequence: stream.lastSequence,
        consumerCount: stream.consumerCount,
        replicas: stream.replicas,
        clusterLeader: stream.clusterLeader,
      })),
      consumers: (systemInfo?.nats?.consumers ?? []).map((consumer) => ({
        stream: consumer.stream,
        name: consumer.name,
        durable: consumer.durable,
        filterSubject: consumer.filterSubject,
        filterSubjects: [...consumer.filterSubjects],
        ackPolicy: consumer.ackPolicy,
        pullBased: consumer.pullBased,
        pushBound: consumer.pushBound,
        pending: Number(consumer.pending),
        ackPending: consumer.ackPending,
        redelivered: consumer.redelivered,
        waiting: consumer.waiting,
        deliveredConsumerSequence: consumer.deliveredConsumerSequence,
        deliveredStreamSequence: consumer.deliveredStreamSequence,
        ackFloorConsumerSequence: consumer.ackFloorConsumerSequence,
        ackFloorStreamSequence: consumer.ackFloorStreamSequence,
      })),
    },
    stats: {
      userCount: systemInfo?.stats?.userCount ?? 0,
      channelRoomCount: systemInfo?.stats?.channelRoomCount ?? 0,
      dmRoomCount: systemInfo?.stats?.dmRoomCount ?? 0,
    },
    projections: response.projections.map((projection) => ({
      key: projection.key,
      name: projection.name,
      subjects: [...projection.subjects],
      started: projection.started,
      startupDurationSeconds: projection.startupDurationSeconds ?? null,
      lastAppliedSequence: projection.lastAppliedSequence,
      matchingStreamSequence: projection.matchingStreamSequence,
      streamLastSequence: projection.streamLastSequence,
      lag: Number(projection.lag),
      failed: projection.failed,
      failedSequence: projection.failedSequence,
      failure: projection.failure,
      entryCount: Number(projection.entryCount),
      estimatedBytes: Number(projection.estimatedBytes),
      averageEntryBytes: Number(projection.averageEntryBytes),
      metrics: projection.metrics.map((metric) => ({
        name: metric.name,
        value: Number(metric.value),
        bytes: Number(metric.bytes),
      })),
    })),
  };
}
