import {useEffect, useState, useCallback} from 'react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {api, type BehaviorReview, type ReviewSummary} from '../../lib/api';
import {Download, Eye, RotateCw, Trash2, XCircle, Loader2} from 'lucide-react';
import {ReviewDetailModal} from './ReviewDetailModal';
import './review.css';

interface ReviewListModalProps {
  currentSessionId?: string;
  onClose: () => void;
  onOpenDetail?: (review: BehaviorReview) => void;
}

export function ReviewListModal({currentSessionId, onClose}: ReviewListModalProps) {
  const [filterMode, setFilterMode] = useState<'current' | 'all'>(currentSessionId ? 'current' : 'all');
  const [reviews, setReviews] = useState<ReviewSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [nextCursor, setNextCursor] = useState<string | undefined>(undefined);
  const [actionError, setActionError] = useState('');
  const [activeDetail, setActiveDetail] = useState<BehaviorReview | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const fetchReviews = useCallback(async (reset = true) => {
    if (reset) {
      setLoading(true);
      setActionError('');
    } else {
      setLoadingMore(true);
    }

    try {
      const sessionId = filterMode === 'current' ? currentSessionId : undefined;
      const cursor = reset ? undefined : nextCursor;
      const res = await api.listReviews(sessionId, cursor, 20);
      setReviews(prev => (reset ? res.reviews : [...prev, ...res.reviews]));
      setNextCursor(res.nextCursor);
    } catch (err: any) {
      setActionError(err?.message || '读取复盘记录失败');
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  }, [filterMode, currentSessionId, nextCursor]);

  useEffect(() => {
    void fetchReviews(true);
  }, [filterMode, currentSessionId]);

  // Periodic poll if any item is pending/running
  useEffect(() => {
    const hasActive = reviews.some(r => r.status === 'pending' || r.status === 'running');
    if (!hasActive) return;

    const interval = setInterval(() => {
      void fetchReviews(true);
    }, 3000);
    return () => clearInterval(interval);
  }, [reviews, fetchReviews]);

  const handleOpenDetail = async (id: string) => {
    setActionError('');
    try {
      const detail = await api.getReview(id);
      setActiveDetail(detail);
    } catch (err: any) {
      setActionError(err?.message || '读取复盘详情失败');
    }
  };

  const handleCancel = async (id: string) => {
    try {
      await api.cancelReview(id);
      void fetchReviews(true);
    } catch (err: any) {
      setActionError(err?.message || '取消复盘失败');
    }
  };

  const handleRetry = async (id: string) => {
    try {
      await api.retryReview(id);
      void fetchReviews(true);
    } catch (err: any) {
      setActionError(err?.message || '重试复盘失败');
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.deleteReview(id);
      setDeletingId(null);
      void fetchReviews(true);
    } catch (err: any) {
      setActionError(err?.message || '删除复盘失败');
    }
  };

  const formatDateTime = (iso: string, tz: string) => {
    try {
      const d = new Date(iso);
      return d.toLocaleString('zh-CN', {timeZone: tz || 'Asia/Shanghai', hour12: false});
    } catch {
      return iso;
    }
  };

  return (
    <>
      <ModalFrame
        title="复盘记录"
        className="review-modal"
        onClose={onClose}
        footer={
          <div className="app-modal-actions">
            <button type="button" className="app-modal-button secondary" onClick={onClose}>
              关闭
            </button>
          </div>
        }
      >
        <div className="review-list-filters">
          {currentSessionId && (
            <button
              type="button"
              className={`review-filter-btn ${filterMode === 'current' ? 'active' : ''}`}
              onClick={() => setFilterMode('current')}
            >
              当前会话记录
            </button>
          )}
          <button
            type="button"
            className={`review-filter-btn ${filterMode === 'all' ? 'active' : ''}`}
            onClick={() => setFilterMode('all')}
          >
            全部记录
          </button>
        </div>

        {actionError && <div className="agent-modal-error" role="alert">{actionError}</div>}

        {loading ? (
          <div className="review-empty-tip">正在加载复盘记录…</div>
        ) : reviews.length === 0 ? (
          <div className="review-empty-tip">暂无复盘记录</div>
        ) : (
          <div className="review-list">
            {reviews.map(item => (
              <div key={item.id} className="review-item">
                <div className="review-item-main">
                  <div className="review-item-header">
                    <span className="review-item-title">{item.title}</span>
                    <span className={`review-status-badge ${item.status}`}>
                      {item.status === 'running' || item.status === 'pending' ? (
                        <>
                          <Loader2 size={12} className="spin" /> 复盘中
                        </>
                      ) : item.status === 'succeeded' ? (
                        '成功'
                      ) : item.status === 'canceled' ? (
                        '已取消'
                      ) : (
                        '失败'
                      )}
                    </span>
                  </div>

                  <div className="review-item-meta">
                    <span>{formatDateTime(item.createdAt, item.timezone)}</span>
                    <span>模型: {item.model}</span>
                    {item.sessionTitleSnapshot && filterMode === 'all' && (
                      <span>原会话: {item.sessionTitleSnapshot}</span>
                    )}
                    {item.status === 'failed' && item.errorMessage && (
                      <span style={{color: '#f87171'}}>({item.errorMessage})</span>
                    )}
                  </div>
                </div>

                <div className="review-item-actions">
                  {item.status === 'succeeded' && (
                    <>
                      <button
                        type="button"
                        className="review-action-btn"
                        onClick={() => void handleOpenDetail(item.id)}
                        title="查看详情"
                      >
                        <Eye size={13} /> 详情
                      </button>
                      <button
                        type="button"
                        className="review-action-btn"
                        onClick={() => window.open(`/api/agent/reviews/${encodeURIComponent(item.id)}/export`, '_blank')}
                        title="导出 Markdown"
                      >
                        <Download size={13} /> 导出
                      </button>
                    </>
                  )}

                  {(item.status === 'running' || item.status === 'pending') && (
                    <button
                      type="button"
                      className="review-action-btn danger"
                      onClick={() => void handleCancel(item.id)}
                      title="取消本次复盘"
                    >
                      <XCircle size={13} /> 取消
                    </button>
                  )}

                  {(item.status === 'failed' || item.status === 'canceled') && (
                    <button
                      type="button"
                      className="review-action-btn"
                      onClick={() => void handleRetry(item.id)}
                      title="使用原脱敏快照重试复盘"
                    >
                      <RotateCw size={13} /> 重试
                    </button>
                  )}

                  {(item.status === 'succeeded' || item.status === 'failed' || item.status === 'canceled') && (
                    deletingId === item.id ? (
                      <div style={{display: 'inline-flex', gap: 4, alignItems: 'center'}}>
                        <button
                          type="button"
                          className="review-action-btn danger"
                          onClick={() => void handleDelete(item.id)}
                        >
                          确认删
                        </button>
                        <button
                          type="button"
                          className="review-action-btn"
                          onClick={() => setDeletingId(null)}
                        >
                          取消
                        </button>
                      </div>
                    ) : (
                      <button
                        type="button"
                        className="review-action-btn danger"
                        onClick={() => setDeletingId(item.id)}
                        title="删除记录（不可恢复，已导出的文件和已加入建议箱的证据将保留）"
                      >
                        <Trash2 size={13} />
                      </button>
                    )
                  )}
                </div>
              </div>
            ))}

            {nextCursor && (
              <div style={{textAlign: 'center', marginTop: 10}}>
                <button
                  type="button"
                  className="review-action-btn"
                  disabled={loadingMore}
                  onClick={() => void fetchReviews(false)}
                >
                  {loadingMore ? '正在加载…' : '加载更多'}
                </button>
              </div>
            )}
          </div>
        )}
      </ModalFrame>

      {activeDetail && (
        <ReviewDetailModal
          review={activeDetail}
          onClose={() => setActiveDetail(null)}
          onUpdate={updated => {
            setActiveDetail(updated);
            void fetchReviews(true);
          }}
        />
      )}
    </>
  );
}
