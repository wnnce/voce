import { ArrowLeft, Moon, Sun } from 'lucide-react';
import { useEffect } from 'react';
import { Link as RouterLink, useSearchParams } from 'react-router-dom';
import { SignalBadge, type SignalTone } from '@/pages/Chat/components/SignalBadge';
import { AudioDock } from '@/pages/Chat/panels/AudioDock';
import { ConversationStream } from '@/pages/Chat/panels/ConversationStream';
import { SessionConfigPanel } from '@/pages/Chat/panels/SessionConfigPanel';
import { useSessionController } from '@/pages/Chat/hooks/useSessionController';
import { useSessionStore } from '@/store/useSessionStore';
import { useThemeControl } from '@/hooks/useThemeControl';
import { Button } from '@/components/ui/button';
import { Tooltip } from '@/components/ui/tooltip';
import type { ConnectionState } from '@/types/session';

const stateSignal: Record<ConnectionState, { tone: SignalTone; label: string }> = {
  idle: { tone: 'neutral', label: '未开始' },
  creating: { tone: 'warn', label: '创建会话' },
  connecting: { tone: 'warn', label: '连接中' },
  active: { tone: 'ok', label: '已连接' },
  reconnecting: { tone: 'warn', label: '重连中' },
  closing: { tone: 'neutral', label: '挂断中' },
  closed: { tone: 'neutral', label: '已挂断' },
  error: { tone: 'danger', label: '错误' },
};

export default function ChatPage() {
  const [params] = useSearchParams();
  const { mode, toggleTheme } = useThemeControl();
  const { start, stop, busy } = useSessionController();

  const state = useSessionStore((s) => s.state);
  const stateDetail = useSessionStore((s) => s.stateDetail);
  const sessionId = useSessionStore((s) => s.sessionId);

  const userSpeaking = useSessionStore((s) =>
    s.spans.some((span) => span.role === 'user' && span.end === undefined)
  );
  const agentSpeaking = useSessionStore((s) =>
    s.spans.some((span) => span.role === 'agent' && span.end === undefined)
  );

  const workflowParam = params.get('workflow');
  useEffect(() => {
    if (!workflowParam || state !== 'idle') return;
    const store = useSessionStore.getState();
    if (store.config.workflowId !== workflowParam) {
      store.setConfig({ workflowId: workflowParam });
    }
  }, [workflowParam, state]);

  const signal = stateSignal[state];

  return (
    <div className="w-screen h-screen flex flex-col overflow-hidden bg-background">
      {/* Top Header */}
      <header className="h-[48px] px-3.5 flex items-center justify-between border-b border-border bg-card/95 backdrop-blur-md z-20 shrink-0">
        <div className="flex items-center gap-2">
          <Tooltip title="返回工作流编辑器">
            <Button
              asChild
              variant="ghost"
              size="icon-sm"
              className="text-muted-foreground hover:text-foreground"
            >
              <RouterLink to="/" aria-label="返回工作流编辑器">
                <ArrowLeft size={16} />
              </RouterLink>
            </Button>
          </Tooltip>
          <span className="text-sm font-bold tracking-tight text-foreground">
            Voce Conversation
          </span>
        </div>

        <div className="flex items-center gap-2.5">
          <SignalBadge tone={signal.tone} label={signal.label} detail={stateDetail} />

          {sessionId && (
            <span className="text-[11px] font-mono text-muted-foreground px-1.5 py-0.5 rounded bg-sunken/60 border border-border/60">
              sid {sessionId.slice(0, 8)}
            </span>
          )}

          <Tooltip title={mode === 'dark' ? '切换到浅色' : '切换到深色'}>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={toggleTheme}
              className="text-muted-foreground hover:text-foreground"
            >
              {mode === 'dark' ? <Sun size={15} /> : <Moon size={15} />}
            </Button>
          </Tooltip>
        </div>
      </header>

      {/* Main Conversation Body */}
      <div className="flex-1 flex min-h-0 overflow-hidden">
        <SessionConfigPanel busy={busy} />

        <div className="flex-1 flex flex-col min-w-0 min-h-0 bg-background relative">
          <ConversationStream agentSpeaking={agentSpeaking} userSpeaking={userSpeaking} />
          <AudioDock
            onStart={() => void start()}
            onStop={() => void stop()}
            busy={busy}
            state={state}
            agentSpeaking={agentSpeaking}
          />
        </div>
      </div>
    </div>
  );
}
