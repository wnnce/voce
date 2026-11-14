import React, { useState, useMemo } from 'react';
import type { Node } from 'reactflow';
import Form from '@rjsf/core';
import type { RJSFSchema } from '@rjsf/utils';
import validator from '@rjsf/validator-ajv8';
import CodeMirror from '@uiw/react-codemirror';
import { json } from '@codemirror/lang-json';
import {
  X,
  Trash2,
  Crown,
  Terminal,
  Database,
  Mic,
  Video,
  Hash,
  AlertCircle,
  Settings2,
  Workflow as WorkflowIcon,
  ArrowRightLeft,
  CheckCircle2,
  AlertTriangle,
  Plus,
} from 'lucide-react';
import type { WorkflowConfig, PluginInfo, Property, PortMetadata, Field } from '@/types/workflow';
import type { DAGValidationResult } from '@/utils/dagValidation';
import { useWorkflowStore } from '@/store/useAppStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Tooltip } from '@/components/ui/tooltip';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { compactWidgets, compactTemplates } from '@/components/Form/RjsfTheme';
import { cn } from '@/lib/utils';

interface StudioInspectorProps {
  open: boolean;
  width?: number;
  onClose: () => void;
  selectedNode: Node | null;
  workflow: WorkflowConfig | null;
  nodeList: { id: string; name: string }[];
  onUpdateNodeData: (nodeId: string, updates: Record<string, unknown>) => void;
  onDeleteNode: (nodeId: string) => void;
  onSetHeadNode: (nodeId: string) => void;
  onUpdateWorkflow: (updates: Partial<WorkflowConfig>) => void;
  validationResult?: DAGValidationResult;
}

export const StudioInspector: React.FC<StudioInspectorProps> = ({
  open,
  width = 340,
  onClose,
  selectedNode,
  workflow,
  nodeList,
  onUpdateNodeData,
  onDeleteNode,
  onSetHeadNode,
  onUpdateWorkflow,
  validationResult,
}) => {
  if (!open) return null;

  return (
    <aside
      style={{ width: `${width}px` }}
      className="shrink-0 border-l border-border bg-card/95 backdrop-blur-md flex flex-col h-full z-20 select-none overflow-hidden"
    >
      {selectedNode ? (
        <NodeInspectorView
          key={selectedNode.id}
          node={selectedNode}
          workflow={workflow}
          nodeList={nodeList}
          onClose={onClose}
          onUpdateNodeData={onUpdateNodeData}
          onDeleteNode={onDeleteNode}
          onSetHeadNode={onSetHeadNode}
        />
      ) : (
        <WorkflowInspectorView
          key={workflow?.id || 'wf-settings'}
          workflow={workflow}
          nodeList={nodeList}
          onClose={onClose}
          onUpdateWorkflow={onUpdateWorkflow}
          validationResult={validationResult}
        />
      )}
    </aside>
  );
};

// -------------------------------------------------------------
// NODE INSPECTOR SUB-COMPONENT
// -------------------------------------------------------------
// Normalize JSON Schema from Go plugins ($defs and draft-2020-12 stripping)
function normalizeSchema(schema?: Record<string, unknown>): RJSFSchema | null {
  if (!schema || typeof schema !== 'object') return null;

  let target: Record<string, unknown> = { ...schema };

  // Resolve $ref if pointing to #/$defs/ or #/definitions/
  if (typeof target['$ref'] === 'string') {
    const ref = target['$ref'];
    if (ref.startsWith('#/$defs/')) {
      const defKey = ref.replace('#/$defs/', '');
      const defs = target['$defs'] as Record<string, unknown> | undefined;
      if (defs && defs[defKey] && typeof defs[defKey] === 'object') {
        target = { ...(defs[defKey] as Record<string, unknown>), $defs: defs };
      }
    } else if (ref.startsWith('#/definitions/')) {
      const defKey = ref.replace('#/definitions/', '');
      const defs = target['definitions'] as Record<string, unknown> | undefined;
      if (defs && defs[defKey] && typeof defs[defKey] === 'object') {
        target = { ...(defs[defKey] as Record<string, unknown>), definitions: defs };
      }
    }
  }

  // Strip $schema and $id to avoid Ajv Draft-2020-12 unknown schema errors
  const { $schema, $id, ...cleanSchema } = target;
  void $schema;
  void $id;

  if (!cleanSchema.type) {
    cleanSchema.type = 'object';
  }

  return cleanSchema as RJSFSchema;
}

