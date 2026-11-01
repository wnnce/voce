import type { Node, Edge } from 'reactflow';

export interface LayoutOptions {
  nodeWidth?: number;
  nodeHeight?: number;
  horizontalSpacing?: number;
  verticalSpacing?: number;
  startX?: number;
  startY?: number;
}

/**
 * Clean topological auto-layout for DAG nodes from left to right.
 * Computes node depth levels from in-degree and edges, then arranges nodes in columns.
 */
export function autoLayoutDAG<T = unknown>(
  nodes: Node<T>[],
  edges: Edge[],
  options: LayoutOptions = {}
): Node<T>[] {
  if (nodes.length === 0) return [];

  const {
    horizontalSpacing = 340,
    verticalSpacing = 160,
    startX = 80,
    startY = 80,
  } = options;

  const nodeMap = new Map<string, Node<T>>();
  const inDegree = new Map<string, number>();
  const adj = new Map<string, string[]>();

  for (const node of nodes) {
    nodeMap.set(node.id, node);
    inDegree.set(node.id, 0);
    adj.set(node.id, []);
  }

  for (const edge of edges) {
    if (nodeMap.has(edge.source) && nodeMap.has(edge.target)) {
      adj.get(edge.source)!.push(edge.target);
      inDegree.set(edge.target, (inDegree.get(edge.target) || 0) + 1);
    }
  }

  // Calculate topological levels
  const levelMap = new Map<string, number>();
  const queue: string[] = [];

  // Start with nodes that have in-degree 0 (or head node)
  for (const [id, deg] of inDegree.entries()) {
    if (deg === 0) {
      queue.push(id);
      levelMap.set(id, 0);
    }
  }

  // If cycle or no in-degree 0, fallback to first node
  if (queue.length === 0 && nodes.length > 0) {
    queue.push(nodes[0].id);
    levelMap.set(nodes[0].id, 0);
  }

  while (queue.length > 0) {
    const current = queue.shift()!;
    const currentLevel = levelMap.get(current) || 0;
    const neighbors = adj.get(current) || [];

    for (const neighbor of neighbors) {
      const existingLevel = levelMap.get(neighbor);
      const nextLevel = Math.max(existingLevel ?? 0, currentLevel + 1);
      levelMap.set(neighbor, nextLevel);

      const deg = (inDegree.get(neighbor) || 1) - 1;
      inDegree.set(neighbor, deg);
      if (deg === 0 || !existingLevel) {
        queue.push(neighbor);
      }
    }
  }

  // Assign any remaining unvisited nodes to level 0
  for (const node of nodes) {
    if (!levelMap.has(node.id)) {
      levelMap.set(node.id, 0);
    }
  }

  // Group nodes by level
  const levels = new Map<number, Node<T>[]>();
  for (const node of nodes) {
    const lvl = levelMap.get(node.id) || 0;
    if (!levels.has(lvl)) {
      levels.set(lvl, []);
    }
    levels.get(lvl)!.push(node);
  }

  // Calculate coordinates
  return nodes.map((node) => {
    const lvl = levelMap.get(node.id) || 0;
    const levelNodes = levels.get(lvl) || [node];
    const indexInLevel = levelNodes.findIndex((n) => n.id === node.id);

    return {
      ...node,
      position: {
        x: startX + lvl * horizontalSpacing,
        y: startY + indexInLevel * verticalSpacing,
      },
    };
  });
}
