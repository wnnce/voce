import { useMemo } from 'react';
import { cn } from '@/lib/utils';

interface LevelMeterProps {
  /** 0..1 normalised RMS. */
  value: number;
  active?: boolean;
  bars?: number;
  height?: number;
}

export function LevelMeter({ value, active = false, bars = 14, height = 18 }: LevelMeterProps) {
  const levels = useMemo(() => {
    const clamped = Math.max(0, Math.min(1, value));
    const shaped = Math.pow(clamped, 0.6);
    return Array.from({ length: bars }, (_, i) => {
      const threshold = (i + 1) / bars;
      return Math.min(1, Math.max(0.12, shaped / threshold));
    });
  }, [value, bars]);

  return (
    <div
      role="img"
      aria-label={`输入电平 ${Math.round(Math.max(0, Math.min(1, value)) * 100)}%`}
      className="flex items-end gap-[2px]"
      style={{ height }}
    >
      {levels.map((level, index) => {
        const isLit = level > 0.14;
        return (
          <div
            key={index}
            className={cn(
              'w-[3px] rounded-[1px] transition-[height] duration-75',
              isLit
                ? active
                  ? 'bg-emerald-500'
                  : 'bg-muted-foreground'
                : 'bg-border opacity-60'
            )}
            style={{
              height: `${Math.round(level * 100)}%`,
              minHeight: 2,
            }}
          />
        );
      })}
    </div>
  );
}
