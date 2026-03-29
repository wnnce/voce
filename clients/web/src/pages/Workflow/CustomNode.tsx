import { memo, useMemo } from 'react';
import { Handle, Position, useEdges, useNodes, useNodeId, type NodeProps } from 'reactflow';
import { Paper, Typography, Box, useTheme, Divider, Tooltip } from '@mui/material';
import { Workflow, Terminal, Database, Mic, Video } from 'lucide-react';
import { EventType, type PluginInfo } from '@/types/workflow';

const TYPE_COLORS = {
  [EventType.SIGNAL]: '#3b82f6', // blue
  [EventType.PAYLOAD]: '#8b5cf6', // purple
  [EventType.AUDIO]: '#10b981', // green
  [EventType.VIDEO]: '#f59e0b', // orange
};

const TYPE_ICONS = {
  [EventType.SIGNAL]: <Terminal size={12} />,
  [EventType.PAYLOAD]: <Database size={12} />,
  [EventType.AUDIO]: <Mic size={12} />,
  [EventType.VIDEO]: <Video size={12} />,
};

interface WorkflowNodeData {
  name: string;
  plugin: string;
  config: Record<string, unknown>;
  pluginInfo?: PluginInfo;
}

const CustomNode = ({ data, selected }: NodeProps<WorkflowNodeData>) => {
  const theme = useTheme();
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
      ? (pluginInfo?.inputs || []).filter(i => i.prefix === prefix)
      : (pluginInfo?.outputs || []).filter(o => o.prefix === prefix);

    if (properties.length === 0) return `No ${prefix} data defined`;

    return (
      <Box sx={{ p: 0.5 }}>
        <Typography variant="caption" fontWeight="bold" sx={{ display: 'block', mb: 0.5, borderBottom: '1px solid rgba(255,255,255,0.2)' }}>
          {isInput ? 'INPUT' : 'OUTPUT'}: {prefix.toUpperCase()} {portId ? `(Port ${portId})` : ''}
        </Typography>
        {properties.map((prop, idx) => (
          <Box key={idx} sx={{ mb: idx === properties.length - 1 ? 0 : 1 }}>
            {prop.name && (
              <Typography variant="caption" sx={{ display: 'block', fontStyle: 'italic', opacity: 0.8 }}>
                • {prop.name}
              </Typography>
            )}
            {prop.fields?.map((field, fIdx) => (
              <Box key={fIdx} sx={{ pl: 1.5, display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <Typography variant="caption" sx={{ fontSize: '0.65rem' }}>
                  {field.key}: <Box component="span" sx={{ color: 'primary.light' }}>{field.type}</Box>
                </Typography>
                {field.required && (
                  <Typography variant="caption" sx={{ fontSize: '0.6rem', color: 'error.light' }}>*</Typography>
                )}
              </Box>
            ))}
            {(!prop.fields || prop.fields.length === 0) && (
              <Typography variant="caption" sx={{ pl: 1.5, fontSize: '0.65rem', opacity: 0.6 }}>
                (Signal only)
              </Typography>
            )}
          </Box>
        ))}
      </Box>
    );
  };

  return (
    <Paper
      elevation={0}
      sx={{
        width: 260,
        overflow: 'visible',
        border: '1px solid',
        borderColor: selected ? theme.palette.primary.main : 'var(--border-color)',
        bgcolor: 'background.paper',
        borderRadius: '12px',
        transition: 'all 0.1s ease-out',
        boxShadow: selected ? `0 0 0 1px ${theme.palette.primary.main}` : 'none',
      }}
    >
      {/* Header */}
      <Box sx={{ 
        p: '10px 12px', 
        display: 'flex', 
        alignItems: 'center', 
        gap: 1.5, 
        bgcolor: 'background.paper', // Unified white card aesthetic
        borderTopLeftRadius: '12px', 
        borderTopRightRadius: '12px',
        borderBottom: '1px solid var(--border-color)'
      }}>
        <Box sx={{ 
          display: 'flex', 
          alignItems: 'center', 
          justifyContent: 'center', 
          width: 28, 
          height: 28, 
          borderRadius: '8px', 
          bgcolor: 'primary.main', 
          color: 'white' 
        }}>
          <Workflow size={18} />
        </Box>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography variant="body2" fontWeight="800" noWrap>
            {data.name || 'Untitled Node'}
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem', display: 'block', lineHeight: 1 }}>
            {data.plugin || 'Generic'}
          </Typography>
        </Box>
      </Box>

      {/* Body with Three Columns */}
      <Box sx={{ display: 'flex', minHeight: 120 }}>
        {/* Left Input Zone */}
        <Box sx={{ 
          width: 36, 
          display: 'flex', 
          flexDirection: 'column', 
          justifyContent: 'space-around', 
          alignItems: 'center',
          py: 2,
          borderRight: '1px solid rgba(0,0,0,0.05)',
          bgcolor: 'rgba(0,0,0,0.01)'
        }}>
          {(() => {
            return eventTypes.map((type) => {
              const prefix = {
                [EventType.SIGNAL]: 'signal',
                [EventType.PAYLOAD]: 'payload',
                [EventType.AUDIO]: 'audio',
                [EventType.VIDEO]: 'video',
              }[type];

              const isRequired = pluginInfo?.inputs?.some(i => i.prefix === prefix && (
                !i.fields || i.fields.length === 0 || i.fields.some(f => f.required)
              ));

              const isConnected = edges.some(e => e.target === nodeId && e.targetHandle === `target-${type}`);
              const isMissing = isRequired && !isConnected;

              return (
                <Box key={`in-${type}`} sx={{ position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center', width: '100%' }}>
                  <Tooltip 
                    title={renderDataTooltip(type, true)} 
                    placement="left"
                    arrow
                  >
                    <Box sx={{ color: isMissing ? 'error.main' : TYPE_COLORS[type], display: 'flex', alignItems: 'center', justifyContent: 'center', opacity: 0.9 }}>
                      {TYPE_ICONS[type]}
                    </Box>
                  </Tooltip>
                  <Handle
                    type="target"
                    position={Position.Left}
                    id={`target-${type}`}
                    style={{ 
                      left: -6, 
                      background: TYPE_COLORS[type],
                      width: 10,
                      height: 10,
                      border: '2px solid #fff',
                      boxShadow: '0 0 4px rgba(0,0,0,0.2)'
                    }}
                  />
                </Box>
              );
            });
          })()}
        </Box>

        {/* Central Content */}
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'space-around', py: 2, px: 2 }}>
          {(() => {
            return eventTypes.map((type) => {
              const upstreamNames = edges
                .filter(e => e.target === nodeId && e.targetHandle === `target-${type}`)
                .map(e => nodes.find(n => n.id === e.source)?.data?.name || 'Unknown');
              
              const downstreamNames = edges
                .filter(e => e.source === nodeId && e.sourceHandle?.startsWith(`source-${type}`))
                .map(e => nodes.find(n => n.id === e.target)?.data?.name || 'Unknown');

              return (
                <Box 
                  key={`center-${type}`} 
                  sx={{ 
                    display: 'flex', 
                    alignItems: 'center', 
                    justifyContent: 'center', 
                    height: 24, 
                    gap: 1.5
                  }}
                >
                  <Tooltip 
                    arrow
                    title={upstreamNames.length > 0 ? (
                      <Box sx={{ p: 0.5 }}>
                        <Typography variant="caption" fontWeight="bold" sx={{ display: 'block', mb: 0.5 }}>UPSTREAM:</Typography>
                        {upstreamNames.map((name, i) => <Typography key={i} variant="caption" sx={{ display: 'block' }}>• {name}</Typography>)}
                      </Box>
                    ) : "No upstream connection"}
                  >
                    <Typography 
                      variant="caption" 
                      sx={{ 
                        fontWeight: '800', 
                        color: upstreamNames.length > 0 ? TYPE_COLORS[type] : 'text.disabled',
                        bgcolor: upstreamNames.length > 0 ? `${TYPE_COLORS[type]}11` : 'transparent',
                        px: 0.8,
                        borderRadius: '4px',
                        fontSize: '0.75rem',
                        cursor: 'help'
                      }}
                    >
                      {upstreamNames.length}
                    </Typography>
                  </Tooltip>
                  
                  <Box sx={{ width: 1, height: 10, bgcolor: 'divider', opacity: 0.3, borderRadius: 1 }} />
                  
                  <Tooltip 
                    arrow
                    title={downstreamNames.length > 0 ? (
                      <Box sx={{ p: 0.5 }}>
                        <Typography variant="caption" fontWeight="bold" sx={{ display: 'block', mb: 0.5 }}>DOWNSTREAM:</Typography>
                        {downstreamNames.map((name, i) => <Typography key={i} variant="caption" sx={{ display: 'block' }}>• {name}</Typography>)}
                      </Box>
                    ) : "No downstream connection"}
                  >
                    <Typography 
                      variant="caption" 
                      sx={{ 
                        fontWeight: '800', 
                        color: downstreamNames.length > 0 ? TYPE_COLORS[type] : 'text.disabled',
                        bgcolor: downstreamNames.length > 0 ? `${TYPE_COLORS[type]}11` : 'transparent',
                        px: 0.8,
                        borderRadius: '4px',
                        fontSize: '0.75rem',
                        cursor: 'help'
                      }}
                    >
                      {downstreamNames.length}
                    </Typography>
                  </Tooltip>
                </Box>
              );
            });
          })()}
        </Box>

        {/* Right Output Zone */}
        <Box sx={{ 
          width: 48, 
          display: 'flex', 
          flexDirection: 'column', 
          gap: 1,
          justifyContent: 'flex-start',
          alignItems: 'center',
          py: 2,
          borderLeft: '1px solid rgba(0,0,0,0.05)',
          bgcolor: 'rgba(0,0,0,0.01)'
        }}>
          {/* Broadcast Outputs */}
          {eventTypes.map((type) => (
            <Box key={`out-${type}-0`} sx={{ position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center', width: '100%', mb: 0.5 }}>
              <Tooltip 
                title={renderDataTooltip(type, false)} 
                placement="right"
                arrow
              >
                <Box sx={{ color: TYPE_COLORS[type], display: 'flex', opacity: 0.9 }}>
                  {TYPE_ICONS[type]}
                </Box>
              </Tooltip>
              <Handle
                type="source"
                position={Position.Right}
                id={`source-${type}-0`}
                style={{ 
                  right: -6, 
                  background: TYPE_COLORS[type],
                  width: 10,
                  height: 10,
                  border: '2px solid #fff',
                  boxShadow: '0 0 4px rgba(0,0,0,0.2)'
                }}
              />
            </Box>
          ))}
          
          {ports.length > 0 && <Divider sx={{ width: '60%', my: 0.5 }} />}

          {/* Dedicated Ports */}
          {ports.map((port) => (
            <Box key={`out-port-${port.port}`} sx={{ position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center', width: '100%' }}>
              <Tooltip title={renderDataTooltip(port.type, false, port.port)} placement="right" arrow>
                <Box sx={{ 
                  width: 24, 
                  height: 18, 
                  borderRadius: '4px', 
                  bgcolor: TYPE_COLORS[port.type] ? `${TYPE_COLORS[port.type]}22` : 'action.selected',
                  border: `1px solid ${TYPE_COLORS[port.type] || 'transparent'}`,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'help'
                }}>
                  <Typography sx={{ fontSize: '0.6rem', fontWeight: 'bold', color: TYPE_COLORS[port.type] }}>
                    {port.port}
                  </Typography>
                </Box>
              </Tooltip>
              <Handle
                type="source"
                position={Position.Right}
                id={`source-${port.type}-${port.port}`}
                style={{ 
                  right: -6, 
                  background: TYPE_COLORS[port.type] || '#ccc',
                  width: 10,
                  height: 10,
                  border: '2px solid #fff',
                  boxShadow: '0 0 4px rgba(0,0,0,0.2)'
                }}
              />
            </Box>
          ))}
        </Box>
      </Box>
    </Paper>
  );
};

export default memo(CustomNode);
