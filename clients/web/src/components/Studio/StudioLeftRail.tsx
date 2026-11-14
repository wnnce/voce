import React, { useState, useMemo } from 'react';
import {
  Puzzle,
  Workflow as WorkflowIcon,
  Search,
  Plus,
  Trash2,
  Copy,
  MoreVertical,
  Mic,
  Bot,
  Volume2,
  AudioLines,
  Radio,
  Server,
  GripVertical,
  Pin,
} from 'lucide-react';
import type { PluginInfo, WorkflowConfig } from '@/types/workflow';
import { useWorkflowStore } from '@/store/useAppStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Tooltip } from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';

export type LeftRailTab = 'nodes' | 'workflows';

interface StudioLeftRailProps {
  open: boolean;
  width?: number;
  activeTab?: LeftRailTab;
  isPinned?: boolean;
  onTogglePin?: () => void;
  onSelectWorkflow: (wf: WorkflowConfig) => void;
  onCreateWorkflow: (wf: Partial<WorkflowConfig>) => void;
  onDuplicateWorkflow: (wf: WorkflowConfig) => void;
  onAddPluginNode: (plugin: PluginInfo) => void;
  currentWorkflowId?: string;
}

const getPluginCategory = (plugin: PluginInfo): string => {
  if (plugin.namespace && plugin.namespace !== 'local') return 'Remote';
  const name = plugin.name.toLowerCase();
  if (name.includes('asr') || name.includes('whisper') || name.includes('sherpa')) return 'ASR';
  if (
    name.includes('llm') ||
    name.includes('gpt') ||
    name.includes('claude') ||
    name.includes('gemini') ||
    name.includes('ollama')
  )
    return 'LLM';
  if (
    name.includes('tts') ||
    name.includes('speech') ||
    name.includes('voice') ||
    name.includes('elevenlabs') ||
    name.includes('cosy')
  )
    return 'TTS';
  if (name.includes('vad')) return 'VAD';
  if (
    name.includes('audio') ||
    name.includes('resample') ||
    name.includes('aec') ||
    name.includes('filter')
  )
    return 'Audio';
  return 'General';
};

const getCategoryIcon = (cat: string) => {
  switch (cat) {
    case 'ASR':
      return <Mic size={13} className="text-emerald-500" />;
    case 'LLM':
      return <Bot size={13} className="text-purple-500" />;
    case 'TTS':
      return <Volume2 size={13} className="text-amber-500" />;
    case 'VAD':
      return <AudioLines size={13} className="text-sky-500" />;
    case 'Audio':
      return <Radio size={13} className="text-blue-500" />;
    case 'Remote':
      return <Server size={13} className="text-rose-500" />;
    default:
      return <Puzzle size={13} className="text-muted-foreground" />;
  }
};