// Fallback Key-Value visual editor for plugins with no JSON schema
const KeyValueConfigEditor: React.FC<{
  config: Record<string, unknown>;
  onChange: (newConfig: Record<string, unknown>) => void;
  onSwitchToJson: () => void;
}> = ({ config, onChange, onSwitchToJson }) => {
  const entries = Object.entries(config || {});

  const handleUpdateKey = (oldKey: string, newKey: string, val: unknown) => {
    if (!newKey.trim() || newKey === oldKey) return;
    const next = { ...config };
    delete next[oldKey];
    next[newKey] = val;
    onChange(next);
  };

  const handleUpdateValue = (key: string, valStr: string) => {
    let parsed: unknown = valStr;
    if (valStr === 'true') parsed = true;
    else if (valStr === 'false') parsed = false;
    else if (!isNaN(Number(valStr)) && valStr.trim() !== '') parsed = Number(valStr);
    onChange({ ...config, [key]: parsed });
  };

  const handleDelete = (key: string) => {
    const next = { ...config };
    delete next[key];
    onChange(next);
  };

  const handleAdd = () => {
    let newKey = 'param_1';
    let count = 1;
    while (newKey in config) {
      newKey = `param_${++count}`;
    }
    onChange({ ...config, [newKey]: '' });
  };

  return (
    <div className="flex flex-col gap-2.5">
      <div className="p-2.5 rounded-lg bg-sunken/40 border border-border/60 text-xs text-muted-foreground flex flex-col gap-1.5">
        <div className="flex items-center justify-between">
          <span className="font-semibold text-foreground">动态表单参数配置</span>
          <Button
            variant="ghost"
            size="sm"
            onClick={onSwitchToJson}
            className="h-5 px-1.5 text-[11px] text-primary hover:text-primary"
          >
            切换到 JSON
          </Button>
        </div>
        <p className="text-[11px] leading-relaxed">
          当前插件未在后端预设结构化 Schema，可通过下方参数列表直接可视化配置。
        </p>
      </div>

      <div className="flex flex-col gap-1.5">
        {entries.map(([key, val]) => (
          <div
            key={key}
            className="flex items-center gap-1.5 p-1 rounded-md bg-sunken/20 border border-border/50"
          >
            <Input
              defaultValue={key}
              onBlur={(e) => handleUpdateKey(key, e.target.value, val)}
              placeholder="key"
              className="h-7 text-xs font-mono w-1/3"
            />
            <span className="text-muted-foreground text-xs">:</span>
            <Input
              value={typeof val === 'object' ? JSON.stringify(val) : String(val ?? '')}
              onChange={(e) => handleUpdateValue(key, e.target.value)}
              placeholder="value"
              className="h-7 text-xs font-mono flex-1"
            />
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => handleDelete(key)}
              className="h-7 w-7 text-muted-foreground hover:text-destructive"
            >
              <Trash2 size={12} />
            </Button>
          </div>
        ))}

        {entries.length === 0 && (
          <div className="py-4 text-center text-xs text-muted-foreground italic border border-dashed border-border/60 rounded-lg">
            暂无配置参数，点击下方按钮添加
          </div>
        )}

        <Button
          variant="outline"
          size="sm"
          onClick={handleAdd}
          className="h-7 text-xs gap-1 border-dashed mt-1"
        >
          <Plus size={12} /> 添加参数项
        </Button>
      </div>
    </div>
  );
};

interface NodeInspectorViewProps {
  node: Node;
  workflow: WorkflowConfig | null;
  nodeList: { id: string; name: string }[];
  onClose: () => void;
  onUpdateNodeData: (nodeId: string, updates: Record<string, unknown>) => void;
  onDeleteNode: (nodeId: string) => void;
  onSetHeadNode: (nodeId: string) => void;
}

