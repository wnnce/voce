import { useState, useCallback } from 'react';

export interface UsePanelResizeOptions {
  initialWidth: number;
  minWidth: number;
  maxWidth: number;
  storageKey: string;
  direction: 'left' | 'right'; // 'left' = resizes left rail (expands to the right), 'right' = resizes right rail (expands to the left)
}

export function usePanelResize({
  initialWidth,
  minWidth,
  maxWidth,
  storageKey,
  direction,
}: UsePanelResizeOptions) {
  const [width, setWidth] = useState<number>(() => {
    try {
      const saved = localStorage.getItem(storageKey);
      if (saved) {
        const val = parseInt(saved, 10);
        if (!isNaN(val) && val >= minWidth && val <= maxWidth) {
          return val;
        }
      }
    } catch {
      // Fallback
    }
    return initialWidth;
  });

  const [isDragging, setIsDragging] = useState(false);

  const startDrag = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsDragging(true);

      const startX = e.clientX;
      const startWidth = width;
      let currentClamped = startWidth;

      const onMouseMove = (moveEvent: MouseEvent) => {
        const delta = moveEvent.clientX - startX;
        const newWidth = direction === 'left' ? startWidth + delta : startWidth - delta;
        currentClamped = Math.max(minWidth, Math.min(maxWidth, newWidth));
        setWidth(currentClamped);
      };

      const onMouseUp = () => {
        setIsDragging(false);
        document.removeEventListener('mousemove', onMouseMove);
        document.removeEventListener('mouseup', onMouseUp);
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        try {
          localStorage.setItem(storageKey, currentClamped.toString());
        } catch {
          // Ignore storage quota errors
        }
      };

      document.body.style.cursor = 'col-resize';
      document.body.style.userSelect = 'none';
      document.addEventListener('mousemove', onMouseMove);
      document.addEventListener('mouseup', onMouseUp);
    },
    [direction, maxWidth, minWidth, storageKey, width]
  );

  const resetWidth = useCallback(() => {
    setWidth(initialWidth);
    try {
      localStorage.setItem(storageKey, initialWidth.toString());
    } catch {
      // Ignore
    }
  }, [initialWidth, storageKey]);

  return {
    width,
    setWidth,
    isDragging,
    startDrag,
    resetWidth,
  };
}
