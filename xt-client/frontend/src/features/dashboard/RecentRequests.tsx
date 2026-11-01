import { UserRound } from 'lucide-react';
import { useState } from 'react';
import type { RequestSummary } from '../../lib/api';
import { formatNumber } from './UsageOverview';

interface Props {
  recentRequests: RequestSummary[];
  activeRequests: RequestSummary[];
  activeCount: number;
  nowMs: number;
  accountNames: Map<string, string>;
}

function SpeedometerIcon() {
  return (
    <svg className="req-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M2.5 11.5A6.5 6.5 0 1 1 13.5 11.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      <path d="M8 8L10.5 5.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      <circle cx="8" cy="8" r="1.5" fill="currentColor" />
    </svg>
  );
}

function ClockIcon() {
  return (
    <svg className="req-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <circle cx="8" cy="8" r="6.5" stroke="currentColor" strokeWidth="1.4" />
      <path d="M8 4.5V8L10.5 9.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function CheckCircleIcon() {
  return (
    <svg className="req-icon status-check-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <circle cx="8" cy="8" r="6.5" stroke="currentColor" strokeWidth="1.4" />
      <path d="M5.5 8L7.2 9.8L10.8 6.2" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function getTagClass(tag: string): string {
  switch (tag) {
    case '用户':
      return 'tag-user';
    case '读':
      return 'tag-read';
    case '写':
      return 'tag-write';
    case '执行':
      return 'tag-exec';
    default:
      return 'tag-tool';
  }
}

export function RecentRequests({ recentRequests, activeRequests, activeCount, nowMs, accountNames }: Props) {
  const [expandedActive, setExpandedActive] = useState(false);
  const [expandedErrorId, setExpandedErrorId] = useState<string | null>(null);

  // If expanded, show all active requests + recent top 3
  const displayed = expandedActive ? activeRequests : recentRequests;

  if (displayed.length === 0) {
    return (
      <section className="requests-section" aria-label="最近请求">
        <div className="requests-header">
          <h2>最近三条请求</h2>
        </div>
        <div className="empty-requests-card" role="status">
          暂无模型请求记录。外部工具通过本地代理（/v1/chat/completions）调用模型时，状态将在此处实时展示。
        </div>
      </section>
    );
  }

  return (
    <section className="requests-section" aria-label="最近请求">
      <div className="requests-header">
        <h2>最近三条请求</h2>
        {activeCount > 3 && (
          <button
            type="button"
            className="requests-toggle-btn"
            aria-expanded={expandedActive}
            onClick={() => setExpandedActive(!expandedActive)}
          >
            {expandedActive ? '收起进行中列表' : `查看全部 ${activeCount} 条进行中请求`}
            <span aria-hidden="true">{expandedActive ? '↑' : '↓'}</span>
          </button>
        )}
      </div>

      <div className="requests-list" role="list">
        {displayed.map((req) => {
          const hasError = req.status === 'failed' || req.status === 'interrupted';
          const isErrorExpanded = expandedErrorId === req.id;

          // Calculate dynamic duration
          const elapsedMs = req.endedAt ? req.endedAt - req.startedAt : nowMs - req.startedAt;
          const elapsedSec = Math.max(1, Math.floor(elapsedMs / 1000));

          const statusTitle =
            req.status === 'pending'
              ? `等待响应 · ${elapsedSec}秒`
              : req.status === 'streaming'
              ? '正在输出'
              : req.status === 'completed'
              ? '已完成'
              : req.status === 'failed'
              ? '执行失败'
              : req.status === 'interrupted'
              ? '已中断'
              : req.statusText;

          const firstTokenVal =
            req.firstTokenDurationMs != null
              ? `${(req.firstTokenDurationMs / 1000).toFixed(1)}s`
              : '—';

          let timeVal = `${(elapsedMs / 1000).toFixed(1)}s`;
          if (req.status === 'pending') {
            timeVal = `${elapsedSec}s`;
          } else if (req.status === 'streaming') {
            timeVal = `${(elapsedMs / 1000).toFixed(1)}s`;
          }

          const firstTokenTitle =
            req.firstTokenDurationMs != null
              ? `首 Token 延迟: ${(req.firstTokenDurationMs / 1000).toFixed(1)} 秒`
              : '首 Token 延迟: 待确定';

          const timeTitle =
            req.status === 'pending'
              ? `已等待: ${elapsedSec} 秒`
              : req.status === 'streaming'
              ? `已运行: ${(elapsedMs / 1000).toFixed(1)} 秒`
              : `总耗时: ${(elapsedMs / 1000).toFixed(1)} 秒`;

          const speedText =
            req.speedTokPerSec != null
              ? `${req.speedTokPerSec.toFixed(1)} tok/s`
              : '— tok/s';

          const displayTitle =
            req.actionTitle && req.actionTitle !== '模型请求'
              ? req.actionTitle
              : req.actionTag === '写'
              ? '代码修改'
              : req.actionTag === '读'
              ? '代码分析'
              : req.actionTag === '执行'
              ? '命令执行'
              : req.actionTag === '工具'
              ? '工具调用'
              : '对话交互';

          const accountName = req.accountId
            ? accountNames.get(req.accountId) || `账号 ${req.accountId.split(':').pop()?.slice(0, 8)}`
            : '未指定账号';

          return (
            <div
              key={req.id}
              role="listitem"
              className={`request-card ${hasError ? 'has-error' : ''}`}
              tabIndex={hasError ? 0 : undefined}
              onClick={() => {
                if (hasError) {
                  setExpandedErrorId(isErrorExpanded ? null : req.id);
                }
              }}
              onKeyDown={(e) => {
                if (hasError && (e.key === 'Enter' || e.key === ' ')) {
                  e.preventDefault();
                  setExpandedErrorId(isErrorExpanded ? null : req.id);
                }
              }}
              aria-expanded={hasError ? isErrorExpanded : undefined}
            >
              <div className="req-card-main">
                {/* Left: Identity & Action */}
                <div className="req-card-left">
                  <div className="req-identity-row">
                    <div className={`req-status-indicator ${req.status}`} title={statusTitle}>
                      {req.status === 'streaming' && <span className="status-dot" aria-hidden="true" />}
                      {req.status === 'pending' && <span className="status-dot amber" aria-hidden="true" />}
                      {req.status === 'completed' && <CheckCircleIcon />}
                      {req.status === 'failed' && <span className="status-dot red" aria-hidden="true" />}
                      {req.status === 'interrupted' && <span className="status-dot red" aria-hidden="true" />}
                    </div>

                    {req.channelLabel && (
                      <span className="channel-badge" title={`模型渠道: ${req.channelLabel}`}>
                        {req.channelLabel}
                      </span>
                    )}

                    <span className="req-model-name" title={req.model}>
                      {req.model}
                    </span>

                    <span className="req-account-name" title={`使用账号: ${accountName}`}>
                      <UserRound aria-hidden="true" />
                      <span>{accountName}</span>
                    </span>
                  </div>

                  <div className="req-action-wrap">
                    <span className={`action-tag ${getTagClass(req.actionTag)}`}>
                      [{req.actionTag || '用户'}]
                    </span>
                    <span className="action-title" title={displayTitle}>
                      {displayTitle}
                    </span>
                  </div>
                </div>

                {/* Right: Quantities & Performance */}
                <div className="req-card-right">
                  <div className="req-speed-item" title={`输出速率: ${speedText}`}>
                    <SpeedometerIcon />
                    <span>{speedText}</span>
                  </div>

                  <div
                    className="req-tokens-vertical"
                    title={`输入: ${formatNumber(req.tokens.prompt)} | 输出: ${formatNumber(req.tokens.completion)} | 缓存命中: ${formatNumber(req.tokens.cached)}`}
                  >
                    <div className="req-token-line">
                      <span title="输入 Token (Prompt)">↑ {formatNumber(req.tokens.prompt)}</span>
                      <span title="缓存命中 Token (Cache)" className="req-token-cache">⛁ {formatNumber(req.tokens.cached)}</span>
                    </div>
                    <div className="req-token-line">
                      <span title="输出 Token (Completion)">↓ {formatNumber(req.tokens.completion)}</span>
                    </div>
                  </div>

                  <div className="req-time-group" title={`${firstTokenTitle} · ${timeTitle}`}>
                    <ClockIcon />
                    <div className="req-time-vertical">
                      <span className="req-time-line" title={firstTokenTitle}>{firstTokenVal}</span>
                      <span className="req-time-line" title={timeTitle}>{timeVal}</span>
                    </div>
                  </div>
                </div>
              </div>

              {/* Expandable Error Summary */}
              {hasError && isErrorExpanded && (
                <div className="req-error-drawer" role="alert">
                  <strong>错误详情：</strong> {req.errorSummary || '请求异常中断或未完成'}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}
