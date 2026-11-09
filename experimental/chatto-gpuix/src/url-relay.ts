/** Temporary macOS URL handler for the experiment's built-in desktop OAuth identity. */
import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
const exec = promisify(execFile);
const launchServices =
  '/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister';
const desktopCallback = 'chatto://desktop/servers/callback?mode=popup';

async function handler(operation: 'read' | 'set', id = ''): Promise<string> {
  const script = `ObjC.import('CoreServices'); function run(args) {
    if (args[0] === 'set') { if ($.LSSetDefaultHandlerForURLScheme($('chatto'), $(args[1])) !== 0) throw Error('URL handler registration failed'); return ''; }
    const value = $.LSCopyDefaultHandlerForURLScheme($('chatto')); return value ? ObjC.unwrap(ObjC.castRefToObject(value)) : '';
  }`;
  return (
    await exec('/usr/bin/osascript', ['-l', 'JavaScript', '-e', script, operation, id])
  ).stdout.trim();
}

/** Register a one-attempt relay. Callback URLs travel in an HTTP body, never shell arguments or files. */
export async function registerDesktopRelay(
  endpoint: string
): Promise<{ redirectUri: string; close: () => Promise<void> }> {
  if (process.platform !== 'darwin')
    throw new Error('Desktop URL sign-in currently requires macOS');
  const cache = join(homedir(), 'Library', 'Caches', 'Chatto GPUix');
  await mkdir(cache, { recursive: true, mode: 0o700 });
  const directory = await mkdtemp(join(cache, 'oauth-relay-'));
  const app = join(directory, 'Chatto GPUix Sign-in.app');
  const id = `run.chatto.gpuix.relay.${crypto.randomUUID()}`;
  const previous = await handler('read');
  let registered = false;
  const close = async () => {
    try {
      if (registered && (await handler('read')) === id && previous) await handler('set', previous);
      if (registered) {
        await exec(launchServices, ['-u', app]);
        await exec('/usr/bin/osascript', [
          '-l',
          'JavaScript',
          '-e',
          "ObjC.import('AppKit'); function run(args) { const apps = $.NSRunningApplication.runningApplicationsWithBundleIdentifier($(args[0])); for (let i = 0; i < apps.count; i++) apps.objectAtIndex(i).forceTerminate; }",
          id
        ]);
      }
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  };
  try {
    // AppleScriptObjC sends the event argument directly through Foundation.
    const source = join(directory, 'relay.applescript');
    await writeFile(
      source,
      `use framework "Foundation"
use scripting additions
on open location incoming
set targetURL to current application's NSURL's URLWithString:${JSON.stringify(endpoint)}
set request to current application's NSMutableURLRequest's requestWithURL:targetURL
request's setHTTPMethod:"POST"
request's setTimeoutInterval:5
set bodyString to current application's NSString's stringWithString:incoming
request's setHTTPBody:(bodyString's dataUsingEncoding:(current application's NSUTF8StringEncoding))
current application's NSURLConnection's sendSynchronousRequest:request returningResponse:(missing value) |error|:(missing value)
end open location
`,
      { mode: 0o600 }
    );
    await exec('/usr/bin/osacompile', ['-s', '-o', app, source]);
    const plist = join(app, 'Contents', 'Info.plist');
    await exec('/usr/bin/plutil', ['-replace', 'CFBundleIdentifier', '-string', id, plist]);
    await exec('/usr/bin/plutil', ['-insert', 'LSUIElement', '-bool', 'YES', plist]);
    await exec('/usr/bin/plutil', [
      '-insert',
      'CFBundleURLTypes',
      '-json',
      JSON.stringify([{ CFBundleURLName: id, CFBundleURLSchemes: ['chatto'] }]),
      plist
    ]);
    await exec('/usr/bin/plutil', [
      '-insert',
      'NSAppTransportSecurity',
      '-json',
      '{"NSAllowsArbitraryLoads":true}',
      plist
    ]);
    await exec('/usr/bin/codesign', ['--force', '--sign', '-', app]);
    await exec(launchServices, ['-f', app]);
    registered = true;
    await exec('/usr/bin/open', ['-g', '-a', app]);
    await handler('set', id);
    return { redirectUri: desktopCallback, close };
  } catch {
    await close();
    throw new Error('Could not register the experimental desktop callback');
  }
}
