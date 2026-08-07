import assert from "node:assert/strict";
import { assertMacOSMediaUsageDescriptions } from "./macos_privacy.ts";

Deno.test("accepts all macOS WebRTC usage descriptions", async () => {
  const plist = await writePlist(`
    <key>NSMicrophoneUsageDescription</key><string>Microphone</string>
    <key>NSCameraUsageDescription</key><string>Camera</string>
    <key>NSAudioCaptureUsageDescription</key><string>Audio</string>
    <key>NSBluetoothAlwaysUsageDescription</key><string>Bluetooth</string>
    <key>NSBluetoothPeripheralUsageDescription</key><string>Bluetooth</string>
  `);
  try {
    await assertMacOSMediaUsageDescriptions(plist);
  } finally {
    await Deno.remove(plist);
  }
});

Deno.test("rejects incomplete macOS WebRTC usage descriptions", async () => {
  const plist = await writePlist(
    "<key>NSCameraUsageDescription</key><string>Camera</string>",
  );
  try {
    await assert.rejects(
      () => assertMacOSMediaUsageDescriptions(plist),
      /NSMicrophoneUsageDescription/,
    );
  } finally {
    await Deno.remove(plist);
  }
});

async function writePlist(contents: string): Promise<string> {
  const path = await Deno.makeTempFile({ suffix: ".plist" });
  await Deno.writeTextFile(path, contents);
  return path;
}
