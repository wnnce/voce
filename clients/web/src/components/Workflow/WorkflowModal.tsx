import React, { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Stack,
  Typography,
  Box,
} from '@mui/material';
import type { WorkflowConfig } from '@/types/workflow';

interface WorkflowModalProps {
  open: boolean;
  onClose: () => void;
  onSave: (wf: Partial<WorkflowConfig>) => void;
  initialData?: Partial<WorkflowConfig>;
  nodes?: { id: string, name: string }[];
  existingWorkflows?: WorkflowConfig[];
}

const WorkflowModal: React.FC<WorkflowModalProps> = ({ open, onClose, onSave, initialData, nodes, existingWorkflows }) => {
  const [name, setName] = useState(initialData?.name || '');
  const [version, setVersion] = useState(initialData?.version || '1.0.0');
  const [head, setHead] = useState(initialData?.head || (nodes?.length ? nodes[0].id : ''));
  const [error, setError] = useState<string | null>(null);

  const handleNameChange = (val: string) => {
    // Sanitize: No spaces, no emojis, only alphanumeric/Chinese/underscore
    const sanitized = val.replace(/[^a-zA-Z0-9_\u4e00-\u9fa5]/g, '');
    setName(sanitized);
    
    // Uniqueness check
    if (existingWorkflows) {
      const exists = existingWorkflows.some(wf => 
        wf.name.toLowerCase() === sanitized.toLowerCase() && wf.id !== initialData?.id
      );
      if (exists) {
        setError('A workflow with this name already exists');
      } else {
        setError(null);
      }
    }
  };

  const handleSave = () => {
    if (!name.trim() || error) return;
    onSave({ name, version, head });
    onClose();
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ py: 2 }}>
        <Typography variant="h6" fontWeight="bold">
          {initialData?.id ? 'Edit Workflow' : 'New Workflow'}
        </Typography>
      </DialogTitle>
      <DialogContent dividers sx={{ py: 2 }}>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <Box>
            <Typography variant="caption" fontWeight="bold" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
              BASIC INFORMATION
            </Typography>
            <Stack spacing={2}>
              <TextField
                label="Workflow Name"
                fullWidth
                value={name}
                onChange={(e) => handleNameChange(e.target.value)}
                required
                autoFocus
                size="small"
                error={!!error}
                helperText={error || "Unique alphanumeric name, underscores allowed"}
              />
              <TextField
                label="Version"
                fullWidth
                value={version}
                onChange={(e) => setVersion(e.target.value)}
                size="small"
              />
              <TextField
                select
                label="Entry Node (Head)"
                fullWidth
                value={head}
                onChange={(e) => setHead(e.target.value)}
                size="small"
                SelectProps={{ native: true }}
                helperText="Select where the workflow execution starts"
              >
                {!nodes?.length && <option value="">No nodes added yet</option>}
                {nodes?.map((node) => (
                  <option key={node.id} value={node.id}>
                    {node.name || 'Untitled Node'}
                  </option>
                ))}
              </TextField>
            </Stack>
          </Box>
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={handleSave} variant="contained" disabled={!name.trim() || !!error}>
          {initialData?.id ? 'Update' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default WorkflowModal;
