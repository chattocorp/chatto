const REQUIRED_MEDIA_USAGE_DESCRIPTIONS = [
  "NSMicrophoneUsageDescription",
  "NSCameraUsageDescription",
  "NSAudioCaptureUsageDescription",
  "NSBluetoothAlwaysUsageDescription",
  "NSBluetoothPeripheralUsageDescription",
] as const;

/** Ensure macOS will prompt for WebRTC devices instead of terminating the app. */
export async function assertMacOSMediaUsageDescriptions(
  plistPath: string,
): Promise<void> {
  const plist = await Deno.readTextFile(plistPath);
  const missing = REQUIRED_MEDIA_USAGE_DESCRIPTIONS.filter(
    (key) => !plist.includes(`<key>${key}</key>`),
  );
  if (missing.length > 0) {
    throw new Error(
      `macOS app metadata is missing media usage descriptions: ${
        missing.join(", ")
      }`,
    );
  }
}
