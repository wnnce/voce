import React, { useState, useCallback, useEffect, useRef } from 'react';
import ReactFlow, {
  Background,
  Controls,
  useNodesState,
  useEdgesState,
  addEdge,
  BackgroundVariant,
  useReactFlow,
  ReactFlowProvider,
  type Connection,
  type Node,
  type Edge,
} from 'reactflow';
import 'reactflow/dist/style.css';
import CustomNode from '@/pages/Workflow/CustomNode';
import { workflowApi } from '@/api/workflow';
import { EventType, type WorkflowConfig, type Property, type PluginInfo } from '@/types/workflow';
import { validateProperties } from '@/utils/validation';
import { autoLayoutDAG } from '@/utils/layout';
import { validateDAG, type DAGValidationResult } from '@/utils/dagValidation';
import { useMessage } from '@/hooks/useMessage';
import { useWorkflowStore } from '@/store/useAppStore';

const nodeTypesConfig = {
  workflowNode: CustomNode,
};

const defaultFitViewOptions = { padding: 0.25, includeHiddenNodes: true };

export interface WorkflowPageHandle {
  onSelectWorkflow: (wf: WorkflowConfig | null) => void;
  onCreateNew: (wf: Partial<WorkflowConfig>) => void;
  saveWorkflow: () => Promise<WorkflowConfig | null>;
  autoLayout: () => void;
  addPluginAtCenter: (plugin: PluginInfo) => void;
  updateNodeData: (nodeId: string, updates: Record<string, unknown>) => void;
  deleteNode: (nodeId: string) => void;
  setHeadNode: (nodeId: string) => void;
  updateWorkflow: (updates: Partial<WorkflowConfig>) => void;
  exportJson: () => void;
  importJson: (jsonString: string) => void;
}

interface WorkflowCanvasProps {
  onWorkflowChange?: (wf: WorkflowConfig | null) => void;
  onSelectedNodeChange?: (node: Node | null) => void;
  onValidationChange?: (result: DAGValidationResult) => void;
  onCountsChange?: (nodeCount: number, edgeCount: number) => void;
  onDirtyChange?: (isDirty: boolean) => void;
  onNodesUpdate?: (nodes: { id: string; name: string }[]) => void;
}

