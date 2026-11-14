import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

export type SignalTone = 'ok' | 'warn' | 'danger' | 'neutral';

const toneClasses: Record<SignalTone, { dot: string; glow: string }> = {
  ok: {
    dot: 'bg-emerald-500',
    glow: 'ring-4 ring-emerald-500/20',
  },
  warn: {
    dot: 'bg-amber-500',
    glow: 'ring-4 ring-amber-500/20',
  },
  danger: {
    dot: 'bg-rose-500',
    glow: 'ring-4 ring-rose-500/20',
  },
  neutral: {
    dot: 'bg-muted-foreground',
    glow: 'ring-0',
  },
};

interface SignalBadgeProps {
  tone: SignalTone;
  label: string;
  detail?: string;
  compact?: boolean;
}

export function SignalBadge({ tone, label, detail, compact }: SignalBadgeProps) {
  const currentTone = toneClasses[tone];

  return (
    <div className="flex items-center gap-2 min-w-0">
      <span
        aria-hidden
        className={cn(
          'rounded-full shrink-0 transition-all',
          compact ? 'w-1.5 h-1.5' : 'w-2 h-2',
          currentTone.dot,
          currentTone.glow
        )}
      />
      <span className={cn('font-semibold text-foreground whitespace-nowrap', compact ? 'text-[11px]' : 'text-xs')}>
        {label}
      </span>
      {detail && (
        <span className="text-[11px] text-muted-foreground whitespace-nowrap font-mono">
          {detail}
        </span>
      )}
    </div>
  );
}

export function SignalRow({ children }: { children: ReactNode }) {
  return <div className="flex items-center gap-2 flex-wrap">{children}</div>;
}
