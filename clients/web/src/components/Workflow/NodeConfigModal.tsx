import React, { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Autocomplete,
  Typography,
  Box,
  Divider,
  Chip,
  Stack,
  TextField,
  Tooltip,
  Grid,
  Tabs,
  Tab,
  Alert,
} from '@mui/material';
import Form from '@rjsf/mui';
import validator from '@rjsf/validator-ajv8';
import CodeMirror from '@uiw/react-codemirror';
import { json } from '@codemirror/lang-json';
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
import type { PluginInfo, Property, PortMetadata, Field } from '@/types/workflow';

interface NodeConfigModalProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: { name: string; namespace?: string; plugin: string; config: Record<string, unknown> }) => void;
  nodeData?: { name: string; namespace?: string; plugin: string; config: Record<string, unknown> } | null;
  plugins: PluginInfo[];
  existingNames?: string[];
}

type ConfigMode = 'form' | 'custom';

const stringifyConfig = (config: Record<string, unknown>) => JSON.stringify(config, null, 2);

const parseJsonConfig = (value: string): { config?: Record<string, unknown>; error: string } => {
  try {
    const parsed = JSON.parse(value) as unknown;
    if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
      return { error: 'Configuration must be a JSON object' };
    }
    return { config: parsed as Record<string, unknown>, error: '' };
  } catch (err) {
    return {
      error: err instanceof Error ? err.message : 'Invalid JSON',
    };
  }
};

const normalizeNamespace = (namespace?: string) => namespace || 'local';
const pluginKey = (plugin: PluginInfo) => `${normalizeNamespace(plugin.namespace)}:${plugin.name}`;
const pluginLabel = (plugin: PluginInfo) => `${normalizeNamespace(plugin.namespace)} / ${plugin.name}`;
const findPlugin = (
  plugins: PluginInfo[],
  pluginName?: string,
  namespace?: string,
) =>
  plugins.find(
    (plugin) =>
      plugin.name === pluginName &&
      normalizeNamespace(plugin.namespace) === normalizeNamespace(namespace),
  ) || null;

interface DraftState {
  key: string;
  selectedPlugin: PluginInfo | null;
  formData: Record<string, unknown>;
  name: string;
  error: string;
  configMode: ConfigMode;
  jsonText: string;
  jsonError: string;
  customDirty: boolean;
}

const draftKey = (
  nodeData: NodeConfigModalProps['nodeData'],
  plugins: PluginInfo[],
) =>
  [
    nodeData?.name || '',
    normalizeNamespace(nodeData?.namespace),
    nodeData?.plugin || '',
    JSON.stringify(nodeData?.config || {}),
    plugins.map(pluginKey).join('|'),
  ].join('\n');

