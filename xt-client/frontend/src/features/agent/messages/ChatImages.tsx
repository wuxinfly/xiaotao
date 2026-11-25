import {useRef} from 'react';
import {ImagePlus, X} from 'lucide-react';
import type {ComposerImage} from '../composerImages';

export function ChatImagePicker({onAdd, disabled}: {onAdd: (files: File[]) => void; disabled: boolean}) {
  const input = useRef<HTMLInputElement>(null);
  return <>
    <button type="button" className="agent-option" aria-label="添加图片" title="添加图片" disabled={disabled} onClick={() => input.current?.click()}>
      <ImagePlus aria-hidden="true"/>
    </button>
    <input ref={input} type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple hidden onChange={event => {
      onAdd(Array.from(event.currentTarget.files ?? []));
      event.currentTarget.value = '';
    }}/>
  </>;
}

export function ChatImagePreviews({images, onRemove}: {images: ComposerImage[]; onRemove: (id: string) => void}) {
  if (!images.length) return null;
  return <div className="agent-composer-images" aria-label="待发送图片">
    {images.map((image, index) => <div className="agent-composer-image" key={image.id}>
      <img src={image.preview} alt={image.name} title={image.name}/>
      <button type="button" aria-label={`移除图片 ${index + 1}`} title="移除图片" onClick={() => onRemove(image.id)}><X aria-hidden="true"/></button>
    </div>)}
  </div>;
}
