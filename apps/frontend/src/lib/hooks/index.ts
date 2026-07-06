// Server events — unified bus from `myEvents` subscription.
export {
  useEvent,
  usePresenceChange,
  useActiveEvent,
  useUserProfileUpdate,
  useUserCustomStatusUpdate,
  useUserSettingsUpdate,
  useNotificationLevelChanged,
  useMention,
  useNewDM,
  useNotificationCreated,
  useNotificationDismissed,
  useRoomMarkedAsRead,
  useRoomLayoutUpdated,
  useSessionTerminated,
  useActiveRoomLayoutUpdated
} from './useEvent.svelte';

// Message actions
export { useMessageActions } from './useMessageActions.svelte';
export type { MessageActionParams } from './useMessageActions.svelte';

// Data hooks
export { useRoomData } from './useRoomData.svelte';
export { useRoomUnread } from './useRoomUnread.svelte';
export type { UnreadMarkerWindow } from './useRoomUnread.svelte';

// Lifecycle hooks
export { useTabResumeCallback } from './useTabResumeCallback.svelte';
export {
  useMayHaveMissedMessagesCallback,
  type MayHaveMissedMessagesReason
} from './useMayHaveMissedMessagesCallback.svelte';
export { useReconnectCallback, useReconnectTrigger } from './useReconnectCallback.svelte';
export type { ResumeSignal } from './resumeCoordinator.svelte';
export { createTypingIndicator } from './useTypingIndicator.svelte';
export type { TypingIndicator, TypingUser } from './useTypingIndicator.svelte';

// UI hooks
export { useVisualViewport } from './useVisualViewport.svelte';
export { usePinchZoomPrevention } from './usePinchZoomPrevention.svelte';
export { usePageTitle } from './usePageTitle.svelte';
