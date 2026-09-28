/**
 * Type registration for applications that extend the client.
 *
 * Augment {@link Register} to give client APIs the application's concrete
 * types. For example, an application with its own voice-call implementation
 * declares:
 *
 * ```ts
 * declare module '@chatto/client/register' {
 *   interface Register {
 *     voiceCall: MyVoiceCall;
 *   }
 * }
 * ```
 *
 * The registered types must match what the application installs at runtime,
 * for example with `setVoiceCallFactory`.
 */

// eslint-disable-next-line @typescript-eslint/no-empty-object-type -- augmentation target
export interface Register {}
