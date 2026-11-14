import React, { useState } from 'react';
import {
  Save,
  Moon,
  Sun,
  LayoutGrid,
  CheckCircle2,
  AlertTriangle,
  SlidersHorizontal,
  Mic,
  Activity,
  PanelRightClose,
  MoreVertical,
  Download,
  Upload,
  Check,
  Edit2,
  Sparkles,
} from 'lucide-react';
import type { WorkflowConfig } from '@/types/workflow';
import type { DAGValidationResult } from '@/utils/dagValidation';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Tooltip } from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useThemeControl } from '@/hooks/useThemeControl';
import { cn } from '@/lib/utils';

export type RightTab = 'inspector' | 'playground' | 'monitor';

interface StudioHeaderProps {
  workflow: WorkflowConfig | null;
  onWorkflowRename: (name: string) => void;
  onSave: () => void;
  isSaving?: boolean;
  isDirty?: boolean;
  onAutoLayout: () => void;
  onValidate: () => void;
  validationResult?: DAGValidationResult;
  rightOpen: boolean;
  rightTab: RightTab;
  onSelectRightTab: (tab: RightTab) => void;
  onToggleRightOpen: () => void;
  onExportJson?: () => void;
  onImportJson?: () => void;
  nodeCount: number;
  edgeCount: number;
}

