import { useEffect, useMemo, useRef, useState } from 'react';
import type { HeatmapData, HeatmapDay } from '../../lib/api';
import { formatNumber } from './UsageOverview';

interface Props {
  heatmap: HeatmapData;
}

interface PopoverState {
  visible: boolean;
  x: number;
  y: number;
  dateStr: string;
  dayName: string;
  data: HeatmapDay | null;
  isUntracked: boolean;
}

const DAY_NAMES = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'];

export function UsageHeatmap({ heatmap }: Props) {
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const [popover, setPopover] = useState<PopoverState | null>(null);

  // Auto-scroll to latest date on initial render
  useEffect(() => {
    const el = scrollContainerRef.current;
    if (el) {
      el.scrollLeft = el.scrollWidth;
    }
  }, []);

  // Calculate 4 quantile thresholds from all positive token days
  const thresholds = useMemo(() => {
    const positiveValues: number[] = [];
    for (const d of Object.values(heatmap.days || {})) {
      if (d.totalTokens > 0) {
        positiveValues.push(d.totalTokens);
      }
    }
    if (positiveValues.length === 0) {
      return [1000, 5000, 20000, 50000];
    }
    positiveValues.sort((a, b) => a - b);
    const q1 = positiveValues[Math.floor(positiveValues.length * 0.25)];
    const q2 = positiveValues[Math.floor(positiveValues.length * 0.50)];
    const q3 = positiveValues[Math.floor(positiveValues.length * 0.75)];
    const q4 = positiveValues[positiveValues.length - 1];

    // Ensure strict ascending order to handle repeated values
    let t1 = q1;
    let t2 = Math.max(q2, t1 + 1);
    let t3 = Math.max(q3, t2 + 1);
    let t4 = Math.max(q4, t3 + 1);
    return [t1, t2, t3, t4];
  }, [heatmap.days]);

  // Generate 53 weeks matrix ending at Today
  const { weeks, monthLabels, todayStr } = useMemo(() => {
    const today = new Date();
    // Format YYYY-MM-DD in local time
    const formatYMD = (d: Date) => {
      const y = d.getFullYear();
      const m = String(d.getMonth() + 1).padStart(2, '0');
      const day = String(d.getDate()).padStart(2, '0');
      return `${y}-${m}-${day}`;
    };

    const todayStr = formatYMD(today);

    // Find end week Sunday (or today)
    // In JS: 0=Sun, 1=Mon, ..., 6=Sat
    // Our row 0=Mon, ..., 6=Sun
    const getRow = (d: Date) => (d.getDay() + 6) % 7;

    // We want 53 weeks ending with this week
    const currentWeekEnd = new Date(today);
    const currentDayRow = getRow(today);
    // Saturday or Sunday of this week: let's end on Sunday of current week
    currentWeekEnd.setDate(today.getDate() + (6 - currentDayRow));

    const startDate = new Date(currentWeekEnd);
    startDate.setDate(currentWeekEnd.getDate() - 53 * 7 + 1); // 53 weeks before Monday

    const matrix: Array<Array<{ date: Date; dateStr: string; invisible: boolean; isToday: boolean }>> = [];
    const months: Array<{ col: number; label: string }> = [];

    let lastMonth = -1;
    const cursor = new Date(startDate);

    for (let w = 0; w < 53; w++) {
      const weekCols: Array<{ date: Date; dateStr: string; invisible: boolean; isToday: boolean }> = [];
      for (let r = 0; r < 7; r++) {
        const cellDate = new Date(cursor);
        const ymd = formatYMD(cellDate);
        const isFuture = cellDate > today;
        const isTooOld = ymd < heatmap.startDate;

        // Detect month transitions
        const m = cellDate.getMonth();
        if (r === 0 && m !== lastMonth && !isFuture && !isTooOld) {
          months.push({ col: w, label: `${m + 1}月` });
          lastMonth = m;
        }

        weekCols.push({
          date: cellDate,
          dateStr: ymd,
          invisible: isFuture || isTooOld,
          isToday: ymd === todayStr,
        });

        cursor.setDate(cursor.getDate() + 1);
      }
      matrix.push(weekCols);
    }

    return { weeks: matrix, monthLabels: months, todayStr };
  }, [heatmap.startDate]);

  const getCellLevel = (dateStr: string): string => {
    const trackingStart = heatmap.trackingStartDate || heatmap.endDate;
    if (dateStr < trackingStart) {
      return 'untracked';
    }
    const day = heatmap.days?.[dateStr];
    if (!day || day.totalTokens <= 0) {
      return 'level-0';
    }
    const val = day.totalTokens;
    if (val <= thresholds[0]) return 'level-1';
    if (val <= thresholds[1]) return 'level-2';
    if (val <= thresholds[2]) return 'level-3';
    return 'level-4';
  };

  const handleCellHover = (e: React.MouseEvent<HTMLDivElement> | React.FocusEvent<HTMLDivElement>, cellDateStr: string, invisible: boolean) => {
    if (invisible) {
      setPopover(null);
      return;
    }
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const d = new Date(cellDateStr + 'T00:00:00');
    const dayOfWeek = DAY_NAMES[(d.getDay() + 6) % 7];
    const trackingStart = heatmap.trackingStartDate || heatmap.endDate;
    const isUntracked = cellDateStr < trackingStart;
    const data = heatmap.days?.[cellDateStr] || null;

    // Position tooltip above cell
    let x = rect.left + rect.width / 2;
    let y = rect.top - 8;

    // Flip below if too close to top
    if (y < 120) {
      y = rect.bottom + 8;
    }
    // Shift left if too close to right edge
    if (x + 130 > window.innerWidth) {
      x = window.innerWidth - 130;
    }

    setPopover({
      visible: true,
      x,
      y,
      dateStr: cellDateStr,
      dayName: dayOfWeek,
      data,
      isUntracked,
    });
  };

  return (
    <section className="heatmap-section" aria-label="用量热力图">
      <div className="heatmap-header">
        <div className="heatmap-title-wrap">
          <h2>用量热力图</h2>
          <span className="heatmap-subtitle">最近一年 · 每日总 Token</span>
        </div>

        <div className="heatmap-legend" aria-label="热力图图例">
          <span>少</span>
          <div className="legend-cell level-0" title="0 用量" />
          <div className="legend-cell level-1" title="较低" />
          <div className="legend-cell level-2" title="中等" />
          <div className="legend-cell level-3" title="较高" />
          <div className="legend-cell level-4" title="最高" />
          <span>多</span>
        </div>
      </div>

      <div className="heatmap-scroll-container" ref={scrollContainerRef}>
        <div className="heatmap-grid-layout">
          {/* Left Day of Week Labels */}
          <div className="day-labels" aria-hidden="true">
            <span />
            <span>周一</span>
            <span />
            <span>周三</span>
            <span />
            <span>周五</span>
            <span />
            <span />
          </div>

          {/* Matrix & Month Labels */}
          <div className="weeks-container">
            {/* Month row */}
            <div className="month-labels" aria-hidden="true">
              {monthLabels.map((m, idx) => (
                <span
                  key={idx}
                  className="month-label-item"
                  style={{ gridColumnStart: m.col + 1 }}
                >
                  {m.label}
                </span>
              ))}
            </div>

            {/* 53 x 7 Calendar Grid */}
            <div
              className="calendar-matrix"
              role="grid"
              aria-label="年度每日用量日历"
              onMouseLeave={() => setPopover(null)}
            >
              {weeks.map((week, wIdx) =>
                week.map((cell, rIdx) => {
                  if (cell.invisible) {
                    return <div key={`${wIdx}-${rIdx}`} className="day-cell invisible" aria-hidden="true" />;
                  }
                  const levelClass = getCellLevel(cell.dateStr);
                  const isTodayClass = cell.isToday ? 'is-today' : '';

                  return (
                    <div
                      key={`${wIdx}-${rIdx}`}
                      role="gridcell"
                      tabIndex={0}
                      className={`day-cell ${levelClass} ${isTodayClass}`}
                      aria-label={`${cell.dateStr} ${DAY_NAMES[rIdx]}`}
                      onMouseEnter={(e) => handleCellHover(e, cell.dateStr, false)}
                      onFocus={(e) => handleCellHover(e, cell.dateStr, false)}
                      onBlur={() => setPopover(null)}
                      onKeyDown={(e) => {
                        if (e.key === 'Escape') setPopover(null);
                      }}
                    />
                  );
                })
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Floating Popover Tooltip */}
      {popover && popover.visible && (
        <div
          className="heatmap-popover"
          role="tooltip"
          style={{ left: `${popover.x}px`, top: `${popover.y}px`, transform: 'translate(-50%, -100%)' }}
        >
          <div className="popover-date">
            {popover.dateStr} {popover.dayName}
          </div>
          {popover.isUntracked ? (
            <div className="popover-row">
              <span>历史状态</span>
              <span className="popover-val">未开启统计</span>
            </div>
          ) : (
            <>
              <div className="popover-row highlight">
                <span>总 Token</span>
                <span className="popover-val">{formatNumber(popover.data?.totalTokens || 0)}</span>
              </div>
              <div className="popover-row">
                <span>输入 Token</span>
                <span className="popover-val">{formatNumber(popover.data?.promptTokens || 0)}</span>
              </div>
              <div className="popover-row">
                <span>输出 Token</span>
                <span className="popover-val">{formatNumber(popover.data?.completionTokens || 0)}</span>
              </div>
              <div className="popover-row">
                <span>缓存命中</span>
                <span className="popover-val">{formatNumber(popover.data?.cachedTokens || 0)}</span>
              </div>
              <div className="popover-row">
                <span>请求次数</span>
                <span className="popover-val">{popover.data?.totalRequests || 0} 次</span>
              </div>
              {popover.data?.hasMissingUsage && (
                <div className="popover-row" style={{ color: '#fca5a5' }}>
                  <span>提示</span>
                  <span>包含用量缺失请求</span>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </section>
  );
}
