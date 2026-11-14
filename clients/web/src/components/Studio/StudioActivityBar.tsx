import React from 'react';
import {
  Puzzle,
  Workflow as WorkflowIcon,
} from 'lucide-react';
import { Tooltip } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

export type ActivityTab = 'workflows' | 'nodes';

interface StudioActivityBarProps {
  activeTab: ActivityTab | null;
  leftRailOpen: boolean;
  onSelectTab: (tab: ActivityTab) => void;
}

export const StudioActivityBar: React.FC<StudioActivityBarProps> = ({
  activeTab,
  leftRailOpen,
  onSelectTab,
}) => {
  const isTabActive = (tab: ActivityTab) => leftRailOpen && activeTab === tab;

  return (
    <nav
      aria-label="Activity Bar"
      className="w-[48px] shrink-0 border-r border-border/60 bg-sunken relative z-40 flex flex-col items-center justify-start py-2.5 select-none gap-1.5"
    >
      {/* Main Navigation Icons */}
      <div className="flex flex-col items-center gap-1.5 w-full">
        {/* Workflows List (First) */}
        <Tooltip title="Workflows & Versions" side="right">
          <button
            type="button"
            onClick={() => onSelectTab('workflows')}
            className={cn(
              'group relative w-10 h-10 rounded-lg flex items-center justify-center transition-all duration-150 cursor-pointer outline-none focus:outline-none focus-visible:ring-1 focus-visible:ring-primary',
              isTabActive('workflows')
                ? 'bg-card text-primary shadow-xs font-semibold'
                : 'text-muted-foreground hover:text-foreground hover:bg-card/60'
            )}
          >
            {isTabActive('workflows') && (
              <span className="absolute left-0 top-2 bottom-2 w-0.5 bg-primary rounded-r" />
            )}
            <WorkflowIcon size={18} className="transition-transform group-hover:scale-110 active:scale-95" />
          </button>
        </Tooltip>

        {/* Nodes / Plugins Library (Second) */}
        <Tooltip title="Plugins & Nodes (Drag & Drop)" side="right">
          <button
            type="button"
            onClick={() => onSelectTab('nodes')}
            className={cn(
              'group relative w-10 h-10 rounded-lg flex items-center justify-center transition-all duration-150 cursor-pointer outline-none focus:outline-none focus-visible:ring-1 focus-visible:ring-primary',
              isTabActive('nodes')
                ? 'bg-card text-primary shadow-xs font-semibold'
                : 'text-muted-foreground hover:text-foreground hover:bg-card/60'
            )}
          >
            {isTabActive('nodes') && (
              <span className="absolute left-0 top-2 bottom-2 w-0.5 bg-primary rounded-r" />
            )}
            <Puzzle size={18} className="transition-transform group-hover:scale-110 active:scale-95" />
          </button>
        </Tooltip>
      </div>
    </nav>
  );
};
