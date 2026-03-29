import React from 'react';
import {
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Box,
  Typography,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  useTheme,
  alpha,
} from '@mui/material';
import { Puzzle, Info, ExternalLink } from 'lucide-react';
import type { PluginInfo } from '@/types/workflow';
import { useWorkflowStore } from '@/store/useAppStore';

const RenderJSON: React.FC<{ data: Record<string, unknown> }> = ({ data }) => {
  const jsonStr = JSON.stringify(data, null, 2);
  
  const escaped = jsonStr
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');

  const highlighted = escaped.replace(
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g,
    (match) => {
      let color = '#ce9178';
      if (/^"/.test(match)) {
        if (/:$/.test(match)) {
          color = '#9cdcfe';
        }
      } else if (/true|false|null/.test(match)) {
        color = '#569cd6';
      } else if (/^-?\d/.test(match)) {
        color = '#b5cea8';
      }
      return `<span style="color: ${color}">${match}</span>`;
    }
  );

  return (
    <pre 
      style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }} 
      dangerouslySetInnerHTML={{ __html: highlighted }} 
    />
  );
};

const PluginList: React.FC = () => {
  const { plugins } = useWorkflowStore();
  const [viewingPlugin, setViewingPlugin] = React.useState<PluginInfo | null>(null);
  const theme = useTheme();

  return (
    <>
      <Box sx={{ p: 2, pb: 1 }}>
        <Typography variant="caption" color="text.secondary" fontWeight="bold" sx={{ letterSpacing: 0.5 }}>
          AVAILABLE PLUGINS
        </Typography>
      </Box>
      <List>
        {plugins.map((plugin) => (
          <ListItem key={plugin.name} disablePadding>
            <ListItemButton 
              onClick={() => setViewingPlugin(plugin)}
              sx={{
                mx: 1,
                borderRadius: '8px',
                py: 0.7,
                '&:hover': { bgcolor: 'action.hover' }
              }}
            >
              <ListItemIcon sx={{ minWidth: 32 }}>
                <Puzzle size={17} />
              </ListItemIcon>
              <ListItemText
                primary={plugin.name}
                secondary={plugin.description}
                primaryTypographyProps={{ 
                  variant: 'body2', 
                  fontWeight: 600
                }}
                secondaryTypographyProps={{ 
                  variant: 'caption',
                  sx: { 
                    display: '-webkit-box',
                    WebkitLineClamp: 1,
                    WebkitBoxOrient: 'vertical',
                    overflow: 'hidden',
                    fontSize: '0.65rem'
                  }
                }}
              />
              <Info size={14} style={{ opacity: 0.3 }} />
            </ListItemButton>
          </ListItem>
        ))}
        {plugins.length === 0 && (
          <Box sx={{ p: 4, textAlign: 'center' }}>
            <Typography variant="caption" color="text.secondary">
              No plugins found
            </Typography>
          </Box>
        )}
      </List>

      <Dialog 
        open={Boolean(viewingPlugin)} 
        onClose={() => setViewingPlugin(null)}
        maxWidth="md"
        fullWidth
        PaperProps={{
          sx: { borderRadius: '16px' }
        }}
      >
        {viewingPlugin && (
          <>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1.5, pb: 1 }}>
              <Box sx={{ bgcolor: alpha(theme.palette.primary.main, 0.1), p: 1, borderRadius: '10px', display: 'flex' }}>
                <Puzzle size={24} color="var(--primary-color)" />
              </Box>
              <Box>
                <Typography variant="h6" fontWeight="800">{viewingPlugin.name}</Typography>
                <Typography variant="caption" color="text.secondary">Plugin Block Details</Typography>
              </Box>
            </DialogTitle>
            <DialogContent dividers>
              <Typography variant="body2" color="text.secondary" paragraph>
                {viewingPlugin.description || 'No description provided for this plugin.'}
              </Typography>
              
              <Box sx={{ mt: 3 }}>
                <Typography variant="subtitle2" fontWeight="700" color="primary" gutterBottom sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <ExternalLink size={14} /> Inputs ({viewingPlugin.inputs?.length || 0})
                </Typography>
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                  {viewingPlugin.inputs?.map((input, idx) => (
                    <Box key={idx} sx={{ px: 1.5, py: 0.5, bgcolor: 'action.hover', borderRadius: '6px', border: '1px solid var(--border-color)' }}>
                      <Typography variant="caption" fontWeight="600">{input.name}</Typography>
                      <Typography variant="caption" color="text.secondary" sx={{ ml: 0.5, opacity: 0.6 }}>[{input.prefix}]</Typography>
                    </Box>
                  ))}
                  {!viewingPlugin.inputs?.length && <Typography variant="caption" color="text.disabled">None</Typography>}
                </Box>
              </Box>

              <Box sx={{ mt: 3 }}>
                <Typography variant="subtitle2" fontWeight="700" color="primary" gutterBottom sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <ExternalLink size={14} /> Outputs ({viewingPlugin.outputs?.length || 0})
                </Typography>
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                  {viewingPlugin.outputs?.map((output, idx) => (
                    <Box key={idx} sx={{ px: 1.5, py: 0.5, bgcolor: 'action.hover', borderRadius: '6px', border: '1px solid var(--border-color)' }}>
                      <Typography variant="caption" fontWeight="600">{output.name}</Typography>
                      <Typography variant="caption" color="text.secondary" sx={{ ml: 0.5, opacity: 0.6 }}>[{output.prefix}]</Typography>
                    </Box>
                  ))}
                  {!viewingPlugin.outputs?.length && <Typography variant="caption" color="text.disabled">None</Typography>}
                </Box>
              </Box>

              {viewingPlugin.schema && (
                <Box sx={{ mt: 3 }}>
                  <Typography variant="subtitle2" fontWeight="700" gutterBottom>Configuration Schema</Typography>
                  <Box 
                    sx={{ 
                      p: 2, 
                      bgcolor: '#1e1e1e', 
                      color: '#d4d4d4', 
                      borderRadius: '12px', 
                      fontFamily: 'monospace',
                      fontSize: '0.8rem',
                      maxHeight: 350,
                      overflow: 'auto',
                      border: '1px solid rgba(255,255,255,0.05)',
                    }}
                  >
                    <RenderJSON data={viewingPlugin.schema} />
                  </Box>
                </Box>
              )}
            </DialogContent>
            <DialogActions sx={{ p: 2 }}>
              <Button onClick={() => setViewingPlugin(null)} variant="outlined" sx={{ borderRadius: '100px', textTransform: 'none' }}>
                Close
              </Button>
            </DialogActions>
          </>
        )}
      </Dialog>
    </>
  );
};

export default PluginList;