const WorkflowCanvasInner = React.forwardRef<WorkflowPageHandle, WorkflowCanvasProps>(
  (
    {
      onWorkflowChange,
      onSelectedNodeChange,
      onValidationChange,
      onCountsChange,
      onDirtyChange,
      onNodesUpdate,
    },
    ref
  ) => {
    const allPlugins = useWorkflowStore((state) => state.plugins);
    const fetchWorkflows = useWorkflowStore((state) => state.fetchWorkflows);
    const reactFlowInstance = useReactFlow();

    const [nodes, setNodes, onNodesChange] = useNodesState([]);
    const [edges, setEdges, onEdgesChange] = useEdgesState([]);
    const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
    const [currentWorkflow, setCurrentWorkflow] = useState<WorkflowConfig | null>(null);
    const [isDirty, setIsDirty] = useState(false);

    const { showError, showSuccess } = useMessage();
    const connectionError = useRef<string | null>(null);

    const prevNodeListKey = useRef<string>('');

    // Sync counts and dirty state
    useEffect(() => {
      onCountsChange?.(nodes.length, edges.length);
      const list = nodes.map((n) => ({
        id: n.id,
        name: (n.data?.name as string) || n.id,
      }));
      const listKey = list.map((item) => `${item.id}:${item.name}`).join('|');
      if (prevNodeListKey.current !== listKey) {
        prevNodeListKey.current = listKey;
        onNodesUpdate?.(list);
      }
    }, [nodes, edges.length, onCountsChange, onNodesUpdate]);

    useEffect(() => {
      onDirtyChange?.(isDirty);
    }, [isDirty, onDirtyChange]);

    // Validation
    useEffect(() => {
      const res = validateDAG(currentWorkflow, nodes, edges);
      onValidationChange?.(res);
    }, [currentWorkflow, nodes, edges, onValidationChange]);

    // Report selected node
    useEffect(() => {
      if (!selectedNodeId) {
        onSelectedNodeChange?.(null);
        return;
      }
      const found = nodes.find((n) => n.id === selectedNodeId) || null;
      onSelectedNodeChange?.(found);
    }, [selectedNodeId, nodes, onSelectedNodeChange]);

    // Ensure plugin info is synced when plugins are loaded
    useEffect(() => {
      if (allPlugins.length === 0) return;

      setNodes((nds) => {
        let changed = false;
        const nextNodes = nds.map((n) => {
          if (!n.data.pluginInfo && n.data.plugin) {
            const pluginNamespace = n.data.namespace || 'local';
            const plugin = allPlugins.find(
              (e) => e.name === n.data.plugin && (e.namespace || 'local') === pluginNamespace
            );
            if (plugin) {
              changed = true;
              return {
                ...n,
                data: { ...n.data, pluginInfo: plugin },
              };
            }
          }
          return n;
        });
        return changed ? nextNodes : nds;
      });
    }, [allPlugins, setNodes]);

    const syncHeadStatus = useCallback(
      (headId: string) => {
        setNodes((nds) =>
          nds.map((n) => ({
            ...n,
            data: { ...n.data, isHead: n.id === headId },
          }))
        );
      },
      [setNodes]
    );

    const handleSetAsHead = useCallback(
      (nodeId: string) => {
        setCurrentWorkflow((prev) => {
          if (!prev) return null;
          const next = { ...prev, head: nodeId };
          onWorkflowChange?.(next);
          return next;
        });
        syncHeadStatus(nodeId);
        setIsDirty(true);
      },
      [syncHeadStatus, onWorkflowChange]
    );

    const onConnect = useCallback(
      (params: Connection) => {
        connectionError.current = null;
        const sourceHandleParts = params.sourceHandle?.split('-');
        const type = sourceHandleParts ? parseInt(sourceHandleParts[1], 10) : 0;
        const sourcePort = sourceHandleParts ? parseInt(sourceHandleParts[2], 10) : 0;

        setEdges((eds) =>
          addEdge(
            {
              ...params,
              data: { type, source_port: sourcePort },
            },
            eds
          )
        );
        setIsDirty(true);
      },
      [setEdges]
    );

    const onConnectEnd = useCallback(() => {
      if (connectionError.current) {
        showError(connectionError.current);
        connectionError.current = null;
      }
    }, [showError]);

    const isValidConnection = useCallback(
      (connection: Connection) => {
        const sourceNode = nodes.find((n: Node) => n.id === connection.source);
        const targetNode = nodes.find((n: Node) => n.id === connection.target);

        if (!sourceNode || !targetNode || !sourceNode.data.pluginInfo || !targetNode.data.pluginInfo) {
          return false;
        }

        const sourceHandleParts = connection.sourceHandle?.split('-');
        const targetHandleParts = connection.targetHandle?.split('-');
        if (!sourceHandleParts || !targetHandleParts) return false;

        const sourceType = parseInt(sourceHandleParts[1], 10);
        const targetType = parseInt(targetHandleParts[1], 10);

        if (sourceType !== targetType) {
          connectionError.current = 'Cannot connect handles of different event types';
          return false;
        }

        const typeToPrefix = {
          [EventType.SIGNAL]: 'signal',
          [EventType.PAYLOAD]: 'payload',
          [EventType.AUDIO]: 'audio',
          [EventType.VIDEO]: 'video',
        };

        const prefix = typeToPrefix[sourceType as keyof typeof typeToPrefix];
        if (!prefix) return false;

        const sourceOutputs = (sourceNode.data.pluginInfo.outputs || []).filter(
          (o: Property) => o.prefix === prefix
        );
        const targetInputs = (targetNode.data.pluginInfo.inputs || []).filter(
          (i: Property) => i.prefix === prefix
        );

        const error = validateProperties(sourceOutputs, targetInputs, prefix);
        if (error) {
          connectionError.current = `Data structure mismatch: ${error}`;
          return false;
        }

        connectionError.current = null;
        return true;
      },
      [nodes]
    );

    // Click on node selects it
    const onNodeClick = useCallback((_e: React.MouseEvent, node: Node) => {
      setSelectedNodeId(node.id);
    }, []);

    // Click on canvas pane deselects node
    const onPaneClick = useCallback(() => {
      setSelectedNodeId(null);
    }, []);

    // Create a new node from a plugin
    const createNodeFromPlugin = useCallback(
      (plugin: PluginInfo, position: { x: number; y: number }) => {
        // Generate clean unique name
        const baseName = plugin.name.replace(/[^a-zA-Z0-9_]/g, '_');
        let candidateName = baseName;
        let counter = 1;
        while (nodes.some((n) => n.data?.name?.toLowerCase() === candidateName.toLowerCase())) {
          candidateName = `${baseName}_${counter++}`;
        }

        const shortId = `node_${Date.now().toString(36)}_${Math.floor(Math.random() * 1000).toString(36)}`;
        const isFirst = nodes.length === 0;

        const newNode: Node = {
          id: shortId,
          type: 'workflowNode',
          position,
          width: 260,
          height: 140,
          data: {
            name: candidateName,
            namespace: plugin.namespace || 'local',
            plugin: plugin.name,
            config: {},
            pluginInfo: plugin,
            isHead: isFirst,
            onSetAsHead: () => handleSetAsHead(shortId),
          },
        };

        if (isFirst) {
          setCurrentWorkflow((prev) => {
            const next = prev ? { ...prev, head: shortId } : null;
            onWorkflowChange?.(next);
            return next;
          });
        }

        setNodes((nds) => nds.concat(newNode));
        setSelectedNodeId(shortId);
        setIsDirty(true);
      },
      [nodes, handleSetAsHead, setNodes, onWorkflowChange]
    );

    // Drag & Drop handlers
    const onDragOver = useCallback((event: React.DragEvent) => {
      event.preventDefault();
      event.dataTransfer.dropEffect = 'copy';
    }, []);

    const onDrop = useCallback(
      (event: React.DragEvent) => {
        event.preventDefault();
        const pluginDataStr = event.dataTransfer.getData('application/voce-plugin');
        if (!pluginDataStr) return;

        try {
          const plugin = JSON.parse(pluginDataStr) as PluginInfo;
          const position = reactFlowInstance.screenToFlowPosition({
            x: event.clientX,
            y: event.clientY,
          });
          createNodeFromPlugin(plugin, position);
        } catch (e) {
          console.error('Failed to parse dropped plugin', e);
        }
      },
      [reactFlowInstance, createNodeFromPlugin]
    );

    // Update node data from StudioInspector
    const updateNodeData = useCallback(
      (nodeId: string, updates: Record<string, unknown>) => {
        setNodes((nds) =>
          nds.map((n) => {
            if (n.id === nodeId) {
              return {
                ...n,
                data: {
                  ...n.data,
                  ...updates,
                },
              };
            }
            return n;
          })
        );
        setIsDirty(true);
      },
      [setNodes]
    );

    // Delete node
    const deleteNode = useCallback(
      (nodeId: string) => {
        setNodes((nds) => nds.filter((n) => n.id !== nodeId));
        setEdges((eds) => eds.filter((e) => e.source !== nodeId && e.target !== nodeId));
        if (selectedNodeId === nodeId) {
          setSelectedNodeId(null);
        }
        setIsDirty(true);
      },
      [setNodes, setEdges, selectedNodeId]
    );

    // Auto layout DAG
    const autoLayout = useCallback(() => {
      setNodes((currentNodes) => {
        const layouted = autoLayoutDAG(currentNodes, edges, {
          horizontalSpacing: 340,
          verticalSpacing: 180,
          startX: 100,
          startY: 100,
        });
        return layouted;
      });
      setIsDirty(true);
      setTimeout(() => {
        reactFlowInstance.fitView({ padding: 0.2, duration: 400 });
      }, 50);
    }, [edges, setNodes, reactFlowInstance]);

    // Save Workflow
    const saveWorkflow = useCallback(async (): Promise<WorkflowConfig | null> => {
      if (!currentWorkflow) return null;

      const config: WorkflowConfig = {
        ...currentWorkflow,
        name: currentWorkflow.name || 'Untitled_Workflow',
        version: currentWorkflow.version || '1.0.0',
        nodes: nodes.map((n) => ({
          id: n.id,
          name: n.data.name,
          namespace: n.data.namespace,
          plugin: n.data.plugin,
          config: n.data.config,
          metadata: { position: n.position },
        })),
        edges: edges.map((e) => ({
          source: e.source,
          source_port: e.data?.source_port || 0,
          target: e.target,
          type: e.data?.type || 0,
        })),
      };

      try {
        const res = await workflowApi.save(config);
        const savedWf = res.data;
        setCurrentWorkflow(savedWf);
        onWorkflowChange?.(savedWf);
        setIsDirty(false);
        showSuccess(`Workflow "${savedWf.name}" saved!`);
        syncHeadStatus(savedWf.head);
        void fetchWorkflows();
        return savedWf;
      } catch (err) {
        showError(err instanceof Error ? err.message : 'Failed to save workflow');
        return null;
      }
    }, [currentWorkflow, nodes, edges, onWorkflowChange, showSuccess, showError, syncHeadStatus, fetchWorkflows]);

    // Export workflow JSON
    const exportJson = useCallback(() => {
      if (!currentWorkflow) return;
      const fullConfig: WorkflowConfig = {
        ...currentWorkflow,
        nodes: nodes.map((n) => ({
          id: n.id,
          name: n.data.name,
          namespace: n.data.namespace,
          plugin: n.data.plugin,
          config: n.data.config,
          metadata: { position: n.position },
        })),
        edges: edges.map((e) => ({
          source: e.source,
          source_port: e.data?.source_port || 0,
          target: e.target,
          type: e.data?.type || 0,
        })),
      };

      const dataStr = 'data:text/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(fullConfig, null, 2));
      const downloadAnchor = document.createElement('a');
      downloadAnchor.setAttribute('href', dataStr);
      downloadAnchor.setAttribute('download', `${currentWorkflow.name || 'workflow'}.json`);
      document.body.appendChild(downloadAnchor);
      downloadAnchor.click();
      downloadAnchor.remove();
    }, [currentWorkflow, nodes, edges]);

    // Import workflow JSON
    const importJson = useCallback(
      (jsonString: string) => {
        try {
          const parsed = JSON.parse(jsonString) as WorkflowConfig;
          if (!parsed.name) {
            showError('Invalid workflow JSON format');
            return;
          }
          // Load as draft
          onSelectWorkflow({ ...parsed, id: '' });
          setIsDirty(true);
          showSuccess('Workflow imported as draft!');
        } catch {
          showError('Failed to parse workflow JSON');
        }
      },
      // eslint-disable-next-line react-hooks/exhaustive-deps
      [showError, showSuccess]
    );

    // Select workflow
    const onSelectWorkflow = useCallback(
      (wf: WorkflowConfig | null) => {
        setCurrentWorkflow(wf);
        onWorkflowChange?.(wf);
        setIsDirty(false);
        setSelectedNodeId(null);

        if (!wf) {
          setNodes([]);
          setEdges([]);
          return;
        }

        const nodeData: Node[] =
          wf.nodes?.map((n) => {
            const pluginName = n.plugin;
            const pluginNamespace = n.namespace || 'local';
            const pluginInfo = allPlugins.find(
              (e) => e.name === pluginName && (e.namespace || 'local') === pluginNamespace
            );
            return {
              id: n.id,
              type: 'workflowNode',
              position: (n.metadata?.position as { x: number; y: number } | undefined) || { x: 0, y: 0 },
              width: 260,
              height: 140,
              data: {
                name: n.name,
                namespace: n.namespace,
                plugin: pluginName,
                config: n.config,
                pluginInfo: pluginInfo,
                isHead: n.id === wf.head,
                onSetAsHead: () => handleSetAsHead(n.id),
              },
            };
          }) || [];

        setNodes(nodeData);

        const edgeData: Edge[] =
          wf.edges?.map((e) => ({
            id: `e${e.source}-${e.target}-${e.type}-${e.source_port}`,
            source: e.source,
            target: e.target,
            sourceHandle: `source-${e.type}-${e.source_port}`,
            targetHandle: `target-${e.type}`,
            data: { type: e.type, source_port: e.source_port },
          })) || [];

        setEdges(edgeData);

        setTimeout(() => {
          reactFlowInstance.fitView({ padding: 0.2 });
        }, 50);
      },
      [allPlugins, setNodes, setEdges, handleSetAsHead, onWorkflowChange, reactFlowInstance]
    );

    // Create new workflow
    const onCreateNew = useCallback(
      (wf: Partial<WorkflowConfig>) => {
        const newWf: WorkflowConfig = {
          id: '',
          name: wf.name || 'New_Workflow',
          version: wf.version || '1.0.0',
          head: '',
          nodes: [],
          edges: [],
          scheduler_mode: wf.scheduler_mode || 'thread-per-node',
          scheduler_workers: wf.scheduler_workers || 0,
        };
        setCurrentWorkflow(newWf);
        onWorkflowChange?.(newWf);
        setNodes([]);
        setEdges([]);
        setSelectedNodeId(null);
        setIsDirty(true);
      },
      [setNodes, setEdges, onWorkflowChange]
    );

    const updateWorkflow = useCallback(
      (updates: Partial<WorkflowConfig>) => {
        setCurrentWorkflow((prev) => {
          if (!prev) return null;
          const next = { ...prev, ...updates };
          onWorkflowChange?.(next);
          return next;
        });
        if (updates.head) {
          syncHeadStatus(updates.head);
        }
        setIsDirty(true);
      },
      [onWorkflowChange, syncHeadStatus]
    );

    // Add plugin at canvas viewport center
    const addPluginAtCenter = useCallback(
      (plugin: PluginInfo) => {
        // Calculate center of current viewport
        const center = reactFlowInstance.screenToFlowPosition({
          x: window.innerWidth / 2,
          y: window.innerHeight / 2,
        });
        createNodeFromPlugin(plugin, center);
      },
      [reactFlowInstance, createNodeFromPlugin]
    );

    // Expose handle
    React.useImperativeHandle(ref, () => ({
      onSelectWorkflow,
      onCreateNew,
      saveWorkflow,
      autoLayout,
      addPluginAtCenter,
      updateNodeData,
      deleteNode,
      setHeadNode: handleSetAsHead,
      updateWorkflow,
      exportJson,
      importJson,
    }));

    return (
      <div
        className="w-full h-full relative bg-background select-none"
        onDragOver={onDragOver}
        onDrop={onDrop}
      >
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onConnectEnd={onConnectEnd}
          onNodeClick={onNodeClick}
          onPaneClick={onPaneClick}
          isValidConnection={isValidConnection}
          nodeTypes={nodeTypesConfig}
          deleteKeyCode={['Backspace', 'Delete']}
          fitView
          fitViewOptions={defaultFitViewOptions}
          minZoom={0.2}
          maxZoom={2}
        >
          <Background gap={24} variant={BackgroundVariant.Dots} color="var(--border-subtle)" />
          <Controls position="bottom-right" className="!bg-card !border-border !shadow-sm !rounded-lg" />
        </ReactFlow>
      </div>
    );
  }
);

WorkflowCanvasInner.displayName = 'WorkflowCanvasInner';

const WorkflowPage = React.forwardRef<WorkflowPageHandle, WorkflowCanvasProps>((props, ref) => {
  return (
    <ReactFlowProvider>
      <WorkflowCanvasInner {...props} ref={ref} />
    </ReactFlowProvider>
  );
});

WorkflowPage.displayName = 'WorkflowPage';

export default WorkflowPage;
