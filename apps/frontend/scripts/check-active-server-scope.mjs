import { readdir, readFile, stat } from 'node:fs/promises';
import path from 'node:path';

const root = process.cwd();
const scanRoots = [
	'src/routes/chat/[serverId]',
	'src/lib/RoomList.svelte',
	'src/lib/components/chat',
	'src/lib/components/composer',
	'src/lib/components/settings',
	'src/lib/components/voice',
	'src/lib/hooks'
];
const directPattern = /serverRegistry\.getStore\s*\(\s*getActiveServer\s*\(/;
const activeServerAliasPattern =
	/\b(?:const|let)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:\$derived\s*\(\s*)?getActiveServer\s*\(\s*\)\s*\)?/g;
const ignoredRuntimeNames = new Set([
	'RoomSidebarTestHarness.svelte',
	'VoiceCallPanelStoryHarness.svelte'
]);

async function* walk(dir) {
	const rootStat = await stat(dir);
	if (rootStat.isFile()) {
		yield dir;
		return;
	}

	for (const entry of await readdir(dir, { withFileTypes: true })) {
		const fullPath = path.join(dir, entry.name);
		if (entry.isDirectory()) {
			yield* walk(fullPath);
			continue;
		}
		if (!entry.isFile()) continue;
		if (!/\.(svelte|svelte\.[tj]s|[tj]s)$/.test(entry.name)) continue;
		if (entry.name.includes('.spec.')) continue;
		if (ignoredRuntimeNames.has(entry.name)) continue;
		yield fullPath;
	}
}

const violations = [];
for (const relativeRoot of scanRoots) {
	for await (const file of walk(path.join(root, relativeRoot))) {
		const source = await readFile(file, 'utf8');
		if (directPattern.test(source) || hasActiveServerAliasLookup(source)) {
			violations.push(path.relative(root, file));
		}
	}
}

function hasActiveServerAliasLookup(source) {
	const aliases = new Set();
	for (const match of source.matchAll(activeServerAliasPattern)) {
		aliases.add(match[1]);
	}
	for (const alias of aliases) {
		const escaped = alias.replace(/[\\^$.*+?()[\]{}|]/g, '\\$&');
		const lookupPattern = new RegExp(`serverRegistry\\.getStore\\s*\\(\\s*${escaped}\\s*\\)`);
		if (lookupPattern.test(source)) return true;
	}
	return false;
}

if (violations.length > 0) {
	console.error(
		[
			'Do not derive room subtree state with serverRegistry.getStore(getActiveServer()).',
			'Use useActiveServerScope() so the active server, segment, store, and connection share one tracked scope.',
			'',
			...violations.map((file) => `- ${file}`)
		].join('\n')
	);
	process.exit(1);
}
