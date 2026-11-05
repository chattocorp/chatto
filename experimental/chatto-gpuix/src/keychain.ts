/** macOS Keychain bridge. Key material is passed on stdin, never in arguments or logs. */
import { execFile } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
const script = String.raw`ObjC.import('Foundation');
ObjC.import('Security');
function run() {
  const input = JSON.parse(ObjC.unwrap($.NSString.alloc.initWithDataEncoding($.NSFileHandle.fileHandleWithStandardInput.readDataToEndOfFile, $.NSUTF8StringEncoding)));
  const query = $.NSMutableDictionary.alloc.init;
  query.setObjectForKey(ObjC.castRefToObject($.kSecClassGenericPassword), ObjC.castRefToObject($.kSecClass));
  query.setObjectForKey($(input.service), ObjC.castRefToObject($.kSecAttrService));
  query.setObjectForKey($(input.account), ObjC.castRefToObject($.kSecAttrAccount));
  if (input.operation === 'create') {
    query.setObjectForKey($(input.value).dataUsingEncoding($.NSUTF8StringEncoding), ObjC.castRefToObject($.kSecValueData));
    const status = $.SecItemAdd(query, null);
    if (status !== 0) throw new Error('Keychain write failed');
    return '';
  }
  if (input.operation === 'delete') { $.SecItemDelete(query); return ''; }
  query.setObjectForKey(ObjC.castRefToObject($.kCFBooleanTrue), ObjC.castRefToObject($.kSecReturnData));
  const result = Ref();
  const status = $.SecItemCopyMatching(query, result);
  if (status === -25300) return '';
  if (status !== 0) throw new Error('Keychain read failed');
  return ObjC.unwrap($.NSString.alloc.initWithDataEncoding(ObjC.castRefToObject(result[0]), $.NSUTF8StringEncoding));
}
`;

/** Load or create the state encryption key. Never replace a missing key for an existing vault. */
export async function stateEncryptionKey(directory: string, exists: boolean): Promise<Buffer> {
  if (process.platform !== 'darwin')
    throw new Error('Native session storage currently requires macOS Keychain');
  const account = createHash('sha256').update(directory).digest('hex');
  const invoke = (operation: string, value?: string) =>
    new Promise<string>((resolve, reject) => {
      const child = execFile(
        '/usr/bin/osascript',
        ['-l', 'JavaScript', '-e', script],
        { timeout: 30_000, maxBuffer: 4096 },
        (error, stdout) => {
          if (error) reject(new Error('Could not access the native session key in Keychain'));
          else resolve(stdout.trim());
        }
      );
      child.stdin?.end(
        JSON.stringify({ operation, service: 'run.chatto.gpuix.state.v1', account, value })
      );
    });
  let value = await invoke('read');
  if (!value) {
    if (exists) throw new Error('The saved session encryption key is missing from Keychain');
    value = randomBytes(32).toString('base64');
    await invoke('create', value);
  }
  const key = Buffer.from(value, 'base64');
  if (key.length !== 32) throw new Error('The native session key is invalid');
  return key;
}
