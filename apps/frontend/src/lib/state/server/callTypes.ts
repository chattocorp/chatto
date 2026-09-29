/**
 * Types shared by the frontend's voice-call state and its call overlays. The
 * Chatto client tracks each server's active calls in the realtime projection;
 * the frontend owns the media implementation.
 */

/** Resolved room actions. Missing permission data always denies access. */
export type CallPermissions = {
  start: boolean;
  join: boolean;
  voice: boolean;
  camera: boolean;
  screenshare: boolean;
};

export const NO_CALL_PERMISSIONS: CallPermissions = {
  start: false,
  join: false,
  voice: false,
  camera: false,
  screenshare: false
};

/** A participant of the call that this client is connected to. */
export interface LiveCallParticipant {
  /** The participant's user ID. */
  readonly identity: string;
  readonly isCameraEnabled: boolean;
  /** Present while the participant publishes a camera track. */
  readonly videoTrack?: unknown;
}

/** A realtime participant change in a room's call. */
export type CallParticipantTransition = {
  /** Realtime event ID, for deduplication. */
  eventId: string;
  kind: 'join' | 'leave';
  roomId: string;
  callId: string | null;
  /** The user who joined or left. */
  actorId: string | null;
  /** The viewer of the realtime projection. */
  viewerId: string | null;
};

/** The part of a call connection that call overlays read. Reads are reactive. */
export interface CallConnection {
  /** Whether this client is connected to a call on the server. */
  readonly connected: boolean;
  /** The room of the connected call. */
  readonly roomId: string | null;
  /** Participants of the connected call, including the viewer. */
  readonly participants: readonly LiveCallParticipant[];
}
