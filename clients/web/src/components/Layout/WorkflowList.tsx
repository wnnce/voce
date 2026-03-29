import React from 'react';
import {
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  IconButton,
  Tooltip,
  Box,
  Typography,
  Menu,
  MenuItem,
  Divider,
} from '@mui/material';
import { Plus, Workflow, Trash2, MoreVertical, Copy } from 'lucide-react';
import type { WorkflowConfig } from '@/types/workflow';
import { useWorkflowStore } from '@/store/useAppStore';

interface WorkflowListProps {
  onSelect?: (wf: WorkflowConfig) => void;
  onCreate?: (wf: Partial<WorkflowConfig>) => void;
  onDuplicate?: (wf: WorkflowConfig) => void;
}

const WorkflowList: React.FC<WorkflowListProps> = ({ 
  onSelect, 
  onCreate, 
  onDuplicate 
}) => {
  const { workflows, selectedWorkflow, deleteWorkflow } = useWorkflowStore();
  const [menuAnchor, setMenuAnchor] = React.useState<null | HTMLElement>(null);
  const [activeWf, setActiveWf] = React.useState<WorkflowConfig | null>(null);

  const handleMenuOpen = (wf: WorkflowConfig, e: React.MouseEvent<HTMLElement>) => {
    e.stopPropagation();
    setActiveWf(wf);
    setMenuAnchor(e.currentTarget);
  };

  const handleMenuClose = () => {
    setMenuAnchor(null);
    setActiveWf(null);
  };

  const handleDelete = async (id: string, e?: React.MouseEvent) => {
    e?.stopPropagation?.();
    if (confirm('Are you sure you want to delete this workflow?')) {
      await deleteWorkflow(id);
    }
  };

  return (
    <>
      <Box sx={{ p: 2, pb: 1, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="caption" color="text.secondary" fontWeight="bold" sx={{ letterSpacing: 0.5 }}>
          SAVED WORKFLOWS
        </Typography>
        <Tooltip title="New Workflow">
          <IconButton size="small" color="primary" onClick={() => onCreate?.({})}>
            <Plus size={16} />
          </IconButton>
        </Tooltip>
      </Box>
      <List>
        {workflows.map((wf) => (
          <ListItem
            key={wf.id}
            disablePadding
            secondaryAction={
              <IconButton 
                className="more-btn"
                edge="end" 
                size="small" 
                onClick={(e) => handleMenuOpen(wf, e)}
                sx={{ 
                  opacity: 0, 
                  transition: 'opacity 0.2s',
                  mr: 0.5,
                  zIndex: 2,
                  '&:hover': { opacity: 1 }
                }}
              >
                <MoreVertical size={14} />
              </IconButton>
            }
            sx={{
              '&:hover .MuiListItemButton-root': {
                bgcolor: selectedWorkflow?.id === wf.id ? 'action.selected' : 'action.hover',
              },
              '&:hover .more-btn': { opacity: 0.7 },
              mb: 0.2,
            }}
          >
            <ListItemButton 
              onClick={() => onSelect?.(wf)} 
              selected={selectedWorkflow?.id === wf.id}
              sx={{
                mx: 1,
                borderRadius: '8px',
                py: 0.7,
                transition: 'background-color 0.1s ease',
                '&.Mui-selected': {
                  bgcolor: 'action.selected',
                  color: 'text.primary',
                  '&:hover': { bgcolor: 'action.selected' },
                  '& .MuiListItemIcon-root': { color: 'primary.main' },
                  '& .MuiListItemText-secondary': { color: 'text.secondary' }
                },
                '&:hover': {
                  bgcolor: 'transparent',
                  '@media (hover: none)': { bgcolor: 'transparent' }
                }
              }}
            >
              <ListItemIcon sx={{ minWidth: 32, color: selectedWorkflow?.id === wf.id ? 'primary.main' : 'inherit' }}>
                <Workflow size={17} />
              </ListItemIcon>
              <ListItemText
                primary={wf.name}
                secondary={wf.version}
                primaryTypographyProps={{ 
                  variant: 'body2', 
                  fontWeight: 600,
                  letterSpacing: -0.1
                }}
                secondaryTypographyProps={{ 
                  variant: 'caption',
                  sx: { opacity: 0.6, fontSize: '0.65rem' }
                }}
              />
            </ListItemButton>
          </ListItem>
        ))}
        {workflows.length === 0 && (
          <Box sx={{ p: 4, textAlign: 'center' }}>
            <Typography variant="caption" color="text.secondary">
              No workflows yet
            </Typography>
          </Box>
        )}
      </List>

      <Menu
        anchorEl={menuAnchor}
        open={Boolean(menuAnchor)}
        onClose={handleMenuClose}
        PaperProps={{
          sx: { 
            boxShadow: '0 4px 12px rgba(0,0,0,0.1)',
            borderRadius: '8px',
            minWidth: 120
          }
        }}
      >
        <MenuItem 
          onClick={() => {
            if (activeWf) onDuplicate?.(activeWf);
            handleMenuClose();
          }}
          sx={{ gap: 1.5, fontSize: '0.85rem' }}
        >
          <Copy size={16} /> Duplicate
        </MenuItem>
        <Divider />
        <MenuItem 
          onClick={() => {
            if (activeWf) handleDelete(activeWf.id);
            handleMenuClose();
          }}
          sx={{ gap: 1.5, fontSize: '0.85rem', color: 'error.main' }}
        >
          <Trash2 size={16} /> Delete
        </MenuItem>
      </Menu>
    </>
  );
};

export default WorkflowList;
