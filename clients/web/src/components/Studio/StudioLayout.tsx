import React, { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import type { Node } from 'reactflow';
import WorkflowPage, { type WorkflowPageHandle } from '@/pages/Workflow/WorkflowPage';
import { StudioActivityBar, type ActivityTab } from '@/components/Studio/StudioActivityBar';
import { StudioHeader, type RightTab } from '@/components/Studio/StudioHeader';
import { StudioLeftRail, type LeftRailTab } from '@/components/Studio/StudioLeftRail';
import { StudioInspector } from '@/components/Studio/StudioInspector';
import { StudioPlayground } from '@/components/Studio/StudioPlayground';
import { StudioMonitor } from '@/components/Studio/StudioMonitor';
import { ResizeHandle } from '@/components/Studio/ResizeHandle';
import { usePanelResize } from '@/hooks/usePanelResize';
import { cn } from '@/lib/utils';
import type { WorkflowConfig, PluginInfo } from '@/types/workflow';
import type { DAGValidationResult } from '@/utils/dagValidation';
import { useWorkflowStore } from '@/store/useAppStore';
import { useSessionStore } from '@/store/useSessionStore';

export const StudioLayout: React.FC = () => {
  const {
    workflows,
    setSelectedWorkflow,
    fetchWorkflows,
    fetchPlugins,
    duplicateWorkflow,
  } = useWorkflowStore();

  const workflowCanvasRef = useRef<WorkflowPageHandle>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Activity Bar & Left Primary Sidebar states (VS Code model)
  const [leftTab, setLeftTab] = useState<LeftRailTab>('workflows');
  const [leftRailOpen, setLeftRailOpen] = useState(true);
  const [leftRailPinned, setLeftRailPinned] = useState(false);

  // Right Panel states (Inspector / Playground / Monitor)
  const [rightOpen, setRightOpen] = useState(true);
  const [rightTab, setRightTab] = useState<RightTab>('inspector');

  // Resizable panels
  const leftResize = usePanelResize({
    initialWidth: 280,
    minWidth: 200,
    maxWidth: 480,
    storageKey: 'voce_studio_left_rail_width',
    direction: 'left',
  });

  const rightResize = usePanelResize({
    initialWidth: 360,
    minWidth: 280,
    maxWidth: 640,
    storageKey: 'voce_studio_right_inspector_width',
    direction: 'right',
  });

  // Studio runtime states
  const [currentWorkflow, setCurrentWorkflow] = useState<WorkflowConfig | null>(null);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [nodeList, setNodeList] = useState<{ id: string; name: string }[]>([]);
  const [validationResult, setValidationResult] = useState<DAGValidationResult | undefined>(undefined);
  const [nodeCount, setNodeCount] = useState(0);
  const [edgeCount, setEdgeCount] = useState(0);
  const [isDirty, setIsDirty] = useState(false);
  const [isSaving, setIsSaving] = useState(false);

  const effectiveNodeList = useMemo(() => {
    if (nodeList.length > 0) return nodeList;
    if (currentWorkflow?.nodes) {
      return currentWorkflow.nodes.map((n) => ({ id: n.id, name: n.name || n.id }));
    }
    return [];
  }, [nodeList, currentWorkflow]);

  // Handle workflow selection
  const handleSelectWorkflow = useCallback(
    (wf: WorkflowConfig) => {
      setSelectedWorkflow(wf);
      setCurrentWorkflow(wf);
      if (wf.id) {
        useSessionStore.getState().setConfig({ workflowId: wf.id });
      }
      workflowCanvasRef.current?.onSelectWorkflow(wf);
    },
    [setSelectedWorkflow]
  );

  // Initial load
  useEffect(() => {
    void fetchPlugins();
    void fetchWorkflows().then(() => {
      const all = useWorkflowStore.getState().workflows;
      if (all.length > 0) {
        handleSelectWorkflow(all[0]);
      }
    });
  }, [fetchWorkflows, fetchPlugins, handleSelectWorkflow]);

  // Handle create new workflow
  const handleCreateWorkflow = useCallback(
    (wf: Partial<WorkflowConfig>) => {
      const draftWf: WorkflowConfig = {
        id: '',
        name: wf.name || `VoiceAgent_${workflows.length + 1}`,
        version: wf.version || '1.0.0',
        head: '',
        nodes: [],
        edges: [],
        scheduler_mode: 'thread-per-node',
        scheduler_workers: 0,
      };
      setSelectedWorkflow(draftWf);
      setCurrentWorkflow(draftWf);
      workflowCanvasRef.current?.onCreateNew(draftWf);
    },
    [workflows.length, setSelectedWorkflow]
  );

  // Handle duplicate workflow
  const handleDuplicateWorkflow = useCallback(
    async (wf: WorkflowConfig) => {
      await duplicateWorkflow(wf);
      const cloned = useWorkflowStore.getState().selectedWorkflow;
      if (cloned) {
        setCurrentWorkflow(cloned);
        workflowCanvasRef.current?.onSelectWorkflow(cloned);
      }
    },
    [duplicateWorkflow]
  );

  // Save workflow
  const handleSave = useCallback(async () => {
    if (!workflowCanvasRef.current) return;
    setIsSaving(true);
    try {
      const saved = await workflowCanvasRef.current.saveWorkflow();
      if (saved) {
        setCurrentWorkflow(saved);
        setSelectedWorkflow(saved);
      }
    } finally {
      setIsSaving(false);
    }
  }, [setSelectedWorkflow]);

  // Keyboard shortcut: Ctrl+S / Cmd+S to save
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        void handleSave();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleSave]);

  // Auto layout
  const handleAutoLayout = useCallback(() => {
    workflowCanvasRef.current?.autoLayout();
  }, []);

  // Validate graph
  const handleValidate = useCallback(() => {
    setRightOpen(true);
    setRightTab('inspector');
    setSelectedNode(null);
  }, []);

  // Workflow rename from header
  const handleWorkflowRename = useCallback((newName: string) => {
    setCurrentWorkflow((prev) => (prev ? { ...prev, name: newName } : null));
    workflowCanvasRef.current?.updateWorkflow({ name: newName });
  }, []);

  // Add plugin node from left rail
  const handleAddPluginNode = useCallback((plugin: PluginInfo) => {
    workflowCanvasRef.current?.addPluginAtCenter(plugin);
  }, []);

  // Node updates from inspector
  const handleUpdateNodeData = useCallback((nodeId: string, updates: Record<string, unknown>) => {
    workflowCanvasRef.current?.updateNodeData(nodeId, updates);
  }, []);

  // Delete node from inspector
  const handleDeleteNode = useCallback((nodeId: string) => {
    workflowCanvasRef.current?.deleteNode(nodeId);
    setSelectedNode(null);
  }, []);

  // Set head node from inspector
  const handleSetHeadNode = useCallback((nodeId: string) => {
    workflowCanvasRef.current?.setHeadNode(nodeId);
  }, []);

  // Update workflow from inspector
  const handleUpdateWorkflow = useCallback((updates: Partial<WorkflowConfig>) => {
    setCurrentWorkflow((prev) => (prev ? { ...prev, ...updates } : null));
    workflowCanvasRef.current?.updateWorkflow(updates);
  }, []);

  // Import / Export JSON
  const handleExportJson = useCallback(() => {
    workflowCanvasRef.current?.exportJson();
  }, []);

  const handleImportJsonClick = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (event) => {
      const content = event.target?.result as string;
      if (content) {
        workflowCanvasRef.current?.importJson(content);
      }
    };
    reader.readAsText(file);
    e.target.value = '';
  };

  // Activity Bar Tab Click (VS Code toggle logic for Left side)
  const handleActivityBarTabClick = useCallback(
    (tab: ActivityTab) => {
      const targetLeftTab = tab as LeftRailTab;
      if (leftRailOpen && leftTab === targetLeftTab) {
        setLeftRailOpen(false);
      } else {
        setLeftTab(targetLeftTab);
        setLeftRailOpen(true);
      }
    },
    [leftRailOpen, leftTab]
  );

  // Top Header Segmented Switcher for Right side (Inspector / Playground / Monitor)
  const handleSelectRightTab = useCallback(
    (tab: RightTab) => {
      if (rightOpen && rightTab === tab) {
        setRightOpen(false);
      } else {
        setRightTab(tab);
        setRightOpen(true);
      }
    },
    [rightOpen, rightTab]
  );

  // When node is selected on the canvas, switch right panel to inspector
  const handleSelectedNodeChange = useCallback(
    (node: Node | null) => {
      setSelectedNode(node);
      if (node) {
        setRightTab('inspector');
        setRightOpen(true);
      }
    },
    []
  );

  return (
    <div className="w-screen h-screen relative bg-background overflow-hidden flex flex-col select-none">
      {/* Hidden file input for JSON import */}
      <input
        type="file"
        ref={fileInputRef}
        onChange={handleFileChange}
        accept=".json"
        className="hidden"
      />

      {/* Studio Header */}
      <StudioHeader
        workflow={currentWorkflow}
        onWorkflowRename={handleWorkflowRename}
        onSave={handleSave}
        isSaving={isSaving}
        isDirty={isDirty}
        onAutoLayout={handleAutoLayout}
        onValidate={handleValidate}
        validationResult={validationResult}
        rightOpen={rightOpen}
        rightTab={rightTab}
        onSelectRightTab={handleSelectRightTab}
        onToggleRightOpen={() => setRightOpen((prev) => !prev)}
        onExportJson={handleExportJson}
        onImportJson={handleImportJsonClick}
        nodeCount={nodeCount}
        edgeCount={edgeCount}
      />

      {/* Middle Studio Body */}
      <div className="flex-1 flex min-h-0 relative overflow-hidden">
        {/* 1. Far Left: VS Code Style Activity Bar */}
        <StudioActivityBar
          activeTab={leftRailOpen ? leftTab : null}
          leftRailOpen={leftRailOpen}
          onSelectTab={handleActivityBarTabClick}
        />

        {/* 2. Left Primary Sidebar (Nodes / Workflows) */}
        {leftRailPinned ? (
          /* Pinned Mode: Docks as a flex column and pushes canvas */
          <>
            <div
              style={{ width: leftRailOpen ? `${leftResize.width}px` : 0 }}
              className={cn(
                'flex h-full shrink-0 overflow-hidden transition-[width] duration-200 ease-in-out',
                leftResize.isDragging && 'transition-none'
              )}
            >
              <StudioLeftRail
                open={true}
                activeTab={leftTab}
                width={leftResize.width}
                isPinned={true}
                onTogglePin={() => setLeftRailPinned(false)}
                onSelectWorkflow={handleSelectWorkflow}
                onCreateWorkflow={handleCreateWorkflow}
                onDuplicateWorkflow={handleDuplicateWorkflow}
                onAddPluginNode={handleAddPluginNode}
                currentWorkflowId={currentWorkflow?.id}
              />
            </div>
            {leftRailOpen && (
              <ResizeHandle
                onMouseDown={leftResize.startDrag}
                onDoubleClick={leftResize.resetWidth}
                isDragging={leftResize.isDragging}
              />
            )}
          </>
        ) : (
          /* Overlay / Float Mode: Slides out over canvas, ZERO shift to DAG workspace! */
          <div
            style={{
              width: `${leftResize.width}px`,
              transform: leftRailOpen ? 'translateX(0)' : 'translateX(-100%)',
            }}
            className={cn(
              'absolute left-[48px] top-0 bottom-0 z-20 flex border-r border-border/60 transition-all duration-200 ease-in-out',
              leftResize.isDragging && 'transition-none',
              !leftRailOpen && 'pointer-events-none opacity-0'
            )}
          >
            <StudioLeftRail
              open={true}
              activeTab={leftTab}
              width={leftResize.width}
              isPinned={false}
              onTogglePin={() => setLeftRailPinned(true)}
              onSelectWorkflow={handleSelectWorkflow}
              onCreateWorkflow={handleCreateWorkflow}
              onDuplicateWorkflow={handleDuplicateWorkflow}
              onAddPluginNode={handleAddPluginNode}
              currentWorkflowId={currentWorkflow?.id}
            />
            <ResizeHandle
              onMouseDown={leftResize.startDrag}
              onDoubleClick={leftResize.resetWidth}
              isDragging={leftResize.isDragging}
            />
          </div>
        )}

        {/* 3. Center Canvas (ReactFlow DAG) */}
        <main className="flex-1 h-full relative overflow-hidden bg-background">
          <WorkflowPage
            ref={workflowCanvasRef}
            onWorkflowChange={setCurrentWorkflow}
            onSelectedNodeChange={handleSelectedNodeChange}
            onValidationChange={setValidationResult}
            onCountsChange={(nc, ec) => {
              setNodeCount(nc);
              setEdgeCount(ec);
            }}
            onDirtyChange={setIsDirty}
            onNodesUpdate={setNodeList}
          />
          {/* Overlay during panel drag to prevent ReactFlow event capture */}
          {(leftResize.isDragging || rightResize.isDragging) && (
            <div className="absolute inset-0 z-50 cursor-col-resize select-none" />
          )}
        </main>

        {/* 4. Right Panel Resizer */}
        {rightOpen && (
          <ResizeHandle
            onMouseDown={rightResize.startDrag}
            onDoubleClick={rightResize.resetWidth}
            isDragging={rightResize.isDragging}
          />
        )}

        {/* 5. Right Docked Panel Container (Inspector / Playground / Monitor) */}
        <div
          style={{ width: rightOpen ? `${rightResize.width}px` : 0 }}
          className={cn(
            'flex flex-col h-full shrink-0 overflow-hidden transition-[width] duration-200 ease-in-out bg-card border-l border-border/80 shadow-xs',
            rightResize.isDragging && 'transition-none'
          )}
        >
          {rightTab === 'inspector' && (
            <StudioInspector
              open={true}
              width={rightResize.width}
              onClose={() => setRightOpen(false)}
              selectedNode={selectedNode}
              workflow={currentWorkflow}
              nodeList={effectiveNodeList}
              onUpdateNodeData={handleUpdateNodeData}
              onDeleteNode={handleDeleteNode}
              onSetHeadNode={handleSetHeadNode}
              onUpdateWorkflow={handleUpdateWorkflow}
              validationResult={validationResult}
            />
          )}

          {rightTab === 'playground' && (
            <StudioPlayground
              open={true}
              width={rightResize.width}
              onClose={() => setRightOpen(false)}
              workflow={currentWorkflow}
            />
          )}

          {rightTab === 'monitor' && (
            <StudioMonitor
              open={true}
              width={rightResize.width}
              onClose={() => setRightOpen(false)}
              workflow={currentWorkflow}
            />
          )}
        </div>
      </div>
    </div>
  );
};

export default StudioLayout;
