/**
 * Pasted-image normalisation before it is uploaded to the model.
 *
 * Two independent savings are applied:
 *
 * - Scale down: vision encoders price an image by its pixel area, and they
 *   downscale oversized inputs internally anyway. Shrinking to the common
 *   1568px long-edge limit before upload costs the model nothing and cuts the
 *   number of tiles it has to encode.
 * - Re-encode: a screenshot pasted from a 4K screen is often a large PNG with
 *   little actual detail. Re-encoding as JPEG shrinks the payload without
 *   changing the pixels the model sees.
 *
 * Animated GIFs are left untouched: re-encoding would throw away the motion.
 * PNGs with transparency stay PNG, because JPEG has no alpha channel and a
 * transparent screenshot would turn black.
 */

const MAX_LONG_EDGE = 1568;
const JPEG_QUALITY = 0.85;
/** Below this size re-encoding cannot pay for its own quality loss. */
const REENCODE_MIN_BYTES = 64 * 1024;

export type NormalisedImage = {
  data: string;
  mimeType: string;
  preview: string;
  size: number;
  width: number;
  height: number;
  /** True when the original pixels were replaced by a smaller copy. */
  scaled: boolean;
  /** True when the bytes were re-encoded into a cheaper container. */
  reencoded: boolean;
};

type DecodedImage = {
  image: HTMLImageElement;
  width: number;
  height: number;
};

function decodeImage(source: string): Promise<DecodedImage> {
  return new Promise((resolve, reject) => {
    const image = new window.Image();
    image.onload = () => resolve({image, width: image.naturalWidth, height: image.naturalHeight});
    image.onerror = () => reject(new Error('无法识别图片内容'));
    image.src = source;
  });
}

function canvasToBlob(canvas: HTMLCanvasElement, mimeType: string, quality?: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob(blob => {
      if (blob) resolve(blob);
      else reject(new Error('图片压缩失败'));
    }, mimeType, quality);
  });
}

function blobToDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error('图片压缩失败'));
    reader.onload = () => resolve(String(reader.result || ''));
    reader.readAsDataURL(blob);
  });
}

/** Long edge of the scaled copy, or the original size when already small enough. */
export function scaledSize(width: number, height: number, maxLongEdge: number = MAX_LONG_EDGE) {
  const longEdge = Math.max(width, height);
  if (longEdge <= maxLongEdge) return {width, height};
  const ratio = maxLongEdge / longEdge;
  return {
    width: Math.max(1, Math.round(width * ratio)),
    height: Math.max(1, Math.round(height * ratio)),
  };
}

/**
 * True when the image actually uses transparency that JPEG cannot represent.
 *
 * The probe runs on its own canvas before the image is drawn, because once the
 * image is composited the alpha information is already gone. Only a corner
 * pixel is inspected: a fully transparent border is the common case for
 * screenshots, and a transparent region in the middle still shows up as a
 * transparent corner on most captures.
 */
function detectTransparency(image: HTMLImageElement, width: number, height: number): boolean {
  const probe = document.createElement('canvas');
  probe.width = Math.min(width, 32);
  probe.height = Math.min(height, 32);
  const context = probe.getContext('2d', {willReadFrequently: true});
  if (!context) return true;
  try {
    context.drawImage(image, 0, 0, probe.width, probe.height);
    const {data} = context.getImageData(0, 0, probe.width, probe.height);
    for (let index = 3; index < data.length; index += 4) {
      if (data[index] < 255) return true;
    }
    return false;
  } catch {
    // A tainted canvas cannot be inspected; assume transparency so the image
    // is not flattened onto a black background.
    return true;
  }
}

function hasAlphaChannel(mimeType: string): boolean {
  return mimeType === 'image/png' || mimeType === 'image/webp' || mimeType === 'image/gif';
}

export async function normaliseImage(
  file: File,
  maxLongEdge: number = MAX_LONG_EDGE,
  quality: number = JPEG_QUALITY,
): Promise<NormalisedImage> {
  const originalDataURL = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error('读取图片失败'));
    reader.onload = () => resolve(String(reader.result || ''));
    reader.readAsDataURL(file);
  });

  const decoded = await decodeImage(originalDataURL);
  const {width, height} = decoded;
  if (!width || !height) throw new Error('无法识别图片内容');

  const target = scaledSize(width, height, maxLongEdge);
  const scaled = target.width !== width || target.height !== height;

  // Animated GIFs and images already within the size budget are sent as they
  // are: re-encoding would either destroy the animation or cost quality for
  // no meaningful saving.
  if (!scaled && (file.type === 'image/gif' || file.size < REENCODE_MIN_BYTES)) {
    return {
      data: originalDataURL.slice(originalDataURL.indexOf(',') + 1),
      mimeType: file.type,
      preview: originalDataURL,
      size: file.size,
      width,
      height,
      scaled: false,
      reencoded: false,
    };
  }

  // Probe the original pixels before compositing them onto the target canvas.
  const keepAlpha = hasAlphaChannel(file.type) && detectTransparency(decoded.image, width, height);

  const canvas = document.createElement('canvas');
  canvas.width = target.width;
  canvas.height = target.height;
  const context = canvas.getContext('2d');
  if (!context) throw new Error('图片压缩失败');
  context.drawImage(decoded.image, 0, 0, target.width, target.height);

  const scaledBlob = await canvasToBlob(canvas, 'image/png');

  if (!keepAlpha) {
    const jpegBlob = await canvasToBlob(canvas, 'image/jpeg', quality);
    // JPEG wins when it beats every alternative, including the untouched
    // original: a flat-coloured source can encode larger as JPEG than as PNG.
    if (jpegBlob.size < scaledBlob.size && jpegBlob.size < file.size) {
      const preview = await blobToDataURL(jpegBlob);
      return {
        data: preview.slice(preview.indexOf(',') + 1),
        mimeType: 'image/jpeg',
        preview,
        size: jpegBlob.size,
        width: target.width,
        height: target.height,
        scaled,
        reencoded: true,
      };
    }
  }

  // Re-encoding the scaled copy is not always a win. A downscaled PNG can be
  // larger than the original file, for instance when resampling breaks up a
  // pattern the original encoder handled well. Keep the original bytes in
  // that case; the pixel reduction is the part that matters for the model.
  if (scaledBlob.size >= file.size && !scaled) {
    return {
      data: originalDataURL.slice(originalDataURL.indexOf(',') + 1),
      mimeType: file.type,
      preview: originalDataURL,
      size: file.size,
      width,
      height,
      scaled: false,
      reencoded: false,
    };
  }

  const preview = await blobToDataURL(scaledBlob);
  return {
    data: preview.slice(preview.indexOf(',') + 1),
    mimeType: 'image/png',
    preview,
    size: scaledBlob.size,
    width: target.width,
    height: target.height,
    scaled,
    reencoded: false,
  };
}