const createDraft = (
  nodeData: NodeConfigModalProps['nodeData'],
  plugins: PluginInfo[],
): DraftState => {
  const plugin = findPlugin(plugins, nodeData?.plugin, nodeData?.namespace);
  return {
    key: draftKey(nodeData, plugins),
    selectedPlugin: plugin,
    formData: nodeData?.config || {},
    name: nodeData?.name || '',
    error: '',
    configMode: plugin?.schema ? 'form' : 'custom',
    jsonText: stringifyConfig(nodeData?.config || {}),
    jsonError: '',
    customDirty: false,
  };
};

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
  const [draft, setDraft] = useState<DraftState>(() => createDraft(nodeData, plugins));
  const currentDraftKey = draftKey(nodeData, plugins);
  const activeDraft = open && draft.key !== currentDraftKey ? createDraft(nodeData, plugins) : draft;

  if (open && draft.key !== currentDraftKey) {
    setDraft(activeDraft);
  }

  const { selectedPlugin, formData, name, error, configMode, jsonText, jsonError, customDirty } = activeDraft;

  const handlePluginChange = (_event: React.SyntheticEvent, plugin: PluginInfo | null) => {
    setDraft((prev) => ({
      ...prev,
      selectedPlugin: plugin || null,
      formData: {},
      jsonText: stringifyConfig({}),
      jsonError: '',
      customDirty: false,
      configMode: plugin?.schema ? 'form' : 'custom',
    }));
  };

  const handleConfigModeChange = (_: React.SyntheticEvent, mode: ConfigMode) => {
    if (mode === 'custom' && configMode === 'form' && !customDirty) {
      const nextJsonText = stringifyConfig(formData);
      setDraft((prev) => ({
        ...prev,
        configMode: mode,
        jsonText: nextJsonText,
        jsonError: parseJsonConfig(nextJsonText).error,
      }));
    } else {
      setDraft((prev) => ({ ...prev, configMode: mode }));
    }
  };

  const handleJsonChange = (value: string) => {
    setDraft((prev) => ({
      ...prev,
      jsonText: value,
      jsonError: parseJsonConfig(value).error,
      customDirty: true,
    }));
  };

  const handleNameChange = (val: string) => {
    // Regex: only allow alphanumeric and underscores. Remove spaces/emojis.
    const filtered = val.replace(/[^a-zA-Z0-9_]/g, '');
    let nextError = '';
    // Check uniqueness
    if (filtered !== nodeData?.name && existingNames.includes(filtered)) {
      nextError = 'Node name must be unique in this workflow';
    } else if (!filtered) {
      nextError = 'Node name is required';
    }
    setDraft((prev) => ({
      ...prev,
      name: filtered,
      error: nextError,
    }));
  };

  const handleSave = () => {
    if (!name.trim() || !selectedPlugin || error) return;

    let config = formData;
    if (configMode === 'custom') {
      const result = parseJsonConfig(jsonText);
      if (result.error || !result.config) {
        setDraft((prev) => ({ ...prev, jsonError: result.error }));
        return;
      }
      config = result.config;
    }

    onSave({
      name,
      namespace: selectedPlugin.namespace || '',
      plugin: selectedPlugin.name,
      config,
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
                  <Autocomplete
                    fullWidth
                    size="small"
                    options={plugins}
                    value={selectedPlugin}
                    onChange={handlePluginChange}
                    getOptionLabel={pluginLabel}
                    isOptionEqualToValue={(option, value) => pluginKey(option) === pluginKey(value)}
                    slotProps={{
                      listbox: {
                        sx: { maxHeight: 320, overflow: 'auto' },
                      },
                    }}
                    renderInput={(params) => (
                      <TextField {...params} required label="Plugin" placeholder="Select a plugin..." />
                    )}
                    renderOption={(props, ext) => (
                      <Box
                        component="li"
                        {...props}
                        key={pluginKey(ext)}
                        sx={{ display: 'flex', alignItems: 'center', gap: 1 }}
                      >
                        <Chip
                          label={normalizeNamespace(ext.namespace)}
                          size="small"
                          variant="outlined"
                          sx={{ height: 20, fontSize: '0.65rem' }}
                        />
                        <Typography variant="body2" sx={{ fontWeight: 600 }}>
                          {ext.name}
                        </Typography>
                      </Box>
                    )}
                  />
                </Stack>
              </Box>

              {selectedPlugin && (
                <Box>
                  <Typography variant="caption" fontWeight="bold" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
                    CONFIGURATION
                  </Typography>
                  <Tabs
                    value={configMode}
                    onChange={handleConfigModeChange}
                    sx={{ minHeight: 36, mb: 1, borderBottom: 1, borderColor: 'divider' }}
                  >
                    <Tab label="Form" value="form" disabled={!selectedPlugin.schema} sx={{ minHeight: 36, py: 0 }} />
                    <Tab label="Custom JSON" value="custom" sx={{ minHeight: 36, py: 0 }} />
                  </Tabs>
                  {configMode === 'form' && selectedPlugin.schema ? (
                    <Box className="rjsf-grid" sx={{ p: 2, bgcolor: 'background.default', borderRadius: 1, border: '1px dashed', borderColor: 'divider' }}>
                      <Form
                        schema={selectedPlugin.schema}
                        validator={validator}
                        formData={formData}
                        onChange={(e) => setDraft((prev) => ({ ...prev, formData: e.formData }))}
                        children={<></>}
                      />
                    </Box>
                  ) : (
                    <Stack spacing={1}>
                      <Box sx={{ overflow: 'hidden', borderRadius: 1, border: 1, borderColor: jsonError ? 'error.main' : 'divider' }}>
                        <CodeMirror
                          value={jsonText}
                          height="320px"
                          extensions={[json()]}
                          onChange={handleJsonChange}
                        />
                      </Box>
                      {jsonError && (
                        <Alert severity="error" sx={{ py: 0 }}>
                          {jsonError}
                        </Alert>
                      )}
                    </Stack>
                  )}
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
                      NAMESPACE
                    </Typography>
                    <Chip
                      label={normalizeNamespace(selectedPlugin.namespace)}
                      size="small"
                      variant="outlined"
                      sx={{ mb: 2, height: 22, fontSize: '0.7rem' }}
                    />
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
          disabled={!name.trim() || !selectedPlugin || !!error || (configMode === 'custom' && !!jsonError)}
        >
          {nodeData ? 'Update Node' : 'Add Node'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default NodeConfigModal;