export const StudioLeftRail: React.FC<StudioLeftRailProps> = ({
  open,
  width = 280,
  activeTab = 'workflows',
  isPinned = false,
  onTogglePin,
  onSelectWorkflow,
  onCreateWorkflow,
  onDuplicateWorkflow,
  onAddPluginNode,
  currentWorkflowId,
}) => {
  const { workflows, plugins, deleteWorkflow } = useWorkflowStore();
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string>('All');

  // Filter workflows
  const filteredWorkflows = useMemo(() => {
    if (!searchQuery.trim()) return workflows;
    const q = searchQuery.toLowerCase();
    return workflows.filter((w) => w.name.toLowerCase().includes(q));
  }, [workflows, searchQuery]);

  // Filter plugins
  const filteredPlugins = useMemo(() => {
    return plugins.filter((p) => {
      const cat = getPluginCategory(p);
      if (selectedCategory !== 'All' && cat !== selectedCategory) return false;
      if (!searchQuery.trim()) return true;
      const q = searchQuery.toLowerCase();
      return (
        p.name.toLowerCase().includes(q) ||
        (p.description && p.description.toLowerCase().includes(q)) ||
        (p.namespace && p.namespace.toLowerCase().includes(q))
      );
    });
  }, [plugins, selectedCategory, searchQuery]);

  const categories = useMemo(() => {
    const set = new Set<string>();
    for (const p of plugins) {
      set.add(getPluginCategory(p));
    }
    return ['All', ...Array.from(set)];
  }, [plugins]);

  const handleDeleteWorkflow = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (confirm('Delete this workflow? This cannot be undone.')) {
      await deleteWorkflow(id);
    }
  };

  if (!open) {
    return null;
  }

  return (
    <aside
      style={{ width: `${width}px` }}
      className="shrink-0 border-r border-border bg-card/95 backdrop-blur-md flex flex-col h-full z-20 select-none overflow-hidden"
    >
      {/* VS Code Style Header */}
      <div className="h-[42px] px-3 border-b border-border/80 flex items-center justify-between bg-sunken/20 shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          {activeTab === 'nodes' && <Puzzle size={15} className="text-primary shrink-0" />}
          {activeTab === 'workflows' && <WorkflowIcon size={15} className="text-primary shrink-0" />}
          <span className="text-xs font-bold uppercase tracking-wider text-foreground truncate">
            {activeTab === 'nodes' ? 'Plugins & Nodes' : 'Workflows'}
          </span>
          <span className="text-[10px] text-muted-foreground font-mono bg-sunken px-1.5 py-0.2 rounded border border-border/50">
            {activeTab === 'nodes' ? plugins.length : workflows.length}
          </span>
        </div>

        <div className="flex items-center gap-1">
          {activeTab === 'workflows' && (
            <Button
              size="sm"
              variant="default"
              onClick={() => onCreateWorkflow({})}
              className="h-6 px-2 text-[11px] gap-1 rounded-md"
            >
              <Plus size={12} /> New
            </Button>
          )}

          {onTogglePin && (
            <Tooltip title={isPinned ? 'Unpin (Float over canvas)' : 'Pin (Dock to canvas)'}>
              <button
                type="button"
                onClick={onTogglePin}
                className={cn(
                  'h-6 w-6 rounded flex items-center justify-center transition-colors cursor-pointer outline-none focus:outline-none',
                  isPinned
                    ? 'text-primary bg-primary/10'
                    : 'text-muted-foreground hover:text-foreground hover:bg-sunken'
                )}
                aria-label={isPinned ? 'Unpin Sidebar' : 'Pin Sidebar'}
              >
                <Pin size={12} className={cn(isPinned && 'fill-current')} />
              </button>
            </Tooltip>
          )}
        </div>
      </div>

      {/* Content for Nodes View */}
      {activeTab === 'nodes' && (
        <div className="flex-1 flex flex-col min-h-0 animate-in fade-in-50 duration-150">
          {/* Search Bar */}
          <div className="p-2 border-b border-border/60">
            <div className="relative">
              <Search size={13} className="absolute left-2.5 top-2.5 text-muted-foreground" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search plugins..."
                className="pl-8 h-8 text-xs bg-sunken/40"
              />
            </div>
          </div>

          {/* Category Filter Chips */}
          <div className="p-2 border-b border-border/40 flex items-center gap-1 overflow-x-auto scrollbar-none">
            {categories.map((cat) => (
              <button
                key={cat}
                type="button"
                onClick={() => setSelectedCategory(cat)}
                className={cn(
                  'px-2.5 py-0.5 rounded-full text-[11px] font-medium shrink-0 transition-all duration-150 active:scale-95 cursor-pointer outline-none focus:outline-none focus-visible:ring-1 focus-visible:ring-primary',
                  selectedCategory === cat
                    ? 'bg-primary text-primary-foreground font-semibold shadow-xs'
                    : 'bg-sunken hover:bg-sunken/80 text-muted-foreground hover:text-foreground'
                )}
              >
                {cat}
              </button>
            ))}
          </div>

          <div className="px-3 py-1.5 flex items-center justify-between text-[10px] font-semibold tracking-wider text-muted-foreground uppercase border-b border-border/40">
            <span>DRAG OR CLICK TO ADD</span>
            <span>{filteredPlugins.length} ITEMS</span>
          </div>

          {/* Plugin list */}
          <div className="flex-1 overflow-y-auto p-2 flex flex-col gap-1.5">
            {filteredPlugins.map((plugin) => {
              const cat = getPluginCategory(plugin);
              return (
                <div
                  key={`${plugin.namespace || 'local'}:${plugin.name}`}
                  draggable
                  onDragStart={(e) => {
                    e.dataTransfer.setData('application/voce-plugin', JSON.stringify(plugin));
                    e.dataTransfer.effectAllowed = 'copy';
                  }}
                  className="group relative flex items-start gap-2 p-2 rounded-lg border border-border/60 bg-card hover:border-primary/50 hover:shadow-xs hover:-translate-y-0.5 transition-all duration-150 cursor-grab active:cursor-grabbing select-none"
                >
                  <div className="mt-0.5 text-muted-foreground/40 group-hover:text-muted-foreground">
                    <GripVertical size={13} />
                  </div>

                  <div className="w-6 h-6 rounded-md bg-sunken/60 flex items-center justify-center shrink-0 mt-0.5">
                    {getCategoryIcon(cat)}
                  </div>

                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-1.5">
                      <span className="text-xs font-semibold text-foreground truncate">
                        {plugin.name}
                      </span>
                      {plugin.namespace && plugin.namespace !== 'local' && (
                        <Badge variant="outline" className="text-[9px] px-1 py-0 h-3.5 font-mono">
                          {plugin.namespace}
                        </Badge>
                      )}
                    </div>
                    {plugin.description && (
                      <div className="text-[10px] text-muted-foreground line-clamp-1 mt-0.5">
                        {plugin.description}
                      </div>
                    )}
                    <div className="flex items-center gap-2 mt-1 text-[9px] font-mono text-muted-foreground">
                      <span>in:{plugin.inputs?.length || 0}</span>
                      <span>out:{plugin.outputs?.length || 0}</span>
                      {plugin.ports && plugin.ports.length > 0 && (
                        <span className="text-amber-500 font-bold">ports:{plugin.ports.length}</span>
                      )}
                    </div>
                  </div>

                  {/* Quick Add Button */}
                  <Tooltip title="Add to Canvas Center">
                    <button
                      type="button"
                      onClick={() => onAddPluginNode(plugin)}
                      className="p-1 rounded-md text-muted-foreground hover:text-foreground hover:bg-sunken/80 active:scale-90 transition-all duration-150 shrink-0 cursor-pointer"
                    >
                      <Plus size={13} />
                    </button>
                  </Tooltip>
                </div>
              );
            })}

            {filteredPlugins.length === 0 && (
              <div className="py-8 text-center text-xs text-muted-foreground">
                No plugins match criteria
              </div>
            )}
          </div>
        </div>
      )}

      {/* Content for Workflows View */}
      {activeTab === 'workflows' && (
        <div className="flex-1 flex flex-col min-h-0 animate-in fade-in-50 duration-150">
          {/* Search Bar */}
          <div className="p-2 border-b border-border/60">
            <div className="relative">
              <Search size={13} className="absolute left-2.5 top-2.5 text-muted-foreground" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search workflows..."
                className="pl-8 h-8 text-xs bg-sunken/40"
              />
            </div>
          </div>

          <div className="flex-1 overflow-y-auto p-2 flex flex-col gap-1">
            {filteredWorkflows.map((wf) => {
              const isActive = wf.id === currentWorkflowId;

              return (
                <div
                  key={wf.id}
                  onClick={() => onSelectWorkflow(wf)}
                  className={cn(
                    'group flex items-center gap-2 p-2 rounded-lg cursor-pointer transition-all duration-150 select-none text-xs border hover:-translate-y-0.5 hover:shadow-xs active:scale-[0.99]',
                    isActive
                      ? 'bg-primary/10 border-primary/30 text-foreground font-semibold shadow-xs'
                      : 'border-transparent hover:border-border/60 hover:bg-sunken/50 text-foreground'
                  )}
                >
                  <WorkflowIcon
                    size={14}
                    className={cn('shrink-0', isActive ? 'text-primary' : 'text-muted-foreground')}
                  />

                  <div className="flex-1 min-w-0">
                    <div className="truncate text-xs leading-tight font-medium">{wf.name}</div>
                    <div className="text-[10px] text-muted-foreground font-mono mt-0.5">
                      v{wf.version || '1.0.0'}
                    </div>
                  </div>

                  {/* Workflow Menu */}
                  <div onClick={(e) => e.stopPropagation()}>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <button
                          type="button"
                          className="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-muted text-muted-foreground hover:text-foreground transition-opacity cursor-pointer"
                        >
                          <MoreVertical size={13} />
                        </button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-36 text-xs">
                        <DropdownMenuItem
                          onClick={() => onDuplicateWorkflow(wf)}
                          className="gap-2 cursor-pointer"
                        >
                          <Copy size={13} /> Duplicate
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={(e) => handleDeleteWorkflow(wf.id, e)}
                          className="gap-2 cursor-pointer"
                        >
                          <Trash2 size={13} /> Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>
              );
            })}

            {filteredWorkflows.length === 0 && (
              <div className="py-8 text-center text-xs text-muted-foreground">
                No workflows found
              </div>
            )}
          </div>
        </div>
      )}
    </aside>
  );
};
