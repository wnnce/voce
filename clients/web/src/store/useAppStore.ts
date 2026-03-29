import { create } from 'zustand';
import type { WorkflowConfig, PluginInfo } from '@/types/workflow';
import { workflowApi, pluginApi } from '@/api/workflow';

interface WorkflowState {
  workflows: WorkflowConfig[];
  plugins: PluginInfo[];
  selectedWorkflow: WorkflowConfig | null;
  isLoading: boolean;
  
  // Actions
  fetchWorkflows: () => Promise<void>;
  fetchPlugins: () => Promise<void>;
  setSelectedWorkflow: (wf: WorkflowConfig | null) => void;
  deleteWorkflow: (id: string) => Promise<void>;
  duplicateWorkflow: (wf: WorkflowConfig) => Promise<void>;
}

export const useWorkflowStore = create<WorkflowState>((set, get) => ({
  workflows: [],
  plugins: [],
  selectedWorkflow: null,
  isLoading: false,

  fetchWorkflows: async () => {
    try {
      const res = await workflowApi.list();
      set({ workflows: res.data });
    } catch (e) {
      console.error('Failed to load workflows', e);
    }
  },

  fetchPlugins: async () => {
    try {
      const res = await pluginApi.list();
      set({ plugins: res.data });
    } catch (e) {
      console.error('Failed to load plugins', e);
    }
  },

  setSelectedWorkflow: (wf) => set({ selectedWorkflow: wf }),

  deleteWorkflow: async (id) => {
    try {
      await workflowApi.delete(id);
      await get().fetchWorkflows();
      if (get().selectedWorkflow?.id === id) {
        set({ selectedWorkflow: null });
      }
    } catch (e) {
      console.error('Failed to delete workflow', e);
    }
  },

  duplicateWorkflow: async (wf) => {
    try {
      const res = await workflowApi.get(wf.id);
      const fullWf = res.data;
      const clone: WorkflowConfig = {
        ...fullWf,
        id: '', // Treated as NEW
        name: `${fullWf.name}_copy`,
      };
      set({ selectedWorkflow: clone });
    } catch (e) {
      console.error('Duplicate failed', e);
    }
  },
}));

// Notification Store
export type MessageSeverity = 'error' | 'success' | 'warning' | 'info';

interface MessageState {
  message: string;
  severity: MessageSeverity;
  open: boolean;
  
  // Actions
  show: (message: string, severity?: MessageSeverity) => void;
  hide: () => void;
  showError: (message: string) => void;
  showSuccess: (message: string) => void;
}

export const useMessageStore = create<MessageState>((set) => ({
  message: '',
  severity: 'info',
  open: false,

  show: (message, severity = 'info') => set({ message, severity, open: true }),
  hide: () => set({ open: false }),
  showError: (message) => set({ message, severity: 'error', open: true }),
  showSuccess: (message) => set({ message, severity: 'success', open: true }),
}));
