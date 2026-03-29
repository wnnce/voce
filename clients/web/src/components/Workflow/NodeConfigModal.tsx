import React, { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  FormControl,
  Select,
  MenuItem,
  Typography,
  Box,
  Divider,
  Chip,
  Stack,
  TextField,
  Tooltip,
  Grid,
  type SelectChangeEvent
} from '@mui/material';
import Form from '@rjsf/mui';
import validator from '@rjsf/validator-ajv8';
import type { PluginInfo, Property, PortMetadata, Field } from '@/types/workflow';

interface NodeConfigModalProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: { name: string; plugin: string; config: Record<string, unknown> }) => void;
  nodeData?: { name: string; plugin: string; config: Record<string, unknown> } | null;
  plugins: PluginInfo[];
  existingNames?: string[];
}

import { 
  ChevronRight, 
  Terminal, 
  Database, 
  Mic, 
  Video, 
  ArrowRightLeft, 
  Info,
  Hash
} from 'lucide-react';

// Help component to render a list of properties
const PropertyList: React.FC<{ 
  title: string; 
  properties: Property[]; 
  color: "primary" | "secondary" | "success" | "warning" | "info" 
}> = ({ title, properties, color }) => {
  if (!properties || properties.length === 0) return null;

  const getIcon = (prefix: string) => {
    switch (prefix) {
      case 'signal': return <Terminal size={14} />;
      case 'payload': return <Database size={14} />;
      case 'audio': return <Mic size={14} />;
      case 'video': return <Video size={14} />;
      default: return <Info size={14} />;
    }
  };

  return (
    <Box sx={{ mb: 2 }}>
      <Typography variant="caption" fontWeight="bold" color={`${color}.main`} sx={{ mb: 1, display: 'flex', alignItems: 'center', gap: 0.5 }}>
        {title.toUpperCase()} ({properties.length})
      </Typography>
      <Stack spacing={0.5}>
        {properties.map((p, i) => (
          <Box 
            key={`${title}-${i}`}
            sx={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: 1, 
              p: 0.75, 
              borderRadius: 1, 
              bgcolor: 'action.hover',
              border: 1,
              borderColor: 'divider'
            }}
          >
            <Chip 
              size="small" 
              icon={getIcon(p.prefix)} 
              label={p.prefix} 
              color={color} 
              variant="filled" 
              sx={{ height: 20, fontSize: '0.65rem', weight: 600, '& .MuiChip-icon': { ml: 0.5 } }} 
            />
            <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }}>
              {p.name || '*'}
            </Typography>
            {p.fields && p.fields.length > 0 && (
              <Box sx={{ display: 'flex', gap: 0.5, ml: 'auto' }}>
                {p.fields.map((f: Field) => (
                  <Tooltip key={f.key} title={`${f.key}: ${f.type}${f.required ? ' (required)' : ''}`}>
                    <Chip 
                      label={f.key} 
                      size="small" 
                      variant="outlined" 
                      sx={{ height: 18, fontSize: '0.6rem', borderStyle: f.required ? 'solid' : 'dashed' }} 
                    />
                  </Tooltip>
                ))}
              </Box>
            )}
          </Box>
        ))}
      </Stack>
    </Box>
  );
};

// Component for ports
const PortList: React.FC<{ ports: PortMetadata[] }> = ({ ports }) => {
  if (!ports || ports.length === 0) return null;

  return (
    <Box sx={{ mb: 2 }}>
      <Typography variant="caption" fontWeight="bold" color="warning.main" sx={{ mb: 1, display: 'flex', alignItems: 'center', gap: 0.5 }}>
        PORTS ({ports.length})
      </Typography>
      <Stack spacing={0.5}>
        {ports.map((p, i) => (
          <Box 
            key={`port-${i}`}
            sx={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: 1, 
              p: 0.75, 
              borderRadius: 1, 
              bgcolor: 'action.hover',
              border: 1,
              borderColor: 'divider'
            }}
          >
            <Chip 
              size="small" 
              icon={<Hash size={12} />} 
              label={p.port} 
              color="warning" 
              sx={{ height: 20, fontSize: '0.65rem', fontWeight: 'bold' }} 
            />
            <Box>
              <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }}>
                {p.name}
              </Typography>
              {p.description && (
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1 }}>
                  {p.description}
                </Typography>
              )}
            </Box>
          </Box>
        ))}
      </Stack>
    </Box>
  );
};

