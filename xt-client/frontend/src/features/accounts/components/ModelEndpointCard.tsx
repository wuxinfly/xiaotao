import React, { useState } from 'react';
import { toast } from '../../../components/toast';

interface ModelEndpointCardProps {
  origin: string;
  apiKey: string | null;
  onRotateKey: () => Promise<void>;
}

async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    let ok = false;
    try {
      ok = document.execCommand('copy');
    } catch {
      // ignore
    }
    ta.remove();
    return ok;
  }
}

function maskKey(key: string | null): string {
  if (!key) return '加载中…';
  if (key.length <= 12) return '••••••••••••';
  const prefix = key.startsWith('sk-') ? 'sk-' : key.slice(0, 3);
  const suffix = key.slice(-4);
  return `${prefix}••••••••••••••••••••••••${suffix}`;
}

export const ModelEndpointCard: React.FC<ModelEndpointCardProps> = ({
  origin,
  apiKey,
  onRotateKey,
}) => {
  const [showKey, setShowKey] = useState(false);
  const [copiedField, setCopiedField] = useState<'url' | 'key' | null>(null);
  const [rotating, setRotating] = useState(false);

  const endpointUrl = `${origin}/v1`;

  const handleCopy = async (field: 'url' | 'key', text: string) => {
    if (!text) return;
    const ok = await copyToClipboard(text);
    if (ok) {
      setCopiedField(field);
      toast.success(field === 'url' ? '接口地址已复制' : 'API 密钥已复制');
      setTimeout(() => setCopiedField(null), 1600);
    } else {
      toast.error('复制失败，请手动选择');
    }
  };

  const handleRotate = async () => {
    if (rotating) return;
    if (!window.confirm('确定重新生成 API 密钥？已在其它工具中配置的旧密钥将会失效。')) {
      return;
    }
    setRotating(true);
    try {
      await onRotateKey();
    } finally {
      setRotating(false);
    }
  };

  return (
    <div className="endpoint-banner-card">
      <div className="endpoint-banner-header">
        <div className="endpoint-title-group">
          <h2 className="endpoint-title">模型接入</h2>
          <p className="endpoint-subtitle">统一的模型调用接口，在支持 OpenAI 规范的工具中填入以下信息即可调用</p>
        </div>
        <div className="endpoint-protocol-capsules">
          <span className="protocol-capsule">OpenAI Chat Completions</span>
          <span className="protocol-capsule code-font">/v1/models</span>
        </div>
      </div>

      <div className="endpoint-grid">
        {/* Field 1: 接口地址 */}
        <div className="endpoint-row">
          <span className="field-label">接口地址</span>
          <div className="field-box">
            <span className="field-value mono" title={endpointUrl}>{endpointUrl}</span>
            <button
              type="button"
              className="field-action-btn"
              onClick={() => handleCopy('url', endpointUrl)}
              title="复制接口地址"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
              </svg>
              <span>{copiedField === 'url' ? '已复制' : '复制'}</span>
            </button>
          </div>
        </div>

        {/* Field 2: API 密钥 */}
        <div className="endpoint-row">
          <span className="field-label">API 密钥</span>
          <div className="field-box">
            <span className="field-value mono" title={showKey && apiKey ? apiKey : '点击眼睛查看'}>
              {showKey && apiKey ? apiKey : maskKey(apiKey)}
            </span>
            <div className="field-actions">
              <button
                type="button"
                className="field-icon-btn"
                onClick={() => setShowKey(!showKey)}
                title={showKey ? '隐藏密钥' : '显示密钥'}
                aria-label={showKey ? '隐藏密钥' : '显示密钥'}
              >
                {showKey ? (
                  <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
                    <line x1="1" y1="1" x2="23" y2="23" />
                  </svg>
                ) : (
                  <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                    <circle cx="12" cy="12" r="3" />
                  </svg>
                )}
              </button>
              <button
                type="button"
                className="field-action-btn"
                onClick={() => apiKey && handleCopy('key', apiKey)}
                disabled={!apiKey}
                title="复制 API 密钥"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                </svg>
                <span>{copiedField === 'key' ? '已复制' : '复制'}</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <div className="endpoint-footer">
        <button
          type="button"
          className="rotate-key-btn"
          onClick={handleRotate}
          disabled={!apiKey || rotating}
        >
          <svg
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            className={rotating ? 'spin' : ''}
            aria-hidden="true"
          >
            <polyline points="23 4 23 10 17 10" />
            <polyline points="1 20 1 14 7 14" />
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
          <span>{rotating ? '正在生成…' : '重新生成密钥'}</span>
        </button>
      </div>
    </div>
  );
};
