package engine

import (
	"fmt"
	"log/slog"
)

// Graph represents the validated static topology of a workflow.
type Graph struct {
	Config       *WorkflowConfig
	OrderedNodes []NodeConfig // Topologically sorted nodes ensuring dependencies are started first
}

// BuildGraph performs a multi-stage validation (loops, contracts, existence)
// and produces a flatten, ordered representation of the workflow.
//
// This function orchestrates the graph construction process:
//  1. Validates the existence of the head node.
//  2. Parses and validates individual nodes, ensuring unique IDs and plugin availability.
//  3. Checks for self-loops in edges.
//  4. Performs contract validation between connected nodes, ensuring output types match input expectations.
//  5. Topologically sorts the nodes to determine a safe execution order,
//     where all dependencies of a node are processed before the node itself.
func BuildGraph(config *WorkflowConfig) (*Graph, error) {
	if config.Head == "" {
		return nil, fmt.Errorf("a 'head' node must be explicitly defined in workflow config")
	}

	nodesMap, err := parseNodes(config.Nodes)
	if err != nil {
		return nil, err
	}

	if _, ok := nodesMap[config.Head]; !ok {
		return nil, fmt.Errorf("head node ID %s not found in workflow configuration", config.Head)
	}

	if loopErr := checkSelfLoops(config.Edges); loopErr != nil {
		return nil, loopErr
	}

	// Validate that outputs of source nodes match inputs of target nodes based on edge type.
	// This ensures data compatibility and prevents runtime errors due to mismatched interfaces.
	if contractErr := validateEdgeContracts(nodesMap, config.Edges); contractErr != nil {
		return nil, contractErr
	}

	// Topologically sort nodes to ensure that nodes are processed in an order
	// where all their upstream dependencies have been initialized.
	// This is crucial for correct workflow execution, especially for stateful nodes.
	orderedNodes := sortNodesTopologically(config, nodesMap)

	return &Graph{
		Config:       config,
		OrderedNodes: orderedNodes,
	}, nil
}

func parseNodes(nodes []NodeConfig) (map[string]NodeConfig, error) {
	nodesMap := make(map[string]NodeConfig, len(nodes))
	for _, n := range nodes {
		if _, exists := nodesMap[n.ID]; exists {
			return nil, fmt.Errorf("duplicate node ID found: %s", n.ID)
		}
		if builder := LoadPluginBuilder(n.Plugin); builder == nil {
			return nil, fmt.Errorf("plugin builder not found for type: %s", n.Plugin)
		}
		nodesMap[n.ID] = n
	}
	return nodesMap, nil
}

func checkSelfLoops(edges []EdgeConfig) error {
	for _, edge := range edges {
		if edge.Source == edge.Target {
			return fmt.Errorf("node %s has a self-loop, which is not allowed", edge.Source)
		}
	}
	return nil
}

func validateEdgeContracts(nodesMap map[string]NodeConfig, edges []EdgeConfig) error {
	for _, edge := range edges {
		srcCfg, srcExists := nodesMap[edge.Source]
		tgtCfg, tgtExists := nodesMap[edge.Target]

		if !srcExists {
			return fmt.Errorf("edge source node not found: %s", edge.Source)
		}
		if !tgtExists {
			return fmt.Errorf("edge target node not found: %s", edge.Target)
		}

		srcBuilder := LoadPluginBuilder(srcCfg.Plugin)
		tgtBuilder := LoadPluginBuilder(tgtCfg.Plugin)

		var prefix string
		switch edge.Type {
		case EventPayload:
			prefix = PrefixPayload
		case EventSignal:
			prefix = PrefixSignal
		case EventAudio:
			prefix = PrefixAudio
		default:
			return fmt.Errorf("unknown edge type: %v", edge.Type)
		}
		if err := ValidateProperties(srcBuilder.Outputs(), tgtBuilder.Inputs(), prefix); err != nil {
			return fmt.Errorf("contract validation failed on edge %s -> %s (%s): %w", edge.Source, edge.Target, prefix, err)
		}
	}
	return nil
}

func sortNodesTopologically(config *WorkflowConfig, nodesMap map[string]NodeConfig) []NodeConfig {
	adj := make(map[string][]string)
	inDegree := make(map[string]int, len(nodesMap))

	for id := range nodesMap {
		inDegree[id] = 0
	}

	for _, edge := range config.Edges {
		adj[edge.Source] = append(adj[edge.Source], edge.Target)
		inDegree[edge.Target]++
	}

	queue := make([]string, 0)
	for id, count := range inDegree {
		if count == 0 {
			queue = append(queue, id)
		}
	}

	// orderedIDs contains nodes in an order that respects edge directions.
	// This ensures that upstream nodes are initialized before downstream ones.
	orderedIDs := make([]string, 0, len(nodesMap))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		orderedIDs = append(orderedIDs, curr)

		for _, next := range adj[curr] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	// Append cyclic remnants
	if len(orderedIDs) != len(config.Nodes) {
		slog.Warn("Graph contains a feedback cycle, some nodes might not start in perfect order.")
		for id, count := range inDegree {
			if count > 0 {
				orderedIDs = append(orderedIDs, id)
			}
		}
	}

	orderedNodes := make([]NodeConfig, len(orderedIDs))
	for i, id := range orderedIDs {
		orderedNodes[i] = nodesMap[id]
	}

	return orderedNodes
}