const NodeInspectorView: React.FC<NodeInspectorViewProps> = ({
  node,
  workflow,
  nodeList,
  onClose,
  onUpdateNodeData,
  onDeleteNode,
  onSetHeadNode,
}) => {
  const allPlugins = useWorkflowStore((s) => s.plugins);
  const pluginInfo = useMemo(() => {
    const pName = node.data?.plugin;
    const pNs = node.data?.namespace || 'local';
    return (
      allPlugins.find((p) => p.name === pName && (p.namespace || 'local') === pNs) ||
      (node.data?.pluginInfo as PluginInfo | undefined)
    );
  }, [allPlugins, node.data?.plugin, node.data?.namespace, node.data?.pluginInfo]);

  const normalizedSchema = useMemo(() => {
    return normalizeSchema(pluginInfo?.schema);
  }, [pluginInfo?.schema]);

  const isHead = workflow?.head === node.id;

  const [activeTab, setActiveTab] = useState<'config' | 'ports'>('config');
  const [configMode, setConfigMode] = useState<'form' | 'json'>('form');
  const [nodeName, setNodeName] = useState(node.data?.name || '');
  const [nameError, setNameError] = useState('');
  const [jsonText, setJsonText] = useState(() =>
    JSON.stringify(node.data?.config || {}, null, 2)
  );
  const [jsonError, setJsonError] = useState('');

  const handleNameChange = (val: string) => {
    const sanitized = val.replace(/[^a-zA-Z0-9_]/g, '');
    setNodeName(sanitized);

    if (!sanitized.trim()) {
      setNameError('Name cannot be empty');
      return;
    }
    const exists = nodeList.some(
      (n) => n.id !== node.id && n.name.toLowerCase() === sanitized.toLowerCase()
    );
    if (exists) {
      setNameError('Name must be unique in workflow');
      return;
    }

    setNameError('');
    onUpdateNodeData(node.id, { name: sanitized });
  };

  const handleFormChange = (newFormData: Record<string, unknown>) => {
    onUpdateNodeData(node.id, { config: newFormData });
    setJsonText(JSON.stringify(newFormData, null, 2));
  };

  const handleJsonChange = (val: string) => {
    setJsonText(val);
    try {
      const parsed = JSON.parse(val) as unknown;
      if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
        setJsonError('Config must be a JSON object');
        return;
      }
      setJsonError('');
      onUpdateNodeData(node.id, { config: parsed as Record<string, unknown> });
    } catch (err) {
      setJsonError(err instanceof Error ? err.message : 'Invalid JSON');
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="px-3.5 py-2.5 border-b border-border/80 flex items-center justify-between bg-sunken/20">
        <div className="flex items-center gap-2 min-w-0">
          <div className="w-6 h-6 rounded-md bg-primary/10 text-primary flex items-center justify-center shrink-0">
            <Settings2 size={14} />
          </div>
          <div className="truncate font-semibold text-xs text-foreground">Node Inspector</div>
        </div>

        <div className="flex items-center gap-1">
          <Tooltip title="Delete Node">
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => onDeleteNode(node.id)}
              className="h-6 w-6 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
            >
              <Trash2 size={13} />
            </Button>
          </Tooltip>
          <Tooltip title="Close Inspector">
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onClose}
              className="h-6 w-6 text-muted-foreground hover:text-foreground"
            >
              <X size={14} />
            </Button>
          </Tooltip>
        </div>
      </div>

      {/* Node Info & Name */}
      <div className="p-3 border-b border-border/60 flex flex-col gap-2.5 bg-card">
        <div>
          <label className="text-[11px] font-medium text-muted-foreground mb-1 block">
            Node Instance Name
          </label>
          <Input
            value={nodeName}
            onChange={(e) => handleNameChange(e.target.value)}
            placeholder="e.g. MyAudioFilter"
            className={cn('h-7 text-xs font-mono', nameError && 'border-destructive')}
          />
          {nameError && <div className="text-[10px] text-destructive mt-0.5">{nameError}</div>}
        </div>

        {/* Plugin Metadata */}
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground text-[11px]">Plugin</span>
          <div className="flex items-center gap-1.5 font-mono text-[11px]">
            <span className="font-semibold text-foreground">{pluginInfo?.name || node.data?.plugin}</span>
            {pluginInfo?.namespace && (
              <Badge variant="outline" className="text-[9px] px-1 py-0 h-4">
                {pluginInfo.namespace}
              </Badge>
            )}
          </div>
        </div>

        {/* Head Node Switch */}
        <div className="flex items-center justify-between pt-1 border-t border-border/40">
          <div className="flex items-center gap-1.5">
            <Crown size={13} className={isHead ? 'text-amber-500' : 'text-muted-foreground'} />
            <span className="text-xs font-medium text-foreground">Entry Head Node</span>
          </div>
          <Switch checked={isHead} onCheckedChange={() => onSetHeadNode(node.id)} />
        </div>
      </div>

      {/* Tabs */}
      <div className="p-2 border-b border-border/60 bg-sunken/20">
        <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as 'config' | 'ports')}>
          <TabsList className="w-full grid grid-cols-2 h-7 p-0.5">
            <TabsTrigger value="config" className="text-xs font-semibold">
              Configuration
            </TabsTrigger>
            <TabsTrigger value="ports" className="text-xs font-semibold">
              Ports ({((pluginInfo?.inputs?.length || 0) + (pluginInfo?.outputs?.length || 0))})
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {/* Tab Content */}
      <div className="flex-1 overflow-y-auto p-3 min-h-0">
        {activeTab === 'config' ? (
          <div key="config" className="flex flex-col gap-3 animate-in fade-in-50 duration-150">
            {/* Form vs JSON Toggle */}
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-bold tracking-wider text-muted-foreground uppercase">
                NODE SETTINGS
              </span>
              <Tabs
                value={configMode}
                onValueChange={(v) => setConfigMode(v as 'form' | 'json')}
              >
                <TabsList className="h-6 p-0.5">
                  <TabsTrigger value="form" className="h-5 px-2 text-[10px]">
                    Form
                  </TabsTrigger>
                  <TabsTrigger value="json" className="h-5 px-2 text-[10px]">
                    JSON
                  </TabsTrigger>
                </TabsList>
              </Tabs>
            </div>

            {configMode === 'form' ? (
              normalizedSchema ? (
                <div key="form-schema" className="rounded-lg border border-border/80 bg-sunken/20 p-2 text-xs animate-in fade-in-50 duration-150">
                  <Form
                    schema={normalizedSchema}
                    validator={validator}
                    formData={node.data?.config || {}}
                    onChange={(e) => handleFormChange(e.formData as Record<string, unknown>)}
                    widgets={compactWidgets}
                    templates={compactTemplates}
                    children={<></>}
                  />
                </div>
              ) : (
                <div key="form-custom" className="animate-in fade-in-50 duration-150">
                  <KeyValueConfigEditor
                    config={(node.data?.config as Record<string, unknown>) || {}}
                    onChange={handleFormChange}
                    onSwitchToJson={() => setConfigMode('json')}
                  />
                </div>
              )
            ) : (
              <div key="json-mode" className="flex flex-col gap-2 animate-in fade-in-50 duration-150">
                <div
                  className={cn(
                    'rounded-lg border overflow-hidden text-xs',
                    jsonError ? 'border-destructive' : 'border-border'
                  )}
                >
                  <CodeMirror
                    value={jsonText}
                    height="280px"
                    extensions={[json()]}
                    onChange={handleJsonChange}
                  />
                </div>
                {jsonError && (
                  <div className="flex items-center gap-1.5 text-xs text-destructive bg-destructive/10 border border-destructive/20 p-2 rounded-md">
                    <AlertCircle className="h-3.5 w-3.5 shrink-0" />
                    <span>{jsonError}</span>
                  </div>
                )}
              </div>
            )}
          </div>
        ) : (
          /* Ports & Contracts Tab */
          <div key="ports" className="flex flex-col gap-3 animate-in fade-in-50 duration-150">
            <PropertyList title="Inputs" properties={pluginInfo?.inputs || []} />
            <PropertyList title="Outputs" properties={pluginInfo?.outputs || []} />
            <PortList ports={pluginInfo?.ports || []} />
          </div>
        )}
      </div>
    </div>
  );
};

