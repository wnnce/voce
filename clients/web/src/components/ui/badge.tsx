import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@/lib/utils';

export const badgeVariants = cva(
  'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium transition-colors focus:outline-none focus:ring-1 focus:ring-ring select-none',
  {
    variants: {
      variant: {
        default:
          'bg-primary text-primary-foreground',
        secondary:
          'bg-muted text-muted-foreground',
        outline:
          'border border-border text-foreground',
        destructive:
          'bg-destructive/10 text-destructive border border-destructive/20',
        success:
          'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20',
        warning:
          'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}
