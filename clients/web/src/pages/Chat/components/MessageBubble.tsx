import type { ChatMessage } from '@/types/session';
import { cn } from '@/lib/utils';
import { Zap } from 'lucide-react';

const time = (at: number) =>
  new Date(at).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });

interface MessageBubbleProps {
  message: ChatMessage;
}

export function MessageBubble({ message }: MessageBubbleProps) {
  const isUser = message.role === 'user';

  return (
    <div
      className={cn(
        'flex px-5 animate-in fade-in slide-in-from-bottom-1 duration-150',
        isUser ? 'justify-end' : 'justify-start'
      )}
    >
      <div className="max-w-[75%] min-w-0 flex flex-col">
        <div
          className={cn(
            'px-3.5 py-2.5 rounded-2xl text-[13px] leading-relaxed break-words whitespace-pre-wrap shadow-xs',
            isUser
              ? 'bg-primary text-primary-foreground rounded-br-sm'
              : 'bg-chat-bubble border border-border/80 text-foreground rounded-bl-sm shadow-xs'
          )}
        >
          {message.text}
          {!message.final && (
            <span
              aria-hidden
              className={cn(
                'inline-block w-1.5 h-3.5 ml-1 align-middle rounded-xs',
                isUser ? 'bg-white/80' : 'bg-primary animate-pulse'
              )}
            />
          )}
        </div>

        <div
          className={cn(
            'flex items-center gap-1.5 mt-1 px-1 text-[10px] text-muted-foreground',
            isUser ? 'justify-end' : 'justify-start'
          )}
        >
          {message.interrupted && (
            <span className="inline-flex items-center gap-0.5 px-1 py-0.2 rounded bg-amber-500/10 text-amber-500 border border-amber-500/20 font-medium">
              <Zap size={9} /> 打断
            </span>
          )}
          <span>{isUser ? '你' : '助手'}</span>
          <span aria-hidden>·</span>
          <time>{time(message.at)}</time>
        </div>
      </div>
    </div>
  );
}