// -------------------------------------------------------------
// WORKFLOW INSPECTOR SUB-COMPONENT
// -------------------------------------------------------------
interface WorkflowInspectorViewProps {
  workflow: WorkflowConfig | null;
  nodeList: { id: string; name: string }[];
  onClose: () => void;
  onUpdateWorkflow: (updates: Partial<WorkflowConfig>) => void;
  validationResult?: DAGValidationResult;
}

const WorkflowInspectorView: React.FC<WorkflowInspectorViewProps> = ({
  workflow,
  nodeList,
  onClose,
  onUpdateWorkflow,
  validationResult,
}) => {
  const [name, setName] = useState(workflow?.name || '');
  const [version, setVersion] = useState(workflow?.version || '1.0.0');
  const [schedulerMode, setSchedulerMode] = useState<WorkflowConfig['scheduler_mode']>(
    workflow?.scheduler_mode || 'thread-per-node'
  );
  const [schedulerWorkers, setSchedulerWorkers] = useState<number>(
    workflow?.scheduler_workers || 0
  );

  const handleNameBlur = () => {
    if (name.trim() && name !== workflow?.name) {
      onUpdateWorkflow({ name: name.trim() });
    }
  };

  const handleVersionBlur = () => {
    if (version.trim() && version !== workflow?.version) {
      onUpdateWorkflow({ version: version.trim() });
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="px-3.5 py-2.5 border-b border-border/80 flex items-center justify-between bg-sunken/20">
        <div className="flex items-center gap-2">
          <div className="w-6 h-6 rounded-md bg-primary/10 text-primary flex items-center justify-center shrink-0">
            <WorkflowIcon size={14} />
          </div>
          <div className="font-semibold text-xs text-foreground">Workflow Settings</div>
        </div>

        <Tooltip title="Close Inspector">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onClose}
            className="h-6 w-6 text-muted-foreground hover:text-foreground"
          >
            <X size={14} />
          </Button>
        </Tooltip>
      </div>

      <div className="flex-1 overflow-y-auto p-3.5 flex flex-col gap-4 text-xs">
        {/* Basic Info */}
        <div className="flex flex-col gap-2.5">
          <span className="text-[10px] font-bold tracking-wider text-muted-foreground uppercase">
            METADATA
          </span>

          <div>
            <label className="text-[11px] font-medium text-foreground mb-1 block">Workflow Name</label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              onBlur={handleNameBlur}
              placeholder="e.g. VoiceAgent"
              className="h-8 text-xs"
            />
          </div>

          <div>
            <label className="text-[11px] font-medium text-foreground mb-1 block">Version</label>
            <Input
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              onBlur={handleVersionBlur}
              placeholder="1.0.0"
              className="h-8 text-xs font-mono"
            />
          </div>

          <div>
            <label className="text-[11px] font-medium text-foreground mb-1 block">Entry Node (Head)</label>
            <Select
              value={workflow?.head || ''}
              onValueChange={(val) => onUpdateWorkflow({ head: val })}
            >
              <SelectTrigger className="h-8 text-xs">
                <SelectValue placeholder="Select entry node...">
                  {nodeList.find((n) => n.id === workflow?.head)?.name || undefined}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {nodeList.map((n) => (
                  <SelectItem key={n.id} value={n.id}>
                    {n.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="h-px bg-border/60" />

        {/* Scheduler */}
        <div className="flex flex-col gap-2.5">
          <span className="text-[10px] font-bold tracking-wider text-muted-foreground uppercase">
            DAG SCHEDULER
          </span>

          <div>
            <label className="text-[11px] font-medium text-foreground mb-1 block">Scheduler Mode</label>
            <Select
              value={schedulerMode || 'thread-per-node'}
              onValueChange={(val) => {
                const mode = val as WorkflowConfig['scheduler_mode'];
                setSchedulerMode(mode);
                onUpdateWorkflow({ scheduler_mode: mode });
              }}
            >
              <SelectTrigger className="h-8 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="thread-per-node">Thread Per Node (Isolated)</SelectItem>
                <SelectItem value="worker-pool">Worker Pool (Cooperative)</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {schedulerMode === 'worker-pool' && (
            <div>
              <label className="text-[11px] font-medium text-foreground mb-1 block">
                Worker Pool Threads
              </label>
              <Input
                type="number"
                min={0}
                value={schedulerWorkers}
                onChange={(e) => {
                  const num = Math.max(0, parseInt(e.target.value, 10) || 0);
                  setSchedulerWorkers(num);
                  onUpdateWorkflow({ scheduler_workers: num });
                }}
                className="h-8 text-xs font-mono"
              />
              <span className="text-[10px] text-muted-foreground mt-0.5 block">
                0 = auto-scaled to 1/4 nodes (min 1).
              </span>
            </div>
          )}
        </div>

        <div className="h-px bg-border/60" />

        {/* Validation & Health Status */}
        <div className="flex flex-col gap-2">
          <span className="text-[10px] font-bold tracking-wider text-muted-foreground uppercase">
            GRAPH DIAGNOSTICS
          </span>

          {validationResult?.errors.length === 0 && validationResult?.warnings.length === 0 ? (
            <div className="flex items-center gap-2 p-2 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-600 dark:text-emerald-400 text-xs">
              <CheckCircle2 size={15} className="shrink-0" />
              <span>DAG topology is fully valid and runnable.</span>
            </div>
          ) : (
            <div className="flex flex-col gap-1.5">
              {validationResult?.errors.map((err, idx) => (
                <div
                  key={`err-${idx}`}
                  className="flex items-start gap-1.5 p-2 rounded-md bg-destructive/10 border border-destructive/20 text-destructive text-[11px]"
                >
                  <AlertCircle size={13} className="shrink-0 mt-0.5" />
                  <span>{err.message}</span>
                </div>
              ))}
              {validationResult?.warnings.map((warn, idx) => (
                <div
                  key={`warn-${idx}`}
                  className="flex items-start gap-1.5 p-2 rounded-md bg-amber-500/10 border border-amber-500/20 text-amber-600 dark:text-amber-400 text-[11px]"
                >
                  <AlertTriangle size={13} className="shrink-0 mt-0.5" />
                  <span>{warn.message}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

// -------------------------------------------------------------
// PROPERTY & PORT HELPERS
// -------------------------------------------------------------
const PropertyList: React.FC<{
  title: string;
  properties: Property[];
}> = ({ title, properties }) => {
  if (!properties || properties.length === 0) return null;

  const getIcon = (prefix: string) => {
    switch (prefix) {
      case 'signal':
        return <Terminal size={12} className="text-sky-500" />;
      case 'payload':
        return <Database size={12} className="text-purple-500" />;
      case 'audio':
        return <Mic size={12} className="text-emerald-500" />;
      case 'video':
        return <Video size={12} className="text-amber-500" />;
      default:
        return <ArrowRightLeft size={12} className="text-muted-foreground" />;
    }
  };

  return (
    <div className="mb-2">
      <div className="text-[10px] font-bold tracking-wider text-muted-foreground uppercase mb-1.5 flex items-center justify-between">
        <span>{title}</span>
        <span>{properties.length}</span>
      </div>
      <div className="flex flex-col gap-1">
        {properties.map((p, i) => (
          <div
            key={`${title}-${i}`}
            className="flex flex-col gap-1 p-1.5 rounded-md bg-sunken/40 border border-border/60 text-xs"
          >
            <div className="flex items-center gap-1.5">
              <span className="flex items-center gap-1 font-mono text-[9px] px-1 py-0.2 rounded bg-card border border-border/60">
                {getIcon(p.prefix)}
                {p.prefix}
              </span>
              <span className="font-semibold text-foreground text-[11px] truncate">{p.name || '*'}</span>
            </div>

            {p.fields && p.fields.length > 0 && (
              <div className="flex flex-wrap gap-1 mt-0.5">
                {p.fields.map((f: Field) => (
                  <Tooltip key={f.key} title={`${f.key}: ${f.type}${f.required ? ' (required)' : ''}`}>
                    <span
                      className={cn(
                        'px-1 py-0.2 rounded text-[9px] font-mono border',
                        f.required
                          ? 'border-border text-foreground bg-card'
                          : 'border-dashed border-border/60 text-muted-foreground'
                      )}
                    >
                      {f.key}
                    </span>
                  </Tooltip>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
};

const PortList: React.FC<{ ports: PortMetadata[] }> = ({ ports }) => {
  if (!ports || ports.length === 0) return null;

  return (
    <div className="mb-2">
      <div className="text-[10px] font-bold tracking-wider text-amber-500 uppercase mb-1.5 flex items-center justify-between">
        <span>PORTS</span>
        <span>{ports.length}</span>
      </div>
      <div className="flex flex-col gap-1">
        {ports.map((p, i) => (
          <div
            key={`port-${i}`}
            className="flex items-center gap-2 p-1.5 rounded-md bg-sunken/40 border border-border/60 text-xs"
          >
            <Badge variant="warning" className="h-4 px-1 text-[9px] font-mono">
              <Hash size={10} /> {p.port}
            </Badge>
            <div className="truncate">
              <div className="font-medium text-foreground text-[11px] truncate">{p.name}</div>
              {p.description && (
                <div className="text-[10px] text-muted-foreground truncate">{p.description}</div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
