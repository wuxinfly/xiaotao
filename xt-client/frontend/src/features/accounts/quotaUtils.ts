import type { QuotaItem, QuotaSnapshot } from '../../lib/api';

export interface QuotaSummary {
  hasQuota: boolean;
  remainingText: string;
  limitText: string;
  ratioText: string;
  percent: number;
  isKnown: boolean;
  isUnlimited: boolean;
}

export function formatNumber(n: number): string {
  return Math.round(n).toLocaleString('en-US');
}

export function formatUnitValue(value: number, unit: string): string {
  if (unit === 'dollar') {
    return `$${value.toFixed(2)}`;
  }
  if (unit === 'credit') {
    return `${formatNumber(value)} 积分`;
  }
  if (unit === 'call' || unit === 'request') {
    return `${formatNumber(value)} 次请求`;
  }
  if (unit === 'percent') {
    return `${value.toFixed(1)}%`;
  }
  return `${formatNumber(value)} ${unit || '配额'}`;
}

export function getSummaryItem(items: QuotaItem[]): QuotaItem | null {
  if (!items || items.length === 0) return null;
  const totalItem = items.find(it => it.label === '总额度' || it.label.includes('总额'));
  if (!totalItem && items.every(it => it.unit === 'credit') && items.some(it => it.label === '订阅额度' || it.label === '加购包')) {
    const sum = (key: 'used' | 'total' | 'remaining') => items.every(it => it[key] != null) ? items.reduce((n, it) => n + (it[key] ?? 0), 0) : null;
    return { label: '合计', unit: 'credit', used: sum('used'), total: sum('total'), remaining: sum('remaining') };
  }
  return totalItem || items[0];
}

export function getDetailItems(items: QuotaItem[]): QuotaItem[] {
  if (!items || items.length === 0) return [];
  const totalItem = items.find(it => it.label === '总额度' || it.label.includes('总额'));
  if (!totalItem) return items;
  return items.filter(it => it !== totalItem);
}

export function calculateQuotaSummary(quota?: QuotaSnapshot | null): QuotaSummary {
  if (!quota || quota.status !== 'ok' || !quota.items || quota.items.length === 0) {
    return {
      hasQuota: false,
      remainingText: '—',
      limitText: '—',
      ratioText: '—',
      percent: 0,
      isKnown: false,
      isUnlimited: false,
    };
  }

  const summary = getSummaryItem(quota.items);
  if (!summary || summary.used == null || summary.total == null) {
    return {
      hasQuota: true,
      remainingText: '—',
      limitText: '—',
      ratioText: '额度未知',
      percent: 0,
      isKnown: false,
      isUnlimited: false,
    };
  }

  const total = summary.total;
  const used = summary.used;
  const remaining = summary.remaining == null ? Math.max(0, total - used) : summary.remaining;

  if (total <= 0) {
    return {
      hasQuota: true,
      remainingText: '0',
      limitText: formatUnitValue(0, summary.unit),
      ratioText: '剩余 0%',
      percent: 0,
      isKnown: true,
      isUnlimited: false,
    };
  }

  const rawPercent = (remaining / total) * 100;
  const clamped = Math.max(0, Math.min(100, rawPercent));
  const ratioText = clamped >= 99.9 && clamped < 100
    ? '剩余 99.9%'
    : clamped > 0 && clamped < 1
      ? `剩余 ${clamped.toFixed(1)}%`
      : `剩余 ${Math.round(clamped)}%`;

  return {
    hasQuota: true,
    remainingText: summary.unit === 'dollar' ? `$${remaining.toFixed(2)}` : summary.unit === 'percent' ? `${remaining.toFixed(1)}%` : formatNumber(remaining),
    limitText: formatUnitValue(total, summary.unit),
    ratioText,
    percent: clamped,
    isKnown: true,
    isUnlimited: false,
  };
}

export function formatQuotaItemLine(item: QuotaItem): { usedText: string; limitText: string } {
  if (item.used == null || item.total == null) {
    return { usedText: '—', limitText: '额度未知' };
  }
  const rem = item.remaining == null ? Math.max(0, item.total - item.used) : item.remaining;
  const remStr = item.unit === 'dollar' ? `$${rem.toFixed(2)}` : item.unit === 'percent' ? `${rem.toFixed(1)}%` : formatNumber(rem);
  return {
    usedText: remStr,
    limitText: formatUnitValue(item.total, item.unit),
  };
}

export function formatRefreshTime(ts?: number): string {
  if (!ts) return '尚未刷新';
  const d = new Date(ts * 1000);
  const now = new Date();
  const isToday = d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  if (isToday) {
    return `今天 ${hh}:${mm}`;
  }
  const month = d.getMonth() + 1;
  const day = d.getDate();
  return `${month}月${day}日 ${hh}:${mm}`;
}
