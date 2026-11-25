import {useRef, useState} from 'react';
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

export function useComposerImageDrafts(onError: (message: string) => void) {
  const [images, setImages] = useState<ComposerImage[]>([]);
  const [reading, setReading] = useState(0);
  const current = useRef(images);

  function update(next: ComposerImage[]) {
    current.current = next;
    setImages(next);
  }

  async function add(files: File[]) {
    if (!files.length) return;
    const space = 4 - current.current.length;
    if (space <= 0) {
      onError('每条消息最多添加 4 张图片');
      return;
    }
    setReading(value => value + 1);
    try {
      const incoming = await Promise.all(files.slice(0, space).map(readImageFile));
      const combined = [...current.current, ...incoming];
      const next = combined.slice(0, 4);
      if (next.reduce((total, image) => total + image.size, 0) > 12 * 1024 * 1024) {
        onError('图片合计不能超过 12 MiB');
        return;
      }
      update(next);
      onError(files.length > space || combined.length > 4 ? '每条消息最多添加 4 张图片' : '');
    } catch (reason) {
      onError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setReading(value => value - 1);
    }
  }

  function remove(ids: string[]) {
    update(current.current.filter(image => !ids.includes(image.id)));
  }

  return {images, reading: reading > 0, add, remove};
}
