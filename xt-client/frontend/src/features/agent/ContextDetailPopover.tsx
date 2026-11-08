import {useRef, useState, type CSSProperties, type RefObject} from 'react';
import {Popover} from '../../components/popover/Popover';
import type {AgentRunStats, ContextBreakdown} from '../../lib/api';
import './context-detail.css';

export interface ContextMeterProps {
  stats?: AgentRunStats;
  contextWindow?: number;
  modelLabel?: string;
}

export interface ContextDetailPopoverProps {
  open: boolean;
  onClose: () => void;
  anchorRef: RefObject<HTMLElement | null>;
  stats?: AgentRunStats;
  contextWindow?: number;
  modelLabel?: string;
}

interface Segment {
  key: string;
  label: string;
  tokens: number;
  color: string;
}

function formatTokens(value?: number): string {
  if (value === undefined || value === null || Number.isNaN(value)) return '—';
  return value.toLocaleString('en-US');
}

function formatCompactTokens(value: number): string {
  if (value >= 1_000_000) return (value / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  if (value >= 1_000) return (value / 1_000).toFixed(1).replace(/\.0$/, '') + 'K';
  return String(value);
}

function buildSegments(breakdown?: ContextBreakdown): Segment[] {
  if (!breakdown) return [];
  const segments: Segment[] = [
    {key: 'system', label: '系统与项目指令', tokens: breakdown.systemTokens, color: '#38bdf8'},
    {key: 'tools', label: '工具定义 (Schema)', tokens: breakdown.toolsTokens, color: '#818cf8'},
    {key: 'messages', label: '对话消息', tokens: breakdown.messagesTokens, color: '#34d399'},
    {key: 'toolResults', label: '工具调用与输出', tokens: breakdown.toolResultsTokens, color: '#fbbf24'},
    {key: 'reference', label: '引用对话', tokens: breakdown.referenceTokens, color: '#f472b6'},
    {key: 'image', label: '图片估算', tokens: breakdown.imageTokens, color: '#a78bfa'},
  ];
  return segments.filter(s => s.tokens > 0);
}

export function ContextMeter({stats, contextWindow = 0, modelLabel}: ContextMeterProps) {
  const [open, setOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const contextTokens = stats?.contextTokens ?? 0;
  if (contextTokens <= 0) return null;

  const effectiveWindow = contextWindow > 0 ? contextWindow : (stats?.contextWindow ?? 0);
  const contextRatio = effectiveWindow > 0 ? Math.min(1, contextTokens / effectiveWindow) : 0;
  const labelText = effectiveWindow > 0
    ? `已用约 ${contextTokens} / ${effectiveWindow} tokens (${Math.round(contextRatio * 100)}%) · 点击查看详情`
    : `已用约 ${contextTokens} tokens · 上限未知 · 点击查看详情`;

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        className="agent-context-meter"
        aria-label={effectiveWindow > 0 ? `上下文用量 ${Math.round(contextRatio * 100)}%` : `上下文用量，约 ${contextTokens} tokens，上限未知`}
        aria-expanded={open}
        aria-haspopup="dialog"
        title={labelText}
        onClick={() => setOpen(prev => !prev)}
      >
        <span
          className={effectiveWindow > 0 ? 'agent-context-ring' : 'agent-context-dot'}
          aria-hidden="true"
          style={{'--context-ratio': contextRatio} as CSSProperties}
        >
          <i />
        </span>
        <span>{effectiveWindow > 0 ? `ctx ${Math.round(contextRatio * 100)}%` : `ctx ${formatCompactTokens(contextTokens)}`}</span>
      </button>

      <ContextDetailPopover
        open={open}
        onClose={() => setOpen(false)}
        anchorRef={buttonRef}
        stats={stats}
        contextWindow={effectiveWindow}
        modelLabel={modelLabel}
      />
    </>
  );
}

export function ContextDetailPopover({
  open,
  onClose,
  anchorRef,
  stats,
  contextWindow = 0,
  modelLabel,
}: ContextDetailPopoverProps) {
  const windowTokens = contextWindow > 0 ? contextWindow : (stats?.contextWindow ?? 0);
  const contextTokens = stats?.contextTokens ?? 0;
  const ratio = windowTokens > 0 ? Math.min(1, contextTokens / windowTokens) : 0;
  const remaining = windowTokens > 0 ? Math.max(0, windowTokens - contextTokens) : null;
  const breakdown = stats?.breakdown;
  const segments = buildSegments(breakdown);
  const totalBreakdown = breakdown?.totalTokens ?? contextTokens;

  return (
    <Popover
      open={open}
      onClose={onClose}
      anchorRef={anchorRef}
      side="top"
      align="start"
      className="agent-context-detail-popover"
      aria-label="上下文与用量详情"
      role="dialog"
    >
      <div className="agent-context-detail-pane">
        <header className="agent-context-detail-header">
          <div className="agent-context-detail-title-row">
            <span className="agent-context-detail-title">上下文与用量详情</span>
            {modelLabel && <span className="agent-context-detail-model" title={modelLabel}>{modelLabel}</span>}
          </div>
        </header>

        {/* 1. 当前请求上下文（估算） */}
        <section className="agent-context-detail-section" aria-labelledby="context-estimate-heading">
          <div className="agent-context-detail-section-title">
            <span id="context-estimate-heading">当前模型请求上下文</span>
            <span className="agent-context-detail-badge is-estimate">估算</span>
          </div>

          <div className="agent-context-detail-metric-card">
            <div className="agent-context-detail-metric-row">
              <span className="agent-context-detail-metric-label">当前准备发送</span>
              <span className="agent-context-detail-metric-value">
                <strong>{formatTokens(contextTokens)}</strong>
                {windowTokens > 0 ? <small> / {formatTokens(windowTokens)} tokens</small> : <small> tokens</small>}
              </span>
            </div>

            {windowTokens > 0 ? (
              <div className="agent-context-detail-progress-track" aria-hidden="true">
                <div
                  className="agent-context-detail-progress-fill"
                  style={{width: `${Math.round(ratio * 100)}%`} as CSSProperties}
                />
              </div>
            ) : null}

            <div className="agent-context-detail-subtext">
              {windowTokens > 0 ? (
                <>
                  已占用 <strong>{Math.round(ratio * 100)}%</strong> · 剩余已知空间 <strong>{formatTokens(remaining ?? 0)}</strong> tokens
                </>
              ) : (
                <>当前模型窗口上限未配置或未知</>
              )}
            </div>
          </div>

          {/* 分类明细 */}
          {segments.length > 0 && totalBreakdown > 0 ? (
            <div className="agent-context-detail-breakdown">
              <div className="agent-context-detail-stacked-bar" aria-hidden="true">
                {segments.map(seg => {
                  const segRatio = (seg.tokens / totalBreakdown) * 100;
                  return (
                    <span
                      key={seg.key}
                      style={{width: `${segRatio}%`, backgroundColor: seg.color} as CSSProperties}
                      title={`${seg.label}: ${formatTokens(seg.tokens)} tokens (${Math.round(segRatio)}%)`}
                    />
                  );
                })}
              </div>

              <ul className="agent-context-detail-legend" aria-label="上下文构成明细">
                {segments.map(seg => {
                  const segPercent = Math.round((seg.tokens / totalBreakdown) * 100);
                  return (
                    <li key={seg.key} className="agent-context-detail-legend-item">
                      <span className="agent-context-detail-legend-dot" style={{backgroundColor: seg.color} as CSSProperties} aria-hidden="true" />
                      <span className="agent-context-detail-legend-name">{seg.label}</span>
                      <span className="agent-context-detail-legend-count">{formatTokens(seg.tokens)}</span>
                      <span className="agent-context-detail-legend-pct">{segPercent}%</span>
                    </li>
                  );
                })}
              </ul>
            </div>
          ) : (
            <p className="agent-context-detail-hint">分类明细将在发起请求后生成</p>
          )}

          {breakdown?.hasImages && (
            <div className="agent-context-detail-note">
              📷 包含图片附件，按模型单图标准预留估算，实际以视觉模型为准。
            </div>
          )}
        </section>

        {/* 2. 任务累计实际用量（实际） */}
        <section className="agent-context-detail-section" aria-labelledby="context-actual-heading">
          <div className="agent-context-detail-section-title">
            <span id="context-actual-heading">本轮任务累计实际用量</span>
            <span className="agent-context-detail-badge is-actual">实际</span>
          </div>

          <div className="agent-context-detail-stats-grid">
            <div className="agent-context-detail-stat-cell">
              <span className="agent-context-detail-stat-label">实际输入 Token</span>
              <span className="agent-context-detail-stat-num">
                {stats?.inputKnown ? formatTokens(stats.inputTokens) : '—'}
              </span>
            </div>
            <div className="agent-context-detail-stat-cell">
              <span className="agent-context-detail-stat-label">实际输出 Token</span>
              <span className="agent-context-detail-stat-num">
                {(stats?.outputTokens ?? 0) > 0 ? formatTokens(stats?.outputTokens) : '—'}
              </span>
            </div>
            <div className="agent-context-detail-stat-cell">
              <span className="agent-context-detail-stat-label">缓存命中 Token</span>
              <span className="agent-context-detail-stat-num">
                {stats?.cacheKnown ? formatTokens(stats.cachedTokens) : '—'}
              </span>
            </div>
            <div className="agent-context-detail-stat-cell">
              <span className="agent-context-detail-stat-label">模型请求轮次</span>
              <span className="agent-context-detail-stat-num">
                {stats?.modelRequests || 1} 次
              </span>
            </div>
          </div>
          <p className="agent-context-detail-footnote">
            实际用量来自各模型供应商返回的真实 usage 字段；未返回缓存明细时标记为“—”。
          </p>
        </section>
      </div>
    </Popover>
  );
}
