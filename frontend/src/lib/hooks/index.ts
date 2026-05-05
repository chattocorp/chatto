// GraphQL
export { useQuery } from './useQuery.svelte';
export type { UseQueryOptions, UseQueryReturn } from './useQuery.svelte';
export { useMutation } from './useMutation.svelte';
export type { UseMutationOptions, UseMutationReturn, MutationResult } from './useMutation.svelte';

// Instance events
export {
  useInstanceEvent,
  useActiveInstanceEvent,
  useUserProfileUpdate,
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
} from './useInstanceEvent.svelte';

// Space events
export { useSpaceEvent, usePresenceChange } from './useSpaceEvent.svelte';

// Message actions
export { useMessageActions } from './useMessageActions.svelte';
export type { MessageActionParams } from './useMessageActions.svelte';

// Data hooks
export { useRoomData } from './useRoomData.svelte';
export { useRoomMembersSync } from './useRoomMembersSync.svelte';
export { useRoomUnread } from './useRoomUnread.svelte';
export { useEffectiveSpaceId } from './useEffectiveSpaceId.svelte';

// Lifecycle hooks
export { useTabResumeCallback } from './useTabResumeCallback.svelte';
export { useReconnectCallback, useReconnectTrigger } from './useReconnectCallback.svelte';
export { createTypingIndicator } from './useTypingIndicator.svelte';
export type { TypingIndicator, TypingUser } from './useTypingIndicator.svelte';

// UI hooks
export { useVisualViewport } from './useVisualViewport.svelte';
export { usePinchZoomPrevention } from './usePinchZoomPrevention.svelte';
export { usePageTitle } from './usePageTitle.svelte';
