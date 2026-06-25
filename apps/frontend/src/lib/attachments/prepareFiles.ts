/**
 * Detects HEIC/HEIF files and converts them to JPEG before upload.
 * Uses dynamic import so the WASM decoder is only fetched when needed.
 */

function isHeic(file: File): boolean {
	const type = file.type.toLowerCase();
	if (type === 'image/heic' || type === 'image/heif') return true;
	// Some browsers report empty MIME type for HEIC files
	if (!type || type === 'application/octet-stream') {
		const name = file.name.toLowerCase();
		return name.endsWith('.heic') || name.endsWith('.heif');
	}
	return false;
}

async function convertHeicToJpeg(file: File): Promise<File> {
	const { default: heic2any } = await import('heic2any');
	const result = await heic2any({ blob: file, toType: 'image/jpeg', quality: 0.92 });
	const blob = Array.isArray(result) ? result[0] : result;
	const newName = file.name.replace(/\.heic$/i, '.jpg').replace(/\.heif$/i, '.jpg');
	return new File([blob], newName, { type: 'image/jpeg' });
}

export async function prepareFiles(files: File[]): Promise<File[]> {
	return Promise.all(
		files.map(async (file) => {
			if (!isHeic(file)) return file;
			try {
				return await convertHeicToJpeg(file);
			} catch (err) {
				console.warn('HEIC conversion failed, keeping original file:', err);
				return file;
			}
		})
	);
}
