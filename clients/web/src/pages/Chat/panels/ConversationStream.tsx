import { useEffect, useRef } from 'react';
import { MessageBubble } from '@/pages/Chat/components/MessageBubble';
import { useSessionStore } from '@/store/useSessionStore';
import { AlertCircle, Mic } from 'lucide-react';
import { cn } from '@/lib/utils';

interface ConversationStreamProps {
  agentSpeaking: boolean;
  userSpeaking: boolean;
}

export function ConversationStream({ agentSpeaking, userSpeaking }: ConversationStreamProps) {
  const messages = useSessionStore((s) => s.messages);
  const state = useSessionStore((s) => s.state);
  const error = useSessionStore((s) => s.error);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const pinnedRef = useRef(true);

  useEffect(() => {
    const node = scrollRef.current;
    if (!node || !pinnedRef.current) return;
    node.scrollTop = node.scrollHeight;
  }, [messages]);

  const onScroll = () => {
    const node = scrollRef.current;
    if (!node) return;
    const distance = node.scrollHeight - node.scrollTop - node.clientHeight;
    pinnedRef.current = distance < 80;
  };

  const empty = messages.length === 0;
  const speechLabel = userSpeaking ? '正在聆听…' : agentSpeaking ? '助手说话中' : undefined;
  const hasInterim = messages.length > 0 && !messages[messages.length - 1].final;

  return (
    <section aria-label="对话流" className="flex-1 flex flex-col min-w-0 min-h-0 bg-transparent">
      <div
        ref={scrollRef}
        onScroll={onScroll}
        role="log"
        aria-live={hasInterim ? 'off' : 'polite'}
        aria-relevant="additions text"
        className="flex-1 overflow-y-auto py-5 flex flex-col gap-3"
      >
        {empty ? (
          <div className="m-auto text-center max-w-sm px-6 py-10 flex flex-col items-center select-none animate-in fade-in-50 duration-200">
            <div className="w-10 h-10 rounded-full bg-sunken border border-border/80 flex items-center justify-center text-muted-foreground mb-3 shadow-xs">
              <Mic size={18} className="text-emerald-500/80" />
            </div>
            <div className="text-sm font-semibold text-foreground mb-1.5">
              {state === 'idle' ? '还没有开始对话' : '等待第一句语音'}
            </div>
            <div className="text-xs text-muted-foreground leading-relaxed max-w-[260px]">
              {state === 'idle'
                ? '点击上方「Start Voice Call」开启对话。麦克风音频流将实时上行并显示字幕。'
                : '说话即可开始。字幕与语音状态会实时出现在这里。'}
            </div>
          </div>
        ) : (
          messages.map((message) => <MessageBubble key={message.id} message={message} />)
        )}

        {error && messages.length > 0 && (
          <div
            role="alert"
            className="mx-5 p-2.5 rounded-lg border border-destructive/20 bg-destructive/10 text-xs text-destructive flex items-center gap-2"
          >
            <AlertCircle size={14} className="shrink-0" />
            <span>{error}</span>
          </div>
        )}
      </div>

      {/* Speaking status bottom strip */}
      <div className="px-5 py-1.5 min-h-[28px] border-t border-border/60 bg-card flex items-center gap-2 shrink-0">
        <span
          className={cn(
            'w-1.5 h-1.5 rounded-full transition-colors',
            agentSpeaking
              ? 'bg-emerald-500 animate-pulse'
              : userSpeaking
              ? 'bg-primary animate-pulse'
              : 'bg-muted-foreground/40'
          )}
        />
        <span className="text-[11px] text-muted-foreground font-medium">
          {speechLabel ?? '静默'}
        </span>
      </div>
    </section>
  );
}
