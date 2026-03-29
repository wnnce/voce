import React from 'react';
import {
  Drawer,
  Divider,
  Box,
  Tabs,
  Tab,
} from '@mui/material';
import type { WorkflowConfig } from '@/types/workflow';
import WorkflowList from './WorkflowList';
import PluginList from './PluginList';

interface SidebarProps {
  open: boolean;
  onClose?: () => void;
  // Events still bubbled up for Canvas focus
  onSelect?: (wf: WorkflowConfig) => void;
  onCreate?: (wf: Partial<WorkflowConfig>) => void;
  onDuplicate?: (wf: WorkflowConfig) => void;
}

const Sidebar: React.FC<SidebarProps> = ({ open, onClose, onSelect, onCreate, onDuplicate }) => {
  const [activeTab, setActiveTab] = React.useState<'workflows' | 'plugins'>('workflows');

  return (
    <Drawer
      variant="temporary"
      anchor="left"
      open={open}
      onClose={onClose}
      ModalProps={{
        hideBackdrop: true, 
        style: { pointerEvents: 'none' } 
      }}
      PaperProps={{
        style: { 
          pointerEvents: 'auto', 
          zIndex: 1400 
        }
      }}
      sx={{
        zIndex: (theme) => theme.zIndex.drawer + 5,
        '& .MuiDrawer-paper': {
          width: 280,
          boxSizing: 'border-box',
          bgcolor: 'background.paper',
          borderRight: '1px solid var(--border-color)',
          pt: '52px',
          boxShadow: 'none',
          borderRadius: 0,
          display: 'flex',
          flexDirection: 'column',
        },
      }}
    >
      <Box sx={{ p: 1, px: 2 }}>
        <Tabs 
          value={activeTab} 
          onChange={(_, v) => setActiveTab(v)}
          variant="fullWidth"
          sx={{
            minHeight: 36,
            height: 36,
            '& .MuiTabs-indicator': {
              height: '100%',
              borderRadius: '8px',
              zIndex: 0,
              opacity: 0.1,
            },
            '& .MuiTab-root': {
              minHeight: 36,
              height: 36,
              textTransform: 'none',
              fontWeight: 600,
              fontSize: '0.8rem',
              zIndex: 1,
              minWidth: 'unset',
            }
          }}
        >
          <Tab label="Workflows" value="workflows" />
          <Tab label="Plugins" value="plugins" />
        </Tabs>
      </Box>

      <Divider />

      <Box sx={{ flex: 1, overflowY: 'auto' }}>
        {activeTab === 'workflows' ? (
          <WorkflowList 
            onSelect={onSelect}
            onCreate={onCreate}
            onDuplicate={onDuplicate}
          />
        ) : (
          <PluginList />
        )}
      </Box>
    </Drawer>
  );
};

export default Sidebar;
