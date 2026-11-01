import { memo, useMemo } from 'react';
import { Handle, Position, useEdges, useNodes, useNodeId, type NodeProps } from 'reactflow';
import { Workflow, Terminal, Database, Mic, Video, Crown } from 'lucide-react';
import { EventType, type PluginInfo } from '@/types/workflow';
import { Tooltip } from '@/components/ui/tooltip';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

const TYPE_COLORS = {
  [EventType.SIGNAL]: 'var(--port-signal)',
  [EventType.PAYLOAD]: 'var(--port-payload)',
  [EventType.AUDIO]: 'var(--port-audio)',
  [EventType.VIDEO]: 'var(--port-video)',
};

const TYPE_TEXT_COLORS = {
  [EventType.SIGNAL]: 'text-sky-500',
  [EventType.PAYLOAD]: 'text-purple-500',
  [EventType.AUDIO]: 'text-emerald-500',
  [EventType.VIDEO]: 'text-amber-500',
};

const TYPE_ICONS = {
  [EventType.SIGNAL]: <Terminal size={12} className="text-sky-500" />,
  [EventType.PAYLOAD]: <Database size={12} className="text-purple-500" />,
  [EventType.AUDIO]: <Mic size={12} className="text-emerald-500" />,
  [EventType.VIDEO]: <Video size={12} className="text-amber-500" />,
};

interface WorkflowNodeData {
  name: string;
  plugin: string;
  config: Record<string, unknown>;
  pluginInfo?: PluginInfo;
  isHead?: boolean;
  onSetAsHead?: () => void;
}

