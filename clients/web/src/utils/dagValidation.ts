import type { Node, Edge } from 'reactflow';
import type { PluginInfo, WorkflowConfig } from '@/types/workflow';

export interface ValidationIssue {
  type: 'error' | 'warning';
  nodeId?: string;
  message: string;
}

export interface DAGValidationResult {
  valid: boolean;
  errors: ValidationIssue[];
  warnings: ValidationIssue[];
}

export function validateDAG(
  workflow: Partial<WorkflowConfig> | null,
  nodes: Node[],
  edges: Edge[]
): DAGValidationResult {
  const errors: ValidationIssue[] = [];
  const warnings: ValidationIssue[] = [];

  if (!workflow) {
    return { valid: true, errors: [], warnings: [] };
  }

  // 1. Check head node
  if (nodes.length > 0) {
    if (!workflow.head) {
      warnings.push({
        type: 'warning',
        message: 'No entry head node designated for this workflow.',
      });
    } else {
      const headExists = nodes.some((n) => n.id === workflow.head);
      if (!headExists) {
        errors.push({
          type: 'error',
          message: `Designated head node '${workflow.head}' does not exist in graph.`,
        });
      }
    }
  }

  // 2. Check for isolated nodes (no incoming and no outgoing edges)
  if (nodes.length > 1) {
    for (const node of nodes) {
      const hasOutgoing = edges.some((e) => e.source === node.id);
      const hasIncoming = edges.some((e) => e.target === node.id);

      if (!hasOutgoing && !hasIncoming && node.id !== workflow.head) {
        warnings.push({
          type: 'warning',
          nodeId: node.id,
          message: `Node '${node.data?.name || node.id}' is isolated with no connections.`,
        });
      }
    }
  }

  // 3. Check for missing required inputs
  for (const node of nodes) {
    const pluginInfo = node.data?.pluginInfo as PluginInfo | undefined;
    if (!pluginInfo || !pluginInfo.inputs) continue;

    for (const input of pluginInfo.inputs) {
      const hasRequiredFields = input.fields?.some((f) => f.required);
      if (hasRequiredFields) {
        // Find if any edge lands on this input prefix
        const connected = edges.some((e) => {
          if (e.target !== node.id) return false;
          // targetHandle format: target-<type>
          return true;
        });

        if (!connected) {
          warnings.push({
            type: 'warning',
            nodeId: node.id,
            message: `Node '${node.data?.name}' requires input [${input.prefix}:${input.name || '*'}] but has no incoming connection.`,
          });
        }
      }
    }
  }

  return {
    valid: errors.length === 0,
    errors,
    warnings,
  };
}
