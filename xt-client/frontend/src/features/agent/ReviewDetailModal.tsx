import {useState} from 'react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {Download, PlusCircle, CheckCircle2} from 'lucide-react';
import {type BehaviorReview, type ReviewIssue, type ReviewSuggestionLink} from '../../lib/api';
import {ReviewSuggestionModal} from './ReviewSuggestionModal';
import './review.css';

interface ReviewDetailModalProps {
  review: BehaviorReview;
  onClose: () => void;
  onUpdate?: (updated: BehaviorReview) => void;
}

export function ReviewDetailModal({review, onClose, onUpdate}: ReviewDetailModalProps) {
  const [selectedIssueForSuggestion, setSelectedIssueForSuggestion] = useState<ReviewIssue | null>(null);
  const [currentReview, setCurrentReview] = useState<BehaviorReview>(review);

  const handleExport = () => {
    window.open(`/api/agent/reviews/${encodeURIComponent(currentReview.id)}/export`, '_blank');
  };

  const handleSuggestionSuccess = (link: ReviewSuggestionLink) => {
    const updatedLinks = [...(currentReview.suggestionLinks || []), link];
    const updated = {...currentReview, suggestionLinks: updatedLinks};
    setCurrentReview(updated);
    if (onUpdate) onUpdate(updated);
  };

  const result = currentReview.result;
  const outcome = result?.outcome || 'insufficient_evidence';

  let outcomeLabel = '证据不足，无法充分判断';
  if (outcome === 'issues_found') outcomeLabel = '发现问题';
  else if (outcome === 'no_issues_found') outcomeLabel = '本次范围内未发现有依据的问题';

  const formatDateTime = (iso: string) => {
    try {
      const d = new Date(iso);
      return d.toLocaleString('zh-CN', {timeZone: currentReview.timezone || 'Asia/Shanghai', hour12: false});
    } catch {
      return iso;
    }
  };

  return (
    <>
      <ModalFrame
        title="行为复盘详情"
        className="review-detail-modal"
        onClose={onClose}
        footer={
          <div className="app-modal-actions" style={{justifyContent: 'space-between', width: '100%'}}>
            <div>
              {currentReview.status === 'succeeded' && (
                <button
                  type="button"
                  className="review-action-btn"
                  onClick={handleExport}
                  title="导出单份独立 Markdown 文件"
                >
                  <Download size={14} /> 导出 Markdown
                </button>
              )}
            </div>
            <button type="button" className="app-modal-button secondary" onClick={onClose}>
              关闭
            </button>
          </div>
        }
      >
        <div className="review-detail-content">
          {/* Section 1: Scope */}
          <div className="review-section">
            <div className="review-section-title">1. 复盘范围</div>
            <div className="review-meta-grid">
              <div className="review-meta-item">
                <strong>会话标题:</strong>
                <span>{currentReview.sessionTitleSnapshot || '未命名会话'}</span>
              </div>
              <div className="review-meta-item">
                <strong>关联项目:</strong>
                <span>{currentReview.projectSnapshot || '默认目录'}</span>
              </div>
              <div className="review-meta-item">
                <strong>生成时间:</strong>
                <span>{formatDateTime(currentReview.createdAt)}</span>
              </div>
              <div className="review-meta-item">
                <strong>评估模型:</strong>
                <span>{currentReview.model}</span>
              </div>
              <div className="review-meta-item">
                <strong>客户端版本:</strong>
                <span>{currentReview.clientVersion}</span>
              </div>
              <div className="review-meta-item">
                <strong>Core 版本:</strong>
                <span>{currentReview.coreVersion}</span>
              </div>
              <div className="review-meta-item" style={{gridColumn: '1 / -1'}}>
                <strong>Run 覆盖:</strong>
                <span>
                  共可用 {currentReview.scope.availableCount} 个已结束 Run，实际纳入分析 {currentReview.scope.includedCount} 个
                  {currentReview.scope.runIds?.length ? ` (${currentReview.scope.runIds.join(', ')})` : ''}
                </span>
              </div>
              {currentReview.scope.truncation && currentReview.scope.truncation.length > 0 && (
                <div className="review-meta-item" style={{gridColumn: '1 / -1', color: 'var(--text-muted, #94a3b8)'}}>
                  <strong>截取说明:</strong>
                  <span>{currentReview.scope.truncation.join('；')}</span>
                </div>
              )}
            </div>
          </div>

          {/* Section 2: Conclusion */}
          <div className="review-section">
            <div className="review-section-title">2. 复盘结论</div>
            <div className={`review-outcome-box ${outcome}`}>
              <div className="review-outcome-header">结论：{outcomeLabel}</div>
              {result?.summary && <div className="review-outcome-summary">{result.summary}</div>}
            </div>
          </div>

          {/* Section 3: Issues */}
          <div className="review-section">
            <div className="review-section-title">3. 发现的问题 ({result?.issues?.length || 0})</div>
            {(!result?.issues || result.issues.length === 0) ? (
              <div className="review-empty-tip">本次复盘范围内未发现有确凿依据的协作或行为问题。</div>
            ) : (
              <div className="review-cards-list">
                {result.issues.map((issue, idx) => {
                  const linked = currentReview.suggestionLinks?.find(l => l.issueId === issue.id);
                  return (
                    <div key={issue.id || idx} className="review-card">
                      <div className="review-card-header">
                        <span className="review-card-title">{idx + 1}. {issue.title}</span>
                        <div>
                          {linked ? (
                            <span className="review-suggestion-link-badge" title={`已关联建议箱：${linked.issueKey} (${linked.status})`}>
                              <CheckCircle2 size={12} /> 已进建议箱: {linked.issueKey}
                            </span>
                          ) : (
                            <button
                              type="button"
                              className="review-action-btn"
                              onClick={() => setSelectedIssueForSuggestion(issue)}
                              title="将此问题作为改进观察加入项目的建议箱"
                            >
                              <PlusCircle size={12} /> 加入建议
                            </button>
                          )}
                        </div>
                      </div>

                      <div className="review-card-prop">
                        <strong>观察到的行为事实:</strong>
                        <span>{issue.problem}</span>
                      </div>
                      <div className="review-card-prop">
                        <strong>产生的影响:</strong>
                        <span>{issue.impact}</span>
                      </div>
                      <div className="review-card-prop">
                        <strong>原因推测:</strong>
                        <span>{issue.speculation}</span>
                      </div>
                      <div className="review-card-prop">
                        <strong>改进建议:</strong>
                        <span>{issue.proposal}</span>
                      </div>
                      <div className="review-card-prop">
                        <strong>验证改善方法:</strong>
                        <span>{issue.acceptance}</span>
                      </div>

                      <div className="review-card-evidence">
                        <span style={{fontSize: 11, color: 'var(--text-muted, #94a3b8)'}}>证据引用:</span>
                        {issue.evidenceIds.map(evId => (
                          <span key={evId} className="review-evidence-tag">
                            {evId}
                          </span>
                        ))}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* Section 4: Good Practices */}
          <div className="review-section">
            <div className="review-section-title">4. 值得保留的做法 ({result?.goodPractices?.length || 0})</div>
            {(!result?.goodPractices || result.goodPractices.length === 0) ? (
              <div className="review-empty-tip">本次复盘未特别记录额外良好做法。</div>
            ) : (
              <div className="review-cards-list">
                {result.goodPractices.map((gp, idx) => (
                  <div key={gp.id || idx} className="review-card">
                    <div className="review-card-title">{idx + 1}. {gp.title}</div>
                    <div className="review-card-prop">{gp.description}</div>
                    {gp.evidenceIds && gp.evidenceIds.length > 0 && (
                      <div className="review-card-evidence">
                        <span style={{fontSize: 11, color: 'var(--text-muted, #94a3b8)'}}>证据依据:</span>
                        {gp.evidenceIds.map(evId => (
                          <span key={evId} className="review-evidence-tag">{evId}</span>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Section 5: Limitations */}
          <div className="review-section">
            <div className="review-section-title">5. 限制说明</div>
            {(!result?.limitations || result.limitations.length === 0) ? (
              <div className="review-empty-tip">本次分析证据完整，无显著限制。</div>
            ) : (
              <ul style={{fontSize: 13, color: 'var(--text-secondary, #94a3b8)', paddingLeft: 18, margin: 0, lineHeight: 1.6}}>
                {result.limitations.map((lim, idx) => (
                  <li key={idx}>{lim}</li>
                ))}
              </ul>
            )}
          </div>

          {/* Appendix: Evidence Snapshot */}
          <div className="review-section">
            <div className="review-section-title">附录：证据快照索引 ({currentReview.evidenceSnapshot?.length || 0})</div>
            <div className="review-evidence-list">
              {currentReview.evidenceSnapshot?.map(ev => (
                <div key={ev.id} className="review-evidence-row">
                  <span className="review-evidence-tag">{ev.id}</span>
                  <span style={{color: 'var(--text-muted, #64748b)'}}>({ev.kind}{ev.truncated ? ' · 已截断' : ''})</span>
                  <span style={{flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>
                    {ev.summary}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </ModalFrame>

      {selectedIssueForSuggestion && (
        <ReviewSuggestionModal
          reviewId={currentReview.id}
          issue={selectedIssueForSuggestion}
          onClose={() => setSelectedIssueForSuggestion(null)}
          onSuccess={handleSuggestionSuccess}
        />
      )}
    </>
  );
}