const NodeConfigModal: React.FC<NodeConfigModalProps> = ({
  open,
  onClose,
  onSave,
  nodeData,
  plugins,
  existingNames = [],
}) => {
  const [selectedPlugin, setSelectedPlugin] = useState<PluginInfo | null>(() => 
    plugins.find((e) => e.name === nodeData?.plugin) || null
  );
  const [formData, setFormData] = useState<Record<string, unknown>>(() => nodeData?.config || {});
  const [name, setName] = useState(nodeData?.name || '');
  const [error, setError] = useState('');

  const handlePluginChange = (event: SelectChangeEvent) => {
    const pluginName = event.target.value as string;
    const plugin = plugins.find((e) => e.name === pluginName);
    setSelectedPlugin(plugin || null);
    setFormData({}); // Reset config on plugin change
  };

  const handleNameChange = (val: string) => {
    // Regex: only allow alphanumeric and underscores. Remove spaces/emojis.
    const filtered = val.replace(/[^a-zA-Z0-9_]/g, '');
    setName(filtered);
    
    // Check uniqueness
    if (filtered !== nodeData?.name && existingNames.includes(filtered)) {
      setError('Node name must be unique in this workflow');
    } else if (!filtered) {
      setError('Node name is required');
    } else {
      setError('');
    }
  };

  const handleSave = () => {
    if (!name.trim() || !selectedPlugin || error) return;
    onSave({
      name,
      plugin: selectedPlugin.name,
      config: formData,
    });
    onClose();
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ py: 2, display: 'flex', alignItems: 'center', gap: 1 }}>
        <ChevronRight size={24} color="gray" />
        <Typography variant="h6" fontWeight="bold">
          {nodeData ? 'Configure Node' : 'Add New Node'}
        </Typography>
      </DialogTitle>
      <DialogContent dividers sx={{ py: 2 }}>
        <Grid container spacing={3}>
          {/* Left Column: Extension Details & Schema */}
          <Grid size={{ xs: 12, md: 7 }}>
            <Stack spacing={3}>
              <Box>
                <Typography variant="caption" fontWeight="bold" color="text.secondary" sx={{ mb: 1.5, display: 'block' }}>
                  GENERAL SETTINGS
                </Typography>
                <Stack spacing={2}>
                  <TextField
                    fullWidth
                    label="Node Instance Name"
                    value={name}
                    onChange={(e) => handleNameChange(e.target.value)}
                    size="small"
                    required
                    error={!!error}
                    helperText={error || "Only letters, numbers and underscores allowed"}
                    placeholder="e.g. MyAudioFilter"
                  />
                  <FormControl fullWidth size="small" required>
                    <Select 
                      value={selectedPlugin?.name || ''} 
                      onChange={handlePluginChange} 
                      displayEmpty
                    >
                      <MenuItem value="" disabled>
                        <Typography variant="body2" color="text.secondary">Select a plugin...</Typography>
                      </MenuItem>
                      {plugins.map((ext) => (
                        <MenuItem key={ext.name} value={ext.name}>
                          {ext.name}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Stack>
              </Box>

              {selectedPlugin && selectedPlugin.schema && (
                <Box>
                  <Typography variant="caption" fontWeight="bold" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
                    CONFIGURATION
                  </Typography>
                  <Box className="rjsf-grid" sx={{ p: 2, bgcolor: 'background.default', borderRadius: 1, border: '1px dashed', borderColor: 'divider' }}>
                    <Form
                      schema={selectedPlugin.schema}
                      validator={validator}
                      formData={formData}
                      onChange={(e) => setFormData(e.formData)}
                      children={<></>}
                    />
                  </Box>
                </Box>
              )}
            </Stack>
          </Grid>

          {/* Right Column: IO & Metadata */}
          <Grid size={{ xs: 12, md: 5 }}>
            <Box sx={{ height: '100%', borderLeft: { md: '1px solid var(--border-color)' }, pl: { md: 3 } }}>
              {selectedPlugin ? (
                <Stack spacing={1}>
                  <Box sx={{ mb: 2 }}>
                    <Typography variant="caption" fontWeight="bold" color="text.secondary" sx={{ mb: 0.5, display: 'block' }}>
                      DESCRIPTION
                    </Typography>
                    <Typography variant="body2" sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                      {selectedPlugin.description || 'No description provided.'}
                    </Typography>
                  </Box>

                  <Divider sx={{ my: 2 }} />

                  <PropertyList title="Inputs" properties={selectedPlugin.inputs || []} color="primary" />
                  <PropertyList title="Outputs" properties={selectedPlugin.outputs || []} color="secondary" />
                  <PortList ports={selectedPlugin.ports || []} />
                </Stack>
              ) : (
                <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', opacity: 0.5 }}>
                  <ArrowRightLeft size={48} />
                  <Typography variant="caption" sx={{ mt: 2 }}>Select a plugin to see details</Typography>
                </Box>
              )}
            </Box>
          </Grid>
        </Grid>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={onClose}>Cancel</Button>
        <Button 
          onClick={handleSave} 
          variant="contained" 
          color="primary"
          disabled={!name.trim() || !selectedPlugin || !!error}
        >
          {nodeData ? 'Update Node' : 'Add Node'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default NodeConfigModal;
