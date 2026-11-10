import {useEffect, useState} from 'react';
import {Clock3, FilePenLine, FileSearch, Gauge, SendHorizontal, Wrench, Zap} from 'lucide-react';
import {activityState, type DropletState} from '../AgentDroplet';
import type {AgentActivity, AgentRunStats, AgentThought} from '../../../lib/api';

export const thoughtKindLabel: Record<AgentThought['kind'], string> = {
  understanding: '理解',
  observation: '进展',
  decision: '决定',
  checkpoint: '阶段',
};

export function runPhase(stats: AgentRunStats): NonNullable<AgentRunStats['phase']> {
  return stats.phase ?? (stats.endedAt ? 'completed' : 'working');
}

export function runDuration(stats: AgentRunStats) {
  const start = new Date(stats.startedAt).getTime();
  const end = stats.endedAt ? new Date(stats.endedAt).getTime() : Date.now();
  const seconds = Math.max(0, Math.round((end - start) / 1000));
  return seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`;
}

export function runSpeed(stats: AgentRunStats) {
  if (!stats.outputSpeed || stats.outputSpeed <= 0) return '';
  return `${stats.outputSpeed.toFixed(1).replace(/\.0$/, '')} tok/s`;
}

// runElapsed is the wall clock time of the run, in whole seconds.
export function runElapsed(stats: AgentRunStats, now: number) {
  const start = new Date(stats.startedAt).getTime();
  if (!Number.isFinite(start)) return 0;
  const end = stats.endedAt ? new Date(stats.endedAt).getTime() : now;
  return Math.max(0, Math.floor((end - start) / 1000));
}

export function formatElapsed(seconds: number) {
  if (seconds < 60) return `${seconds} 秒`;
  return `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`;
}

// useTicker advances once per second so the live duration keeps moving even
// when the backend has nothing new to report.
export function useTicker(active: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active]);
  return now;
}

// liveOutput reads the streaming values first and falls back to the values
// reported by a finished request.
export function liveOutput(stats: AgentRunStats) {
  const tokens = (stats.liveTokens ?? 0) > 0 ? stats.liveTokens! : (stats.outputTokens ?? 0);
  const speed = (stats.liveSpeed ?? 0) > 0 ? stats.liveSpeed! : (stats.outputSpeed ?? 0);
  return {tokens, speed};
}

export function runSummaryItems(stats: AgentRunStats) {
  const items = [{key: 'duration', label: `共思考 ${runDuration(stats)}`, Icon: Clock3, value: runDuration(stats)}];
  if (stats.firstTokenMs != null && stats.firstTokenMs > 0) {
    const value = `${(stats.firstTokenMs / 1000).toFixed(1)} 秒`;
    items.push({key: 'firstToken', label: `首 token 延迟 ${value}`, Icon: Zap, value});
  }
  if (stats.filesRead > 0) items.push({key: 'read', label: `读取 ${stats.filesRead} 个文件`, Icon: FileSearch, value: String(stats.filesRead)});
  if (stats.filesWritten > 0) items.push({key: 'write', label: `写入 ${stats.filesWritten} 个文件`, Icon: FilePenLine, value: String(stats.filesWritten)});
  if (stats.toolCalls > 0) items.push({key: 'tools', label: `调用 ${stats.toolCalls} 次工具`, Icon: Wrench, value: String(stats.toolCalls)});
  if ((stats.modelRequests ?? 0) > 0) items.push({key: 'requests', label: `模型请求 ${stats.modelRequests} 次`, Icon: SendHorizontal, value: String(stats.modelRequests)});
  const speed = runSpeed(stats);
  if (speed) items.push({key: 'speed', label: `输出速度 ${speed}`, Icon: Gauge, value: speed.replace(' tok/s', '')});
  return items;
}

export function liveRunState(runId: string, thoughts: AgentThought[], activity: AgentActivity[]): DropletState {
  const latestActivity = activity.filter(item => item.runId === runId).sort((a, b) => a.startedAt.localeCompare(b.startedAt)).at(-1);
  const latestThought = thoughts.filter(item => item.runId === runId).sort((a, b) => a.createdAt.localeCompare(b.createdAt)).at(-1);
  if (!latestActivity || (latestThought && latestThought.createdAt >= latestActivity.startedAt)) return 'thinking';
  if (latestActivity.status === 'failed') return 'error';
  return activityState(latestActivity.kind);
}

export function formatCompactTokens(value: number) {
  if (value >= 1_000_000) return (value / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  if (value >= 1_000) return (value / 1_000).toFixed(1).replace(/\.0$/, '') + 'K';
  return String(value);
}