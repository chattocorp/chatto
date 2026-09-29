/**
 * Voice-call integration point of a server store.
 *
 * The client tracks the server's active calls in the realtime projection. It
 * does not include a media implementation. An application that joins calls
 * installs a {@link VoiceCallFactory}; each server store then creates one
 * controller and forwards call-related realtime facts and authorization
 * changes to it. Without a factory, stores use a controller that is never
 * connected.
 */

import type { VoiceCallAPI } from '../api/voiceCalls.js';
import type { Register } from '../register.js';

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

/**
 * The part of a voice-call implementation that a server store uses. Reads of
 * `connected`, `roomId`, and `participants` should be reactive, so that
 * computed call state updates.
 */
export interface VoiceCallController {
  /** Whether this client is connected to a call on this server. */
  readonly connected: boolean;
  /** The room of the connected call. */
  readonly roomId: string | null;
  /** Participants of the connected call, including the viewer. */
  readonly participants: readonly LiveCallParticipant[];
  /** Recheck call permissions after the viewer's authority changed. */
  reconcilePermissions(): Promise<void>;
  /** The viewer lost access to a room; leave its call. */
  handleRoomAccessRevoked(roomId: string): void;
  /** The server ended a call. */
  handleCallEndedEvent(roomId: string, callId: string | null): void;
  /** A participant joined or left a call. */
  handleParticipantTransition(transition: CallParticipantTransition): void;
  /** The realtime projection was reset; forget per-event call state. */
  handleProjectionReset(): void;
}

/** What a store gives a voice-call implementation. */
export interface VoiceCallContext {
  readonly serverId: string;
  readonly api: VoiceCallAPI;
  /** The viewer's current call permissions in a room. */
  permissions(roomId: string): CallPermissions;
}

/** Create the voice-call controller of one server store. */
export type VoiceCallFactory<Controller extends VoiceCallController = VoiceCallController> = (
  context: VoiceCallContext
) => Controller;

/** The application's voice-call type; see {@link Register}. */
export type RegisteredVoiceCall = Register extends {
  voiceCall: infer T extends VoiceCallController;
}
  ? T
  : VoiceCallController;

/** A controller for clients without call media. It is never connected. */
export class DetachedVoiceCall implements VoiceCallController {
  readonly connected: boolean = false;
  readonly roomId: string | null = null;
  readonly participants: readonly LiveCallParticipant[] = [];
  async reconcilePermissions(): Promise<void> {}
  handleRoomAccessRevoked(): void {}
  handleCallEndedEvent(): void {}
  handleParticipantTransition(): void {}
  handleProjectionReset(): void {}
}

/** The default factory: a client without call media. */
export const detachedVoiceCallFactory: VoiceCallFactory<DetachedVoiceCall> = () =>
  new DetachedVoiceCall();
