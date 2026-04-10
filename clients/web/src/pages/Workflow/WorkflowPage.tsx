import React, { useState, useCallback, useEffect, useRef } from 'react';
import ReactFlow, { 
  Background, 
  Controls, 
  Panel, 
  useNodesState, 
  useEdgesState, 
  addEdge, 
  BackgroundVariant,
  type Connection,
  type Node
} from 'reactflow';
import 'reactflow/dist/style.css';
import { Box, Button, useTheme, Typography, Stack } from '@mui/material';
import { Plus, Save, Play, Workflow } from 'lucide-react';
import CustomNode from '@/pages/Workflow/CustomNode';
import NodeConfigModal from '@/components/Workflow/NodeConfigModal';
import WorkflowModal from '@/components/Workflow/WorkflowModal';
import { workflowApi } from '@/api/workflow';
import { EventType, type WorkflowConfig, type NodeConfig, type Property } from '@/types/workflow';
import { validateProperties } from '@/utils/validation';
import { useMessage } from '@/hooks/useMessage';
import { useWorkflowStore } from '@/store/useAppStore';

const nodeTypes = {
  workflowNode: CustomNode,
};

export interface WorkflowPageHandle {
  onSelectWorkflow: (wf: WorkflowConfig | null) => void;
  onCreateNew: (wf: Partial<WorkflowConfig>) => void;
}

