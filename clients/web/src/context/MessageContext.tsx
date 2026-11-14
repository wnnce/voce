import React, { useEffect } from 'react';
import { useMessageStore } from '@/store/useAppStore';
import { CheckCircle2, AlertTriangle, AlertCircle, Info, X } from 'lucide-react';

export const MessageProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { message, severity, open, hide } = useMessageStore();

  useEffect(() => {
    if (open) {
      const timer = setTimeout(() => {
        hide();
      }, 3500);
      return () => clearTimeout(timer);
    }
  }, [open, hide, message]);

  const severityIcon = {
    success: <CheckCircle2 className="h-4 w-4 text-emerald-500 shrink-0" />,
    warning: <AlertTriangle className="h-4 w-4 text-amber-500 shrink-0" />,
    error: <AlertCircle className="h-4 w-4 text-rose-500 shrink-0" />,
    info: <Info className="h-4 w-4 text-sky-500 shrink-0" />,
  }[severity] ?? <Info className="h-4 w-4 text-sky-500 shrink-0" />;

  return (
    <>
      {children}
      {open && (
        <div className="fixed top-4 left-1/2 -translate-x-1/2 z-[2000] flex items-center gap-2.5 px-3.5 py-2 rounded-xl border border-border bg-popover/95 backdrop-blur-md text-foreground shadow-xl animate-in fade-in slide-in-from-top-3 duration-200 max-w-md">
          {severityIcon}
          <span className="text-xs font-medium leading-relaxed">{message}</span>
          <button
            onClick={hide}
            className="ml-2 text-muted-foreground hover:text-foreground p-0.5 rounded transition-colors cursor-pointer"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      )}
    </>
  );
};