const CustomNode = ({ data, selected }: NodeProps<WorkflowNodeData>) => {
  const edges = useEdges();
  const nodes = useNodes<WorkflowNodeData>();
  const nodeId = useNodeId();
  const pluginInfo: PluginInfo | undefined = data.pluginInfo;
  const eventTypes = [EventType.SIGNAL, EventType.PAYLOAD, EventType.AUDIO, EventType.VIDEO] as const;
  const ports = useMemo(() => pluginInfo?.ports || [], [pluginInfo]);

  const renderDataTooltip = (type: number, isInput: boolean, portId?: number) => {
    const prefix = ({
      [EventType.SIGNAL]: 'signal',
      [EventType.PAYLOAD]: 'payload',
      [EventType.AUDIO]: 'audio',
      [EventType.VIDEO]: 'video',
    } as Record<number, string>)[type] || 'unknown';

    const properties = isInput
      ? (pluginInfo?.inputs || []).filter((i) => i.prefix === prefix)
      : (pluginInfo?.outputs || []).filter((o) => o.prefix === prefix);

    if (properties.length === 0) return `No ${prefix} data defined`;

    return (
      <div className="p-1 max-w-xs text-xs">
        <div className="font-bold border-b border-border/60 pb-1 mb-1 tracking-wider uppercase text-[10px] text-muted-foreground">
          {isInput ? 'INPUT' : 'OUTPUT'}: {prefix} {portId ? `(Port ${portId})` : ''}
        </div>
        {properties.map((prop, idx) => (
          <div key={idx} className="mb-1 last:mb-0">
            {prop.name && <div className="font-medium italic text-[11px]">• {prop.name}</div>}
            {prop.fields?.map((field, fIdx) => (
              <div key={fIdx} className="pl-2 flex items-center gap-1 text-[10px]">
                <span className="font-mono text-muted-foreground">{field.key}:</span>
                <span className="text-primary font-mono">{field.type}</span>
                {field.required && <span className="text-destructive">*</span>}
              </div>
            ))}
            {(!prop.fields || prop.fields.length === 0) && (
              <div className="pl-2 text-[10px] text-muted-foreground italic">(Signal only)</div>
            )}
          </div>
        ))}
      </div>
    );
  };

  return (
    <div
      className={cn(
        'w-[260px] rounded-xl border bg-card/95 backdrop-blur-md shadow-sm transition-all duration-200 select-none',
        selected
          ? 'border-primary ring-1 ring-primary shadow-md'
          : 'border-border/80 hover:border-border-strong hover:shadow-md hover:-translate-y-0.5',
        data.isHead && 'ring-1 ring-amber-500/50'
      )}
    >
      {/* Header */}
      <div className="px-3 py-2 flex items-center gap-2 border-b border-border/80 bg-sunken/30 rounded-t-xl">
        <div
          className={cn(
            'flex items-center justify-center w-6 h-6 rounded-md shrink-0 text-white transition-transform duration-150 hover:scale-105',
            data.isHead ? 'bg-amber-500 shadow-xs' : 'bg-primary'
          )}
        >
          {data.isHead ? <Crown size={14} /> : <Workflow size={14} />}
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5">
            <span className="text-xs font-semibold text-foreground truncate flex-1">
              {data.name || 'Untitled Node'}
            </span>
            {data.isHead && (
              <Badge variant="warning" className="h-4 px-1 text-[9px] font-black uppercase">
                HEAD
              </Badge>
            )}
            {!data.isHead && data.onSetAsHead && (
              <Tooltip title="Set as Entry Head Node">
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    data.onSetAsHead?.();
                  }}
                  className="text-muted-foreground/50 hover:text-amber-500 p-0.5 rounded cursor-pointer transition-transform duration-150 hover:scale-115 active:scale-90"
                >
                  <Crown size={13} />
                </button>
              </Tooltip>
            )}
          </div>
          <div className="text-[10px] text-muted-foreground truncate font-mono">
            {pluginInfo?.namespace ? `${pluginInfo.namespace} / ${data.plugin}` : data.plugin || 'Generic'}
          </div>
        </div>
      </div>

      {/* Body with Three Columns */}
      <div className="flex min-h-[110px]">
        {/* Left Input Zone */}
        <div className="w-8 flex flex-col justify-around items-center py-2 border-r border-border/50 bg-sunken/20">
          {eventTypes.map((type) => {
            const prefix = {
              [EventType.SIGNAL]: 'signal',
              [EventType.PAYLOAD]: 'payload',
              [EventType.AUDIO]: 'audio',
              [EventType.VIDEO]: 'video',
            }[type];

            const isRequired = pluginInfo?.inputs?.some(
              (i) => i.prefix === prefix && (!i.fields || i.fields.length === 0 || i.fields.some((f) => f.required))
            );

            const isConnected = edges.some(
              (e) => e.target === nodeId && e.targetHandle === `target-${type}`
            );
            const isMissing = isRequired && !isConnected;

            return (
              <div key={`in-${type}`} className="relative flex items-center justify-center w-full">
                <Tooltip title={renderDataTooltip(type, true)} side="left">
                  <span
                    className={cn(
                      'cursor-help p-0.5 flex items-center justify-center',
                      isMissing && 'animate-pulse text-destructive'
                    )}
                  >
                    {TYPE_ICONS[type]}
                  </span>
                </Tooltip>
                <Handle
                  type="target"
                  position={Position.Left}
                  id={`target-${type}`}
                  className="transition-transform duration-150 hover:scale-135 cursor-crosshair"
                  style={{
                    left: -5,
                    background: TYPE_COLORS[type],
                    width: 9,
                    height: 9,
                    border: '2px solid var(--bg-card)',
                    boxShadow: '0 0 2px rgba(0,0,0,0.3)',
                  }}
                />
              </div>
            );
          })}
        </div>

        {/* Central Content */}
        <div className="flex-1 flex flex-col justify-around py-2 px-3">
          {eventTypes.map((type) => {
            const upstreamNames = edges
              .filter((e) => e.target === nodeId && e.targetHandle === `target-${type}`)
              .map((e) => nodes.find((n) => n.id === e.source)?.data?.name || 'Unknown');

            const downstreamNames = edges
              .filter((e) => e.source === nodeId && e.sourceHandle?.startsWith(`source-${type}`))
              .map((e) => nodes.find((n) => n.id === e.target)?.data?.name || 'Unknown');

            return (
              <div key={`center-${type}`} className="flex items-center justify-center h-5 gap-2">
                <Tooltip
                  title={
                    upstreamNames.length > 0 ? (
                      <div className="p-1 text-[11px]">
                        <div className="font-semibold text-muted-foreground uppercase text-[9px] mb-0.5">
                          UPSTREAM ({upstreamNames.length})
                        </div>
                        {upstreamNames.map((n, i) => (
                          <div key={i}>• {n}</div>
                        ))}
                      </div>
                    ) : (
                      'No upstream connection'
                    )
                  }
                >
                  <span
                    className={cn(
                      'font-mono text-xs font-semibold px-1.5 py-0.2 rounded cursor-help transition-all duration-150 hover:scale-105 active:scale-95',
                      upstreamNames.length > 0
                        ? `${TYPE_TEXT_COLORS[type]} bg-sunken/60 border border-border/40`
                        : 'text-muted-foreground/40'
                    )}
                  >
                    {upstreamNames.length}
                  </span>
                </Tooltip>

                <span className="w-px h-2.5 bg-border/60" />

                <Tooltip
                  title={
                    downstreamNames.length > 0 ? (
                      <div className="p-1 text-[11px]">
                        <div className="font-semibold text-muted-foreground uppercase text-[9px] mb-0.5">
                          DOWNSTREAM ({downstreamNames.length})
                        </div>
                        {downstreamNames.map((n, i) => (
                          <div key={i}>• {n}</div>
                        ))}
                      </div>
                    ) : (
                      'No downstream connection'
                    )
                  }
                >
                  <span
                    className={cn(
                      'font-mono text-xs font-semibold px-1.5 py-0.2 rounded cursor-help transition-all duration-150 hover:scale-105 active:scale-95',
                      downstreamNames.length > 0
                        ? `${TYPE_TEXT_COLORS[type]} bg-sunken/60 border border-border/40`
                        : 'text-muted-foreground/40'
                    )}
                  >
                    {downstreamNames.length}
                  </span>
                </Tooltip>
              </div>
            );
          })}
        </div>

        {/* Right Output Zone */}
        <div className="w-10 flex flex-col justify-start items-center py-2 gap-1 border-l border-border/50 bg-sunken/20">
          {/* Broadcast Outputs */}
          {eventTypes.map((type) => (
            <div
              key={`out-${type}-0`}
              className="relative flex items-center justify-center w-full mb-0.5"
            >
              <Tooltip title={renderDataTooltip(type, false)} side="right">
                <span className="cursor-help p-0.5 flex items-center justify-center">
                  {TYPE_ICONS[type]}
                </span>
              </Tooltip>
              <Handle
                type="source"
                position={Position.Right}
                id={`source-${type}-0`}
                className="transition-transform duration-150 hover:scale-135 cursor-crosshair"
                style={{
                  right: -5,
                  background: TYPE_COLORS[type],
                  width: 9,
                  height: 9,
                  border: '2px solid var(--bg-card)',
                  boxShadow: '0 0 2px rgba(0,0,0,0.3)',
                }}
              />
            </div>
          ))}

          {ports.length > 0 && <div className="w-4 h-px bg-border/60 my-0.5" />}

          {/* Dedicated Ports */}
          {ports.map((port) => (
            <div
              key={`out-port-${port.port}`}
              className="relative flex items-center justify-center w-full"
            >
              <Tooltip title={renderDataTooltip(port.type, false, port.port)} side="right">
                <span
                  className="w-5 h-4 rounded text-[9px] font-mono font-bold border flex items-center justify-center cursor-help transition-transform duration-150 hover:scale-105"
                  style={{
                    color: TYPE_COLORS[port.type],
                    borderColor: `${TYPE_COLORS[port.type]}40`,
                    background: `${TYPE_COLORS[port.type]}15`,
                  }}
                >
                  {port.port}
                </span>
              </Tooltip>
              <Handle
                type="source"
                position={Position.Right}
                id={`source-${port.type}-${port.port}`}
                className="transition-transform duration-150 hover:scale-135 cursor-crosshair"
                style={{
                  right: -5,
                  background: TYPE_COLORS[port.type] || '#ccc',
                  width: 9,
                  height: 9,
                  border: '2px solid var(--bg-card)',
                  boxShadow: '0 0 2px rgba(0,0,0,0.3)',
                }}
              />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

export default memo(CustomNode);