const WorkflowPage = React.forwardRef<WorkflowPageHandle, object>((_, ref) => {
  const allPlugins = useWorkflowStore(state => state.plugins);
  const allWorkflows = useWorkflowStore(state => state.workflows);
  const fetchWorkflows = useWorkflowStore(state => state.fetchWorkflows);
  
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [currentWorkflow, setCurrentWorkflow] = useState<WorkflowConfig | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [wfModalOpen, setWfModalOpen] = useState(false);
  
  const theme = useTheme();
  const { showError, showSuccess } = useMessage();
  const connectionError = useRef<string | null>(null);


  // Ensure plugin info is synced when plugins are loaded
  useEffect(() => {
    if (allPlugins.length === 0) return;
    
    setNodes((nds) => {
      let changed = false;
      const nextNodes = nds.map((n) => {
        if (!n.data.pluginInfo && n.data.plugin) {
          const plugin = allPlugins.find((e) => e.name === n.data.plugin);
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

  const onConnect = useCallback(
    (params: Connection) => {
      connectionError.current = null;
      const sourceHandleParts = params.sourceHandle?.split('-');
      const type = sourceHandleParts ? parseInt(sourceHandleParts[1]) : 0;
      const sourcePort = sourceHandleParts ? parseInt(sourceHandleParts[2]) : 0;

      setEdges((eds) => addEdge({
        ...params,
        data: { type, source_port: sourcePort }
      }, eds));
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

      const sourceType = parseInt(sourceHandleParts[1]);
      const targetType = parseInt(targetHandleParts[1]);

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

      const sourceOutputs = (sourceNode.data.pluginInfo.outputs || []).filter((o: Property) => o.prefix === prefix);
      const targetInputs = (targetNode.data.pluginInfo.inputs || []).filter((i: Property) => i.prefix === prefix);

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

  const syncHeadStatus = useCallback((headId: string) => {
    setNodes((nds) => nds.map((n) => ({
      ...n,
      data: { ...n.data, isHead: n.id === headId }
    })));
  }, [setNodes]);

  const handleSetAsHead = useCallback((nodeId: string) => {
    setCurrentWorkflow(prev => prev ? { ...prev, head: nodeId } : null);
    syncHeadStatus(nodeId);
  }, [syncHeadStatus]);

  const addNode = useCallback(() => {
    setSelectedNode(null);
    setModalOpen(true);
  }, []);

  const onNodeDoubleClick = useCallback((_event: React.MouseEvent, node: Node) => {
    setSelectedNode(node);
    setModalOpen(true);
  }, []);

  const handleSaveNodeConfig = useCallback((config: Partial<NodeConfig>) => {
    // Handle both property names
    const pluginName = config.plugin;
    const plugin = allPlugins.find((e) => e.name === pluginName);

    if (selectedNode) {
      setNodes((nds) =>
        nds.map((node) => {
          if (node.id === selectedNode.id) {
            return {
              ...node,
              data: { ...node.data, ...config, plugin: pluginName, pluginInfo: plugin },
            };
          }
          return node;
        })
      );
    } else {
      const shortId = `node_${Date.now().toString(36)}_${Math.floor(Math.random() * 1000).toString(36)}`;
      const newNode = {
        id: shortId,
        type: 'workflowNode',
        position: { x: 100, y: 100 },
        data: {
          ...config,
          plugin: pluginName, // Ensure standardized key
          pluginInfo: plugin,
          isHead: nodes.length === 0,
          onSetAsHead: () => handleSetAsHead(shortId)
        },
      };

      if (nodes.length === 0) {
        setCurrentWorkflow((prev) => (prev ? { ...prev, head: shortId } : null));
      }
      setNodes((nds) => nds.concat(newNode));
    }
  }, [allPlugins, selectedNode, nodes.length, setNodes, handleSetAsHead]);

  const onSave = async () => {
    if (!currentWorkflow) return;
    setWfModalOpen(true);
  };

  const handleSaveWorkflowMetadata = async (wfMetadata: Partial<WorkflowConfig>) => {
    if (!currentWorkflow) return;
    const updatedWf = {
      ...currentWorkflow,
      name: wfMetadata.name || '',
      version: wfMetadata.version || '1.0.0',
      head: wfMetadata.head || currentWorkflow.head,
    };
    await performSave(updatedWf);
    setWfModalOpen(false);
  };

  const performSave = async (wf: WorkflowConfig) => {
    const config: WorkflowConfig = {
      ...wf,
      nodes: nodes.map((n) => ({
        id: n.id,
        name: n.data.name,
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
      showSuccess('Workflow saved successfully!');
      syncHeadStatus(savedWf.head);
      // Refresh global list via store
      fetchWorkflows();
    } catch {
      // Error handled globally
    }
  };


  const onSelectWorkflow = useCallback((wf: WorkflowConfig | null) => {
    setCurrentWorkflow(wf);
    if (!wf) {
      setNodes([]);
      setEdges([]);
      return;
    }
    
    const nodeData = wf.nodes?.map(n => {
      const pluginName = n.plugin;
      const pluginInfo = allPlugins.find((e) => e.name === pluginName);
      return {
        id: n.id,
        type: 'workflowNode',
        position: (n.metadata?.position as { x: number; y: number } | undefined) || { x: 0, y: 0 },
        data: { 
          name: n.name, 
          plugin: pluginName,
          config: n.config,
          pluginInfo: pluginInfo,
          isHead: n.id === wf.head,
          onSetAsHead: () => handleSetAsHead(n.id)
        }
      };
    }) || [];
    
    setNodes(nodeData);

    const edgeData = wf.edges?.map(e => ({
      id: `e${e.source}-${e.target}-${e.type}-${e.source_port}`,
      source: e.source,
      target: e.target,
      sourceHandle: `source-${e.type}-${e.source_port}`,
      targetHandle: `target-${e.type}`,
      data: { type: e.type, source_port: e.source_port }
    })) || [];
    
    setEdges(edgeData);
  }, [allPlugins, setNodes, setEdges, handleSetAsHead]);

  const onCreateNew = useCallback((wf: Partial<WorkflowConfig>) => {
    setCurrentWorkflow({
      id: '',
      name: wf.name || '',
      version: wf.version || '1.0.0',
      head: '',
      nodes: [],
      edges: []
    });
    setNodes([]);
    setEdges([]);
  }, [setNodes, setEdges]);
  React.useImperativeHandle(ref, () => ({
    onSelectWorkflow,
    onCreateNew,
  }));

  return (
    <Box sx={{ width: '100%', height: '100%', position: 'relative', bgcolor: 'background.default' }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onConnectEnd={onConnectEnd}
        onNodeDoubleClick={onNodeDoubleClick}
        isValidConnection={isValidConnection}
        nodeTypes={nodeTypes}
        deleteKeyCode={['Backspace', 'Delete']}
        fitView
      >
        <Background 
          color={theme.palette.mode === 'light' ? 'rgba(0,0,0,0.15)' : 'rgba(255,255,255,0.1)'} 
          gap={20} 
          variant={BackgroundVariant.Dots} 
        />
        <Controls position="bottom-right" />
        {currentWorkflow && (
          <Panel position="top-right" style={{ margin: 12 }}>
            <Box
              sx={{
                p: 0.85,
                pl: 2.8,
                pr: 0.85,
                display: 'flex',
                alignItems: 'center',
                gap: 2.8,
                bgcolor: theme.palette.mode === 'light' ? 'rgba(255, 255, 255, 0.8)' : 'rgba(35, 35, 35, 0.8)',
                backdropFilter: 'blur(24px) saturate(180%)',
                borderRadius: '100px',
                border: '1px solid var(--border-color)',
              }}
            >
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                <Workflow size={18} color={theme.palette.primary.main} />
                <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    <Typography variant="body2" fontWeight="700" sx={{ color: 'text.primary', lineHeight: 1.2, fontSize: '0.85rem' }}>
                      {currentWorkflow.name || 'Unnamed Workflow'}
                    </Typography>
                    {!currentWorkflow.id && (
                      <Box 
                        sx={{ 
                          px: 0.7, 
                          py: 0.15, 
                          bgcolor: 'warning.light', 
                          color: 'warning.contrastText', 
                          borderRadius: '100px', 
                          fontSize: '0.55rem',
                          fontWeight: '800',
                        }}
                      >
                        DRAFT
                      </Box>
                    )}
                  </Box>
                  <Typography variant="caption" sx={{ color: 'text.secondary', opacity: 0.7, fontSize: '0.65rem', lineHeight: 1, mt: 0.1 }}>
                    v{currentWorkflow.version || '1.0.0'}
                  </Typography>
                </Box>
              </Box>

              <Box sx={{ width: '1px', height: 28, bgcolor: 'var(--border-color)', opacity: 0.5 }} />

              <Stack direction="row" spacing={1}>
                <Button
                  variant="contained"
                  color="primary"
                  size="small"
                  startIcon={<Plus size={16} />}
                  onClick={addNode}
                  sx={{ borderRadius: '100px', textTransform: 'none', fontWeight: 700, height: 32, px: 2.2, boxShadow: 'none' }}
                >
                  Add Node
                </Button>
                <Button
                  variant="outlined"
                  color="primary"
                  size="small"
                  startIcon={<Save size={16} />}
                  onClick={onSave}
                  sx={{ borderRadius: '100px', textTransform: 'none', fontWeight: 700, height: 32, px: 2.2 }}
                >
                  Save
                </Button>
                <Button
                  variant="contained"
                  color="success"
                  size="small"
                  startIcon={<Play size={16} />}
                  disabled
                  sx={{ borderRadius: '100px', textTransform: 'none', fontWeight: 700, height: 32, px: 2.2, boxShadow: 'none' }}
                >
                  Run
                </Button>
              </Stack>
            </Box>
          </Panel>
        )}
      </ReactFlow>

      <NodeConfigModal
        key={selectedNode?.id || 'new'}
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        onSave={handleSaveNodeConfig}
        nodeData={selectedNode?.data}
        plugins={allPlugins}
        existingNames={nodes.map(n => n.data.name)}
      />

      <WorkflowModal
        key={currentWorkflow?.id || 'new-wf'}
        open={wfModalOpen}
        onClose={() => setWfModalOpen(false)}
        onSave={handleSaveWorkflowMetadata}
        initialData={currentWorkflow || undefined}
        nodes={nodes.map(n => ({ id: n.id, name: n.data.name }))}
        existingWorkflows={allWorkflows}
      />
    </Box>
  );
});

export default WorkflowPage;
