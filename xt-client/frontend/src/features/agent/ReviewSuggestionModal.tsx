import {useEffect, useState} from 'react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {api, type ExistingSuggestionItem, type ReviewIssue, type ReviewSuggestionLink} from '../../lib/api';
import './review.css';

interface ReviewSuggestionModalProps {
  reviewId: string;
  issue: ReviewIssue;
  onClose: () => void;
  onSuccess: (link: ReviewSuggestionLink) => void;
}

export function ReviewSuggestionModal({reviewId, issue, onClose, onSuccess}: ReviewSuggestionModalProps) {
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [existing, setExisting] = useState<ExistingSuggestionItem[]>([]);
  const [mode, setMode] = useState<'new' | 'existing'>('new');
  const [selectedKey, setSelectedKey] = useState<string>('');

  useEffect(() => {
    let active = true;
    api.getReviewProjectSuggestions(reviewId)
      .then(res => {
        if (!active) return;
        const list = res.suggestions || [];
        setExisting(list);
        if (list.length > 0) {
          // Keep default to 'new' or let user switch
        }
      })
      .catch(err => {
        if (!active) return;
        console.warn('Failed to load project suggestions:', err);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [reviewId]);

  const handleSubmit = async () => {
    if (submitting) return;
    setSubmitting(true);
    setError('');
    try {
      const issueKey = mode === 'existing' ? selectedKey : undefined;
      const link = await api.addIssueSuggestion(reviewId, issue.id, issueKey);
      onSuccess(link);
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '加入建议箱失败，请重试');
    } finally {
      setSubmitting(false);
    }
  };

  const isSubmitDisabled = submitting || (mode === 'existing' && !selectedKey);

  return (
    <ModalFrame
      title="加入改进建议箱"
      className="review-modal"
      onClose={() => {
        if (!submitting) onClose();
      }}
      footer={
        <div className="app-modal-actions">
          <button
            type="button"
            className="app-modal-button secondary"
            disabled={submitting}
            onClick={onClose}
          >
            取消
          </button>
          <button
            type="button"
            className="app-modal-button primary"
            disabled={isSubmitDisabled}
            onClick={() => void handleSubmit()}
          >
            {submitting ? '提交中…' : '确认加入'}
          </button>
        </div>
      }
    >
      <div style={{display: 'flex', flexDirection: 'column', gap: 14}}>
        <div style={{fontSize: 13, color: 'var(--text-secondary, #94a3b8)', lineHeight: 1.5}}>
          将问题 <strong>「{issue.title}」</strong> 的脱敏事实快照保存至项目的 <code>.xiaotao/review-evidence</code>，并写入建议箱。该操作不会自动修改代码或规则，待处理状态由设计 AI 或人工评估。
        </div>

        <div style={{display: 'flex', gap: 16}}>
          <label style={{display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, cursor: 'pointer'}}>
            <input
              type="radio"
              name="suggest-mode"
              checked={mode === 'new'}
              onChange={() => setMode('new')}
            />
            创建新建议
          </label>
          <label
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              fontSize: 13,
              cursor: existing.length > 0 ? 'pointer' : 'not-allowed',
              opacity: existing.length > 0 ? 1 : 0.5,
            }}
          >
            <input
              type="radio"
              name="suggest-mode"
              disabled={existing.length === 0}
              checked={mode === 'existing'}
              onChange={() => setMode('existing')}
            />
            关联已有同根因建议 ({existing.length})
          </label>
        </div>

        {mode === 'existing' && (
          <div className="suggestion-existing-list">
            {existing.map(item => (
              <div
                key={item.issueKey}
                className={`suggestion-item-radio ${selectedKey === item.issueKey ? 'selected' : ''}`}
                onClick={() => setSelectedKey(item.issueKey)}
              >
                <input
                  type="radio"
                  name="existing-item"
                  checked={selectedKey === item.issueKey}
                  onChange={() => setSelectedKey(item.issueKey)}
                />
                <div style={{display: 'flex', flexDirection: 'column', gap: 2, flex: 1}}>
                  <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center'}}>
                    <strong style={{fontSize: 13, color: 'var(--text-primary, #f1f5f9)'}}>{item.title}</strong>
                    <span style={{fontSize: 11, color: 'var(--text-muted, #64748b)'}}>
                      已观察 {item.observationsCount} 次 · {item.status}
                    </span>
                  </div>
                  <div style={{fontSize: 12, color: 'var(--text-muted, #94a3b8)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>
                    {item.problem}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        {loading && <div style={{fontSize: 12, color: 'var(--text-muted, #64748b)'}}>正在加载项目建议列表…</div>}
        {error && <div className="agent-modal-error" role="alert">{error}</div>}
      </div>
    </ModalFrame>
  );
}
