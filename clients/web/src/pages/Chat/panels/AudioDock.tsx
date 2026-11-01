import { Mic, MicOff, PhoneOff } from 'lucide-react';
import { LevelMeter } from '@/pages/Chat/components/LevelMeter';
import { useSessionStore } from '@/store/useSessionStore';
import { Button } from '@/components/ui/button';
import type { ConnectionState } from '@/types/session';
import { cn } from '@/lib/utils';

interface AudioDockProps {
  onStart: () => void;
  onStop: () => void;
  busy: boolean;
  state: ConnectionState;
  agentSpeaking: boolean;
}

const stateCopy: Record<ConnectionState, string> = {
  idle: '未开始',
  creating: '正在创建会话…',
  connecting: '正在连接…',
  active: '已连接',
  reconnecting: '连接中断，正在重连…',
  closing: '正在挂断…',
  closed: '已挂断',
  error: '出错了',
};

export function AudioDock({ onStart, onStop, busy, state, agentSpeaking }: AudioDockProps) {
  const workflowId = useSessionStore((s) => s.config.workflowId);
  const uplinkRms = useSessionStore((s) => s.uplinkRms);
  const sessionId = useSessionStore((s) => s.sessionId);
  const canStart = !busy && Boolean(workflowId);

  return (
    <div className="px-5 py-3 border-t border-border bg-card/95 backdrop-blur-md flex items-center gap-4 shrink-0 shadow-xs">
      <div className="flex items-center gap-3">
        <div
          className={cn(
            'w-10 h-10 rounded-xl flex items-center justify-center border transition-colors',
            busy
              ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-500'
              : 'bg-sunken border-border text-muted-foreground'
          )}
          aria-hidden
        >
          {busy ? <Mic size={18} /> : <MicOff size={18} />}
        </div>

        <div className="flex flex-col">
          <span className="text-xs font-semibold text-foreground">{stateCopy[state]}</span>
          <div className="flex items-center gap-2 mt-0.5">
            <LevelMeter value={uplinkRms} active={busy && !agentSpeaking} height={12} bars={10} />
            <span className="text-[10px] font-mono text-muted-foreground">
              {sessionId ? `sid ${sessionId.slice(0, 8)}` : '未分配会话'}
            </span>
          </div>
        </div>
      </div>

      <div className="flex-1" />

      <div>
        {busy ? (
          <Button
            variant="destructive"
            onClick={onStop}
            className="gap-1.5 h-8 px-4 text-xs font-semibold"
          >
            <PhoneOff size={14} /> 挂断
          </Button>
        ) : (
          <Button
            variant="default"
            onClick={onStart}
            disabled={!canStart}
            className="gap-1.5 h-8 px-4 text-xs font-semibold"
          >
            <Mic size={14} /> 开始对话
          </Button>
        )}
      </div>
    </div>
  );
}
