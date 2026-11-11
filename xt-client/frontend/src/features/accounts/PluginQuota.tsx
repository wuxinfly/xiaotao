import {ChevronDown} from 'lucide-react';
import type {QuotaItem} from '../../lib/api';
import {getSummaryItem} from './quotaUtils';

type Aggregate = QuotaItem & {count: number};

function numeric(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

export function readQuotaItems(value: unknown): QuotaItem[] {
  if (!value || typeof value !== 'object') return [];
  const items = (value as {items?: unknown}).items;
  if (!Array.isArray(items)) return [];
  return items.flatMap(item => {
    if (!item || typeof item !== 'object') return [];
    const source = item as Record<string, unknown>;
    const label = typeof source.label === 'string' && source.label.trim() ? source.label.trim() : '额度';
    return [{label, unit: typeof source.unit === 'string' ? source.unit : 'credit', used: numeric(source.used), total: numeric(source.total), remaining: numeric(source.remaining)}];
  });
}

function amount(value: number): string {
  return value.toLocaleString('zh-CN', {maximumFractionDigits: 2});
}

function unitLabel(unit: string): string {
  if (unit === 'credit') return '积分';
  if (unit === 'dollar') return '美元';
  if (unit === 'percent') return '%';
  return unit || '额度';
}

function valueText(value: number, unit: string): string {
  if (unit === 'dollar') return `$${value.toFixed(2)}`;
  if (unit === 'percent') return `${value.toFixed(1)}%`;
  return `${amount(value)} ${unitLabel(unit)}`;
}

function percentText(used: number | null | undefined, total: number | null | undefined): string {
  if (used == null || total == null || total <= 0) return '—';
  return `${(Math.max(0, Math.min(1, used / total)) * 100).toLocaleString('zh-CN', {maximumFractionDigits: 1})}%`;
}

function ratio(used?: number | null, total?: number | null): number {
  if (used == null || total == null || total <= 0) return 0;
  return Math.max(0, Math.min(1, used / total));
}

export function aggregateQuotaItems(items: QuotaItem[]): Aggregate[] {
  const groups = new Map<string, Aggregate>();
  for (const item of items) {
    const existing = groups.get(item.label);
    if (!existing) {
      groups.set(item.label, {...item, count: 1});
      continue;
    }
    existing.count += 1;
    existing.used = existing.used == null || item.used == null ? null : existing.used + item.used;
    existing.total = existing.total == null || item.total == null ? null : existing.total + item.total;
    existing.remaining = existing.remaining == null || item.remaining == null ? null : existing.remaining + item.remaining;
  }
  return [...groups.values()];
}

export function QuotaLine({items, expanded, onToggle}: {items: QuotaItem[]; expanded: boolean; onToggle: () => void}) {
  const summary = getSummaryItem(items);
  const details = summary ? items.filter(item => item !== summary) : items;
  const used = summary?.used ?? null;
  const total = summary?.total ?? null;
  const known = used != null && total != null;
  return <>
    <div className="plugin-quota">
      <span className={`plugin-quota-track${known ? '' : ' is-empty'}`} aria-hidden="true">
        <span className="plugin-quota-fill" style={{width: `${ratio(used, total) * 100}%`}}/>
      </span>
      <span className="plugin-quota-text">
        {known ? `${amount(used)} / ${amount(total)} ${unitLabel(summary?.unit ?? 'credit')} · 已用 ${percentText(used, total)}` : '额度未知'}
      </span>
      {details.length > 0 && <button type="button" className="plugin-quota-toggle" aria-expanded={expanded} aria-label={expanded ? '收起额度明细' : '展开额度明细'} onClick={onToggle}>
        明细<ChevronDown size={12} aria-hidden="true"/>
      </button>}
    </div>
    {expanded && details.length > 0 && <ul className="plugin-quota-details" aria-label="额度明细">
      {aggregateQuotaItems(details).map(item => <li key={item.label}>
        <span className="plugin-quota-name">{item.label}{item.count > 1 && <small>×{item.count}</small>}</span>
        <span className="plugin-quota-track" aria-hidden="true"><span className="plugin-quota-fill" style={{width: `${ratio(item.used, item.total) * 100}%`}}/></span>
        <span className="plugin-quota-value">
          {item.used != null && item.total != null ? `${amount(item.used)} / ${valueText(item.total, item.unit)} · 已用 ${percentText(item.used, item.total)}` : item.remaining != null ? `剩余 ${valueText(item.remaining, item.unit)}` : '额度未知'}
        </span>
      </li>)}
    </ul>}
  </>;
}