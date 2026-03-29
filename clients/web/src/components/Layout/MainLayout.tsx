import React, { useState } from 'react';
import { Box, AppBar, Toolbar, Typography, IconButton, useTheme } from '@mui/material';
import { Menu as MenuIcon, Moon, Sun } from 'lucide-react';
import { useThemeControl } from '@/hooks/useThemeControl';
import Sidebar from '@/components/Layout/Sidebar';
import type { WorkflowConfig } from '@/types/workflow';

interface MainLayoutProps {
  children: React.ReactNode;
  onSelectWorkflow?: (wf: WorkflowConfig) => void;
  onCreateNew?: (wf: Partial<WorkflowConfig>) => void;
  onDuplicateWorkflow?: (wf: WorkflowConfig) => void;
}

const MainLayout: React.FC<MainLayoutProps> = ({ 
  children, 
  onSelectWorkflow, 
  onCreateNew,
  onDuplicateWorkflow
}) => {
  const [drawerOpen, setDrawerOpen] = useState(true);
  const { mode, toggleTheme } = useThemeControl();
  const theme = useTheme();

  return (
    <Box sx={{ width: '100vw', height: '100vh', position: 'relative', bgcolor: 'background.default', overflow: 'hidden' }}>
      <AppBar
        position="fixed"
        sx={{
          zIndex: 1600, // Explicitly higher than the floating Sidebar (1400)
          height: 52,
          justifyContent: 'center'
        }}
      >
        <Toolbar sx={{ justifyContent: 'space-between', minHeight: 'unset !important' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
            <IconButton onClick={() => setDrawerOpen(!drawerOpen)} edge="start" size="small">
              <MenuIcon size={18} />
            </IconButton>
            <Typography variant="body1" fontWeight="800" sx={{ letterSpacing: -0.5 }}>
              Voce Orchestrator
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <IconButton onClick={toggleTheme} size="small">
              {mode === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
            </IconButton>
          </Box>
        </Toolbar>
      </AppBar>

      <Box
        component="main"
        sx={{
          width: '100%',
          height: '100%',
          pt: '52px',
          overflow: 'hidden',
          position: 'absolute',
          top: 0,
          left: 0,
          zIndex: theme.zIndex.drawer,
        }}
      >
        {children}
      </Box>

      <Sidebar 
        open={drawerOpen} 
        onClose={() => setDrawerOpen(false)}
        onSelect={onSelectWorkflow} 
        onCreate={onCreateNew} 
        onDuplicate={onDuplicateWorkflow}
      />
    </Box>
  );
};

export default MainLayout;
