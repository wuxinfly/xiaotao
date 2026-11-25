import {useEffect, useRef, useState, type FormEvent} from 'react';
import {ArrowLeft, ArrowRight, ExternalLink, Globe2, RotateCw} from 'lucide-react';
import {toast} from '../../components/toast';
import {workspaceApi} from '../../lib/workspace';
import './workspace-tools.css';

const errorText = (reason: unknown) => reason instanceof Error ? reason.message : String(reason);

/** Normalizes a typed address; only http(s) targets may reach the frame. */
export function normalizeAddress(input: string) {
  const value = input.trim();
  if (!value) throw new Error('先输入一个网址');
  const withScheme = /^[a-zA-Z][a-zA-Z\d+.-]*:/.test(value) ? value
    : `${/^(localhost|127\.0\.0\.1|\[::1\]|([\w-]+\.)*localhost)(:\d+)?([/?#]|$)/i.test(value) ? 'http' : 'https'}://${value}`;
  let url: URL;
  try { url = new URL(withScheme); }
  catch { throw new Error('网址格式不对，例如 https://example.com'); }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') throw new Error('只支持 http 或 https 地址');
  if (!url.hostname) throw new Error('网址缺少主机名');
  return url.toString();
}

export function WorkspaceBrowser({active}: {active: boolean}) {
  const [address, setAddress] = useState('');
  const [src, setSrc] = useState('');
  const [error, setError] = useState('');
  const [history, setHistory] = useState<string[]>([]);
  const [cursor, setCursor] = useState(-1);
  const [reloadKey, setReloadKey] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const frame = useRef<HTMLIFrameElement>(null);

  useEffect(() => {
    if (!active) return;
    input.current?.focus({preventScroll: true});
  }, [active]);

  function go(event: FormEvent) {
    event.preventDefault();
    try {
      const url = normalizeAddress(address);
      const next = [...history.slice(0, cursor + 1), url];
      setError('');
      setHistory(next);
      setCursor(next.length - 1);
      setAddress(url);
      setSrc(url);
    } catch (reason) {
      setError(errorText(reason));
    }
  }

  function travel(step: number) {
    const next = cursor + step;
    if (next < 0 || next >= history.length) return;
    setCursor(next);
    setAddress(history[next]);
    setSrc(history[next]);
    setError('');
  }

  function syncAddress() {
    if (!frame.current) return;
    try {
      const href = frame.current.contentWindow?.location.href;
      if (href && href !== 'about:blank') setAddress(href);
    } catch { /* Cross-origin pages cannot be read; the typed address stays. */ }
  }

  function openOutside() {
    if (!src) return;
    workspaceApi.openUrl(src).catch(reason => toast.error(errorText(reason)));
  }

  return <section className="workspace-browser-pane" aria-label="浏览器">
    <header className="workspace-tool-head">
      <button type="button" className="workspace-tool-icon" aria-label="后退到上一个输入的网址" disabled={cursor <= 0} onClick={() => travel(-1)}>
        <ArrowLeft aria-hidden="true"/>
      </button>
      <button type="button" className="workspace-tool-icon" aria-label="前进到下一个输入的网址" disabled={cursor >= history.length - 1} onClick={() => travel(1)}>
        <ArrowRight aria-hidden="true"/>
      </button>
      <button type="button" className="workspace-tool-icon" aria-label="刷新网页" title="刷新" disabled={!src} onClick={() => setReloadKey(current => current + 1)}>
        <RotateCw aria-hidden="true"/>
      </button>
      <form className="workspace-address" onSubmit={go}>
        <input ref={input} value={address} aria-label="浏览器地址" placeholder="输入网址，比如 example.com"
          spellCheck={false} autoCapitalize="off" autoComplete="off" onChange={event => setAddress(event.target.value)}/>
        <button type="submit">访问</button>
      </form>
      <button type="button" className="workspace-tool-icon" aria-label="在系统浏览器中打开" title="在系统浏览器中打开" disabled={!src} onClick={openOutside}>
        <ExternalLink aria-hidden="true"/>
      </button>
    </header>
    {error && <p className="workspace-browser-error" role="alert">{error}</p>}
    <div className="workspace-browser-view">
      {src ? <iframe key={reloadKey} ref={frame} src={src} title={`网页 ${src}`} onLoad={syncAddress}
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-downloads"
        referrerPolicy="no-referrer"/> : <div className="workspace-browser-empty">
        <span><Globe2 aria-hidden="true"/></span>
        <strong>在应用里直接看网页</strong>
        <p>输入地址后按「访问」。有些网站不允许被嵌入，白屏时点右上角用系统浏览器打开。</p>
      </div>}
    </div>
    {src && <p className="workspace-browser-note">页面内跳转可能不会同步到地址栏；打不开的网站请用系统浏览器。</p>}
  </section>;
}
