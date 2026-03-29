import { useRef, useEffect } from 'react';
import { CustomThemeProvider } from '@/theme/ThemeContext';
import { MessageProvider } from '@/context/MessageContext';
import MainLayout from '@/components/Layout/MainLayout';
import WorkflowPage, { type WorkflowPageHandle } from '@/pages/Workflow/WorkflowPage';
import type { WorkflowConfig } from '@/types/workflow';
import { useWorkflowStore } from '@/store/useAppStore';

function App() {
  const workflowPageRef = useRef<WorkflowPageHandle>(null);
  const { 
    setSelectedWorkflow, 
    fetchWorkflows, 
    fetchPlugins, 
    duplicateWorkflow 
  } = useWorkflowStore();

  useEffect(() => {
    fetchWorkflows();
    fetchPlugins();
  }, [fetchWorkflows, fetchPlugins]);

  // Handle cross-component selections
  const handleSelectWorkflow = (wf: WorkflowConfig) => {
    setSelectedWorkflow(wf);
    workflowPageRef.current?.onSelectWorkflow(wf);
  };

  const handleCreateNew = (wf: Partial<WorkflowConfig>) => {
    const tempWf = { ...wf, id: 'new', nodes: [], edges: [] } as WorkflowConfig;
    setSelectedWorkflow(tempWf);
    workflowPageRef.current?.onCreateNew(wf);
  };

  const handleDuplicate = async (wf: WorkflowConfig) => {
    await duplicateWorkflow(wf);
    // After store sets selectedWorkflow, we trigger the canvas
    const newWf = useWorkflowStore.getState().selectedWorkflow;
    if (newWf) {
      workflowPageRef.current?.onSelectWorkflow(newWf);
    }
  };

  return (
    <CustomThemeProvider>
      <MessageProvider>
        <MainLayout 
          onSelectWorkflow={handleSelectWorkflow} 
          onCreateNew={handleCreateNew}
          onDuplicateWorkflow={handleDuplicate}
        >
          <WorkflowPage ref={workflowPageRef} />
        </MainLayout>
      </MessageProvider>
    </CustomThemeProvider>
  );
}

export default App;