export const StudioHeader: React.FC<StudioHeaderProps> = ({
  workflow,
  onWorkflowRename,
  onSave,
  isSaving = false,
  isDirty = false,
  onAutoLayout,
  onValidate,
  validationResult,
  rightOpen,
  rightTab,
  onSelectRightTab,
  onToggleRightOpen,
  onExportJson,
  onImportJson,
  nodeCount,
  edgeCount,
}) => {
  const { mode, toggleTheme } = useThemeControl();
  const [isEditingTitle, setIsEditingTitle] = useState(false);
  const [titleInput, setTitleInput] = useState('');

  const handleStartRename = () => {
    setTitleInput(workflow?.name || 'Untitled');
    setIsEditingTitle(true);
  };

  const handleFinishRename = () => {
    setIsEditingTitle(false);
    if (titleInput.trim() && titleInput !== workflow?.name) {
      onWorkflowRename(titleInput.trim());
    }
  };

  const hasErrors = validationResult && validationResult.errors.length > 0;
  const hasWarnings = validationResult && validationResult.warnings.length > 0;

  return (
    <header className="h-[48px] px-3 flex items-center justify-between border-b border-border/60 bg-card/95 backdrop-blur-md relative z-50 shrink-0 select-none">
      {/* Left Section: Brand & Workflow Identity */}
      <div className="flex items-center gap-2.5">
        <div className="flex items-center gap-2">
          {/* Logo icon */}
          <div className="w-6 h-6 rounded-md bg-primary flex items-center justify-center text-primary-foreground shadow-xs">
            <Sparkles size={13} />
          </div>
          <span className="text-xs font-bold tracking-tight text-foreground hidden sm:inline">
            Voce Studio
          </span>
        </div>

        <span className="text-border mx-0.5">/</span>

        {/* Workflow Title & Edit */}
        {workflow ? (
          <div className="flex items-center gap-2">
            {isEditingTitle ? (
              <div className="flex items-center gap-1">
                <input
                  type="text"
                  value={titleInput}
                  onChange={(e) => setTitleInput(e.target.value)}
                  onBlur={handleFinishRename}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleFinishRename();
                    if (e.key === 'Escape') setIsEditingTitle(false);
                  }}
                  autoFocus
                  className="h-6 px-1.5 py-0.5 text-xs font-semibold bg-background border border-primary rounded outline-hidden text-foreground w-40"
                />
                <Button
                  size="icon-sm"
                  variant="ghost"
                  onClick={handleFinishRename}
                  className="h-6 w-6 text-primary"
                >
                  <Check size={13} />
                </Button>
              </div>
            ) : (
              <button
                type="button"
                onClick={handleStartRename}
                className="group flex items-center gap-1.5 px-1.5 py-0.5 rounded hover:bg-sunken text-xs font-semibold text-foreground cursor-pointer transition-colors"
              >
                <span>{workflow.name || 'Untitled Workflow'}</span>
                <Edit2
                  size={11}
                  className="text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity"
                />
              </button>
            )}

            <span className="text-[10px] font-mono text-muted-foreground bg-sunken/60 px-1 py-0.2 rounded border border-border/50">
              v{workflow.version || '1.0.0'}
            </span>

            {!workflow.id ? (
              <Badge variant="warning" className="h-4 px-1 text-[9px] font-black uppercase">
                DRAFT
              </Badge>
            ) : isDirty ? (
              <Badge variant="outline" className="h-4 px-1 text-[9px] text-amber-500 border-amber-500/40">
                UNSAVED
              </Badge>
            ) : (
              <Badge variant="outline" className="h-4 px-1 text-[9px] text-emerald-500 border-emerald-500/40">
                SAVED
              </Badge>
            )}
          </div>
        ) : (
          <span className="text-xs text-muted-foreground italic">No workflow loaded</span>
        )}
      </div>

      {/* Center Section: Graph Stats & Quick Validation */}
      <div className="hidden md:flex items-center gap-3">
        <div className="text-[11px] font-mono text-muted-foreground flex items-center gap-2">
          <span>{nodeCount} nodes</span>
          <span className="w-1 h-1 rounded-full bg-border" />
          <span>{edgeCount} edges</span>
        </div>

        {validationResult && (
          <button
            type="button"
            onClick={onValidate}
            className={cn(
              'flex items-center gap-1 text-[11px] font-medium px-2 py-0.5 rounded-full border transition-all duration-150 hover:scale-[1.02] active:scale-95 cursor-pointer',
              hasErrors
                ? 'bg-destructive/10 text-destructive border-destructive/30'
                : hasWarnings
                  ? 'bg-amber-500/10 text-amber-500 border-amber-500/30'
                  : 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/30'
            )}
          >
            {hasErrors ? (
              <>
                <AlertTriangle size={11} /> {validationResult.errors.length} Errors
              </>
            ) : hasWarnings ? (
              <>
                <AlertTriangle size={11} /> {validationResult.warnings.length} Warnings
              </>
            ) : (
              <>
                <CheckCircle2 size={11} /> Valid DAG
              </>
            )}
          </button>
        )}
      </div>

      {/* Right Section: Actions & Tooling */}
      <div className="flex items-center gap-1.5">
        <Tooltip title="Auto-layout DAG left-to-right" side="bottom">
          <Button
            variant="ghost"
            size="sm"
            onClick={onAutoLayout}
            className="h-7 px-2.5 text-xs text-muted-foreground hover:text-foreground gap-1.5"
          >
            <LayoutGrid size={13} />
            <span className="hidden lg:inline">Auto Layout</span>
          </Button>
        </Tooltip>

        <Tooltip title="Save Workflow (Ctrl+S / Cmd+S)" side="bottom">
          <Button
            variant="outline"
            size="sm"
            onClick={onSave}
            disabled={isSaving || !workflow}
            className="h-7 px-2.5 text-xs gap-1.5"
          >
            <Save size={13} className={cn(isSaving && 'animate-spin')} />
            <span>{isSaving ? 'Saving...' : 'Save'}</span>
          </Button>
        </Tooltip>

        <div className="w-px h-4 bg-border mx-1" />

        {/* Right Panel View Segmented Switcher (Inspector / Voice Playground / Monitor) */}
        <div className="flex items-center p-0.5 rounded-lg border border-border/70 bg-sunken/40 gap-0.5">
          {/* Inspector Button */}
          <Tooltip title={rightOpen && rightTab === 'inspector' ? 'Collapse Inspector' : 'Node & Workflow Settings (Inspector)'} side="bottom">
            <button
              type="button"
              onClick={() => onSelectRightTab('inspector')}
              className={cn(
                'h-6 px-2 rounded-md flex items-center gap-1.5 text-xs font-medium transition-all duration-150 cursor-pointer select-none outline-none',
                rightOpen && rightTab === 'inspector'
                  ? 'bg-card text-foreground font-semibold'
                  : 'text-muted-foreground hover:text-foreground hover:bg-card/40'
              )}
            >
              <SlidersHorizontal size={12} className={cn(rightOpen && rightTab === 'inspector' && 'text-primary')} />
              <span>Inspector</span>
            </button>
          </Tooltip>

          {/* Playground Button (Voice Test) */}
          <Tooltip title={rightOpen && rightTab === 'playground' ? 'Collapse Playground' : 'Voice Conversation Test (Playground)'} side="bottom">
            <button
              type="button"
              onClick={() => onSelectRightTab('playground')}
              className={cn(
                'h-6 px-2 rounded-md flex items-center gap-1.5 text-xs font-medium transition-all duration-150 cursor-pointer select-none outline-none',
                rightOpen && rightTab === 'playground'
                  ? 'bg-card text-emerald-500 font-semibold'
                  : 'text-muted-foreground hover:text-emerald-500 hover:bg-card/40'
              )}
            >
              <Mic size={12} className={cn(rightOpen && rightTab === 'playground' && 'text-emerald-500')} />
              <span>Playground</span>
            </button>
          </Tooltip>

          {/* Monitor Button */}
          <Tooltip title={rightOpen && rightTab === 'monitor' ? 'Collapse Monitor' : 'Runtime & Latency Telemetry (Monitor)'} side="bottom">
            <button
              type="button"
              onClick={() => onSelectRightTab('monitor')}
              className={cn(
                'h-6 px-2 rounded-md flex items-center gap-1.5 text-xs font-medium transition-all duration-150 cursor-pointer select-none outline-none',
                rightOpen && rightTab === 'monitor'
                  ? 'bg-card text-primary font-semibold'
                  : 'text-muted-foreground hover:text-foreground hover:bg-card/40'
              )}
            >
              <Activity size={12} className={cn(rightOpen && rightTab === 'monitor' && 'text-primary')} />
              <span>Monitor</span>
            </button>
          </Tooltip>
        </div>

        {/* Toggle Right Panel Open / Close */}
        <Tooltip title={rightOpen ? 'Collapse Right Panel' : 'Expand Right Panel'} side="bottom">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onToggleRightOpen}
            className={cn(
              'h-7 w-7 text-muted-foreground hover:text-foreground',
              !rightOpen && 'text-foreground bg-sunken'
            )}
            aria-label="Toggle Right Panel"
          >
            <PanelRightClose size={14} className={cn(!rightOpen && 'rotate-180 transition-transform')} />
          </Button>
        </Tooltip>

        <Tooltip title={mode === 'dark' ? 'Light Theme' : 'Dark Theme'} side="bottom">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={toggleTheme}
            className="h-7 w-7 text-muted-foreground hover:text-foreground"
            aria-label="Toggle Theme"
          >
            {mode === 'dark' ? <Sun size={14} /> : <Moon size={14} />}
          </Button>
        </Tooltip>

        {/* More Actions Menu */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="h-7 w-7 text-muted-foreground hover:text-foreground"
              aria-label="More studio actions"
            >
              <MoreVertical size={14} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48 text-xs">
            <DropdownMenuItem onClick={onValidate} className="gap-2 cursor-pointer">
              <CheckCircle2 size={13} /> Validate DAG Graph
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onAutoLayout} className="gap-2 cursor-pointer">
              <LayoutGrid size={13} /> Arrange Nodes (DAG)
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            {onExportJson && (
              <DropdownMenuItem onClick={onExportJson} className="gap-2 cursor-pointer">
                <Download size={13} /> Export Workflow JSON
              </DropdownMenuItem>
            )}
            {onImportJson && (
              <DropdownMenuItem onClick={onImportJson} className="gap-2 cursor-pointer">
                <Upload size={13} /> Import Workflow JSON
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => onSelectRightTab('inspector')} className="gap-2 cursor-pointer">
              <SlidersHorizontal size={13} /> Workflow Settings
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
};
