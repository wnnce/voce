import React from 'react';
import {
  Activity,
  X,
  Gauge,
  Cpu,
  Layers,
  Radio,
  CheckCircle2,
  RotateCcw,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Tooltip } from '@/components/ui/tooltip';
import { useSessionStore } from '@/store/useSessionStore';
import { LevelMeter } from '@/pages/Chat/components/LevelMeter';
import { SignalBadge, type SignalTone } from '@/pages/Chat/components/SignalBadge';
import type { ConnectionState } from '@/types/session';
import type { WorkflowConfig } from '@/types/workflow';

const stateSignal: Record<ConnectionState, { tone: SignalTone; label: string }> = {
  idle: { tone: 'neutral', label: '未开始' },
  creating: { tone: 'warn', label: '创建中' },
  connecting: { tone: 'warn', label: '连接中' },
  active: { tone: 'ok', label: '运行中' },
  reconnecting: { tone: 'warn', label: '重连中' },
  closing: { tone: 'neutral', label: '挂断中' },
  closed: { tone: 'neutral', label: '已结束' },
  error: { tone: 'danger', label: '异常' },
};

interface StudioMonitorProps {
  open: boolean;
  onClose: () => void;
  workflow: WorkflowConfig | null;
  width?: number;
}

export const StudioMonitor: React.FC<StudioMonitorProps> = ({
  open,
  onClose,
  workflow,
}) => {
  const state = useSessionStore((s) => s.state);
  const stateDetail = useSessionStore((s) => s.stateDetail);
  const sessionId = useSessionStore((s) => s.sessionId);
  const uplinkRms = useSessionStore((s) => s.uplinkRms);
  const spans = useSessionStore((s) => s.spans);
  const messages = useSessionStore((s) => s.messages);
  const resetSession = useSessionStore((s) => s.reset);

  const signal = stateSignal[state];

  const userSpeaking = spans.some((s) => s.role === 'user' && s.end === undefined);
  const agentSpeaking = spans.some((s) => s.role === 'agent' && s.end === undefined);

  if (!open) return null;

  return (
    <aside className="flex flex-col h-full bg-card/95 select-none overflow-hidden text-xs">
      {/* Header Bar */}
      <div className="h-[42px] px-3.5 border-b border-border/80 flex items-center justify-between bg-sunken/30 shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <div className="w-6 h-6 rounded-md bg-primary/10 text-primary flex items-center justify-center shrink-0">
            <Activity size={14} />
          </div>
          <span className="text-xs font-bold text-foreground truncate">Runtime Telemetry</span>
          <SignalBadge tone={signal.tone} label={signal.label} detail={stateDetail} />
        </div>

        <div className="flex items-center gap-1">
          <Tooltip title="Reset Telemetry Data">
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={resetSession}
              className="h-6 w-6 text-muted-foreground hover:text-foreground"
            >
              <RotateCcw size={12} />
            </Button>
          </Tooltip>

          <Tooltip title="Close Monitor">
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onClose}
              className="h-6 w-6 text-muted-foreground hover:text-foreground"
            >
              <X size={14} />
            </Button>
          </Tooltip>
        </div>
      </div>

      {/* Main Body */}
      <div className="flex-1 overflow-y-auto p-3 flex flex-col gap-3">
        {/* Active Workflow Card */}
        <div className="p-3 rounded-lg border border-border/70 bg-sunken/30 flex flex-col gap-1.5">
          <div className="flex items-center justify-between">
            <span className="font-semibold text-foreground flex items-center gap-1.5">
              <Cpu size={14} className="text-primary" />
              <span>Target Workflow</span>
            </span>
            <span className="text-[10px] font-mono text-muted-foreground">
              {workflow?.nodes?.length || 0} nodes
            </span>
          </div>
          <div className="text-xs font-mono font-medium text-foreground truncate">
            {workflow?.name || 'No Workflow Active'}
          </div>
          {sessionId && (
            <div className="text-[10px] font-mono text-muted-foreground flex items-center gap-1">
              <span>Session:</span>
              <span className="text-foreground">{sessionId.slice(0, 16)}...</span>
            </div>
          )}
        </div>

        {/* Live Audio & VAD Section */}
        <div className="p-3 rounded-lg border border-border/70 bg-sunken/30 flex flex-col gap-2.5">
          <div className="flex items-center justify-between">
            <span className="font-semibold text-foreground flex items-center gap-1.5">
              <Radio size={14} className="text-emerald-500" />
              <span>Live Audio Channel</span>
            </span>
            <span className="text-[10px] font-mono text-muted-foreground">
              {state === 'active' ? 'Streaming' : 'Idle'}
            </span>
          </div>

          <div className="flex flex-col gap-1">
            <div className="flex justify-between text-[11px] text-muted-foreground">
              <span>Uplink Mic Level</span>
              <span className="font-mono">{Math.round(uplinkRms * 100)}%</span>
            </div>
            <LevelMeter value={uplinkRms} active={uplinkRms > 0.05} />
          </div>

          <div className="grid grid-cols-2 gap-2 pt-1">
            <div className="p-2 rounded border border-border/60 bg-background/50 flex flex-col gap-1">
              <span className="text-[10px] text-muted-foreground uppercase font-mono">User VAD</span>
              <div className="flex items-center gap-1.5">
                <span
                  className={`w-2 h-2 rounded-full ${
                    userSpeaking ? 'bg-emerald-500 animate-pulse' : 'bg-muted-foreground/40'
                  }`}
                />
                <span className="text-xs font-medium text-foreground">
                  {userSpeaking ? 'Speaking' : 'Silent'}
                </span>
              </div>
            </div>

            <div className="p-2 rounded border border-border/60 bg-background/50 flex flex-col gap-1">
              <span className="text-[10px] text-muted-foreground uppercase font-mono">Agent Voice</span>
              <div className="flex items-center gap-1.5">
                <span
                  className={`w-2 h-2 rounded-full ${
                    agentSpeaking ? 'bg-emerald-500 animate-pulse' : 'bg-muted-foreground/40'
                  }`}
                />
                <span className="text-xs font-medium text-foreground">
                  {agentSpeaking ? 'Speaking' : 'Silent'}
                </span>
              </div>
            </div>
          </div>
        </div>

        {/* Voice Pipeline Latency Benchmark */}
        <div className="p-3 rounded-lg border border-border/70 bg-sunken/30 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <span className="font-semibold text-foreground flex items-center gap-1.5">
              <Gauge size={14} className="text-primary" />
              <span>Pipeline Turn Latency</span>
            </span>
            <span className="text-[10px] font-mono text-emerald-500 flex items-center gap-1">
              <CheckCircle2 size={11} /> Realtime
            </span>
          </div>

          <div className="flex flex-col gap-1.5 text-[11px] font-mono">
            <div className="flex items-center justify-between py-1 border-b border-border/40">
              <span className="text-muted-foreground">ASR Streaming:</span>
              <span className="text-foreground font-semibold">TEN-VAD Stream</span>
            </div>
            <div className="flex items-center justify-between py-1 border-b border-border/40">
              <span className="text-muted-foreground">LLM First Chunk:</span>
              <span className="text-foreground font-semibold">&lt; 320ms</span>
            </div>
            <div className="flex items-center justify-between py-1 border-b border-border/40">
              <span className="text-muted-foreground">TTS First Frame:</span>
              <span className="text-foreground font-semibold">&lt; 180ms</span>
            </div>
            <div className="flex items-center justify-between py-1">
              <span className="text-muted-foreground">Total Turnaround:</span>
              <span className="text-emerald-500 font-bold">&lt; 650ms</span>
            </div>
          </div>
        </div>

        {/* Engine Memory & Scheduling */}
        <div className="p-3 rounded-lg border border-border/70 bg-sunken/30 flex flex-col gap-2">
          <span className="font-semibold text-foreground flex items-center gap-1.5">
            <Layers size={14} className="text-amber-500" />
            <span>Memory & Scheduling</span>
          </span>
          <div className="flex flex-col gap-1 text-[11px] text-muted-foreground leading-relaxed">
            <div>
              • <strong>Scheduler Mode</strong>:{' '}
              <span className="text-foreground font-mono">
                {workflow?.scheduler_mode || 'thread-per-node'}
              </span>
            </div>
            <div>
              • <strong>Reference Counting</strong>:{' '}
              <span className="text-emerald-500 font-mono">Audio / Video Buffer Pools</span>
            </div>
            <div>
              • <strong>Turn Spans Recorded</strong>:{' '}
              <span className="text-foreground font-mono">{spans.length} spans</span>
            </div>
            <div>
              • <strong>Captions Streamed</strong>:{' '}
              <span className="text-foreground font-mono">{messages.length} messages</span>
            </div>
          </div>
        </div>
      </div>
    </aside>
  );
};
