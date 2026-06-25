import { describe, it, expect, vi } from 'vitest';
import { prepareFiles } from './prepareFiles';

// Mock heic2any — dynamically imported, so we mock the module
vi.mock('heic2any', () => ({
	default: vi.fn(async ({ blob: _blob }: { blob: Blob }) => {
		// Return a fake JPEG blob
		return new Blob([new Uint8Array([0xff, 0xd8, 0xff])], { type: 'image/jpeg' });
	})
}));

function makeFile(name: string, type: string, size = 100): File {
	return new File([new Uint8Array(size)], name, { type });
}

describe('prepareFiles', () => {
	it('passes non-HEIC files through unchanged', async () => {
		const jpeg = makeFile('photo.jpg', 'image/jpeg');
		const png = makeFile('screenshot.png', 'image/png');
		const result = await prepareFiles([jpeg, png]);
		expect(result).toHaveLength(2);
		expect(result[0]).toBe(jpeg);
		expect(result[1]).toBe(png);
	});

	it('converts HEIC files to JPEG', async () => {
		const heic = makeFile('photo.heic', 'image/heic');
		const result = await prepareFiles([heic]);
		expect(result).toHaveLength(1);
		expect(result[0].type).toBe('image/jpeg');
		expect(result[0].name).toBe('photo.jpg');
	});

	it('converts HEIF files to JPEG', async () => {
		const heif = makeFile('photo.heif', 'image/heif');
		const result = await prepareFiles([heif]);
		expect(result).toHaveLength(1);
		expect(result[0].type).toBe('image/jpeg');
		expect(result[0].name).toBe('photo.jpg');
	});

	it('detects HEIC by extension when MIME type is empty', async () => {
		const heic = makeFile('photo.HEIC', '');
		const result = await prepareFiles([heic]);
		expect(result).toHaveLength(1);
		expect(result[0].type).toBe('image/jpeg');
	});

	it('detects HEIC by extension when MIME type is application/octet-stream', async () => {
		const heic = makeFile('photo.heic', 'application/octet-stream');
		const result = await prepareFiles([heic]);
		expect(result).toHaveLength(1);
		expect(result[0].type).toBe('image/jpeg');
	});

	it('keeps original file if conversion fails', async () => {
		const { default: heic2any } = await import('heic2any');
		(heic2any as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('decode failed'));

		const heic = makeFile('broken.heic', 'image/heic');
		const result = await prepareFiles([heic]);
		expect(result).toHaveLength(1);
		expect(result[0]).toBe(heic);
	});

	it('handles mixed HEIC and non-HEIC files', async () => {
		const jpeg = makeFile('normal.jpg', 'image/jpeg');
		const heic = makeFile('iphone.heic', 'image/heic');
		const result = await prepareFiles([jpeg, heic]);
		expect(result).toHaveLength(2);
		expect(result[0]).toBe(jpeg);
		expect(result[1].type).toBe('image/jpeg');
		expect(result[1].name).toBe('iphone.jpg');
	});
});
