import React from 'react';
import { cn } from '@/lib/utils';
import { Tooltip } from '@/components/ui/tooltip';

interface ResizeHandleProps {
  onMouseDown: (e: React.MouseEvent) => void;
  onDoubleClick?: () => void;
  isDragging?: boolean;
  className?: string;
}

export const ResizeHandle: React.FC<ResizeHandleProps> = ({
  onMouseDown,
  onDoubleClick,
  isDragging = false,
  className,
}) => {
  return (
    <Tooltip title="拖拽调整宽度，双击重置默认大小">
      <div
        onMouseDown={onMouseDown}
        onDoubleClick={onDoubleClick}
        className={cn(
          'relative w-1.5 hover:w-1.5 cursor-col-resize group flex items-center justify-center shrink-0 z-20 select-none transition-colors duration-100',
          isDragging ? 'bg-primary/40' : 'bg-transparent hover:bg-primary/20',
          className
        )}
      >
        {/* Subtle pill indicator */}
        <div
          className={cn(
            'w-0.5 h-7 rounded-full transition-all duration-150',
            isDragging
              ? 'bg-primary h-12 shadow-sm'
              : 'bg-border/60 group-hover:bg-primary/80 group-hover:h-9'
          )}
        />
      </div>
    </Tooltip>
  );
};
