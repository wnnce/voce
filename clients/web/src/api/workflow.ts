import client from '@/api/request';
import type { Result, WorkflowConfig, PluginInfo } from '@/types/workflow';

export const workflowApi = {
  list: () => client.get<Result<WorkflowConfig[]>>('/workflows'),
  get: (id: string) => client.get<Result<WorkflowConfig>>(`/workflows/${id}`),
  save: (config: WorkflowConfig | Partial<WorkflowConfig>) => client.post<Result<WorkflowConfig>>('/workflows', config),
  delete: (id: string) => client.delete<Result<void>>(`/workflows/${id}`),
};

export const pluginApi = {
  list: () => client.get<Result<PluginInfo[]>>('/plugins'),
};
