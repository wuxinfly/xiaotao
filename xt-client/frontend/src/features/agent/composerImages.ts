import {normaliseImage} from './imageNormalise';

export type ComposerImage = {
  id: string;
  data: string;
  mimeType: string;
  name: string;
  preview: string;
  size: number;
  width?: number;
  height?: number;
};

const composerImageTypes = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp']);

export async function readImageFile(file: File): Promise<ComposerImage> {
  if (!composerImageTypes.has(file.type)) {
    throw new Error('仅支持 PNG、JPEG、GIF 和 WebP 图片');
  }
  if (file.size > 5 * 1024 * 1024) {
    throw new Error('单张图片不能超过 5 MiB');
  }
  // Scale and re-encode before upload: vision models price images by pixel
  // area, and a large screenshot costs far more to send than it does to read.
  const normalised = await normaliseImage(file);
  if (!normalised.width || !normalised.height || normalised.width * normalised.height > 40_000_000) {
    throw new Error('图片尺寸无效或过大');
  }
  return {
    id: crypto.randomUUID(),
    data: normalised.data,
    mimeType: normalised.mimeType,
    name: file.name || '粘贴的图片',
    preview: normalised.preview,
    size: normalised.size,
    width: normalised.width,
    height: normalised.height,
  };
}

export function pastedImageFiles(data: DataTransfer | null): File[] {
  if (!data) return [];
  const files = Array.from(data.files).filter(file => file.type.startsWith('image/'));
  if (files.length) return files;
  return Array.from(data.items)
    .filter(item => item.kind === 'file' && item.type.startsWith('image/'))
    .map(item => item.getAsFile())
    .filter((file): file is File => !!file);
}
