import {useEffect, useRef, useState} from 'react';
import {api} from '../../lib/api';
import {toast} from '../../components/toast';
import {ModelEndpointCard} from './components/ModelEndpointCard';
import './accounts.css';

export function LocalGateway() {
  const [key, setKey] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  const live = useRef(true);
  useEffect(() => {live.current = true; return () => {live.current = false;};}, []);
  useEffect(() => {
    let current = true;
    api.proxyKey().then(result => {if (current) {setKey(result.key); setError('');}}).catch(reason => {if (current) setError(reason instanceof Error ? reason.message : String(reason));});
    return () => {current = false;};
  }, [attempt]);
  return <div className="source-local-gateway">
    {error && <p role="alert" className="source-error">本地访问密钥读取失败：{error} <button type="button" onClick={() => setAttempt(value => value + 1)}>重试</button></p>}
    <ModelEndpointCard origin={window.location.origin} apiKey={key} onRotateKey={async () => {
      try {const result = await api.rotateProxyKey(); if (live.current) {setKey(result.key); toast.success('本地访问密钥已更新');}}
      catch (reason) {if (live.current) toast.error(reason instanceof Error ? reason.message : String(reason));}
    }}/>
  </div>;
}
