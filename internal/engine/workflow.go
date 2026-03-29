package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

const (
	outputBufferSize = 1024
)

// Workflow orchestrates a graph of nodes, managing their lifecycle, connectivity,
// and the backpressure-aware egress of processed packets.
type Workflow struct {
	ctx      context.Context
	cancel   context.CancelFunc
	graph    *Graph
	nodes    []*node
	indexMap map[string]int        // node ID to nodes index
	nameMap  map[string]int        // node Name to nodes index
	output   chan *protocol.Packet // Common egress for all results/media from the workflow
	state    atomic.Int32
}

// NewWorkflow creates a new workflow runtime instance from a validated Graph.
func NewWorkflow(parentCtx context.Context, graph *Graph) (*Workflow, error) {
	ctx, cancel := context.WithCancel(parentCtx)

	w := &Workflow{
		ctx:    ctx,
		cancel: cancel,
		graph:  graph,
		output: make(chan *protocol.Packet, outputBufferSize),
	}
	w.state.Store(int32(WorkflowStatePending))

	if err := w.initNodes(); err != nil {
		cancel()
		return nil, err
	}

	if err := w.wireNodes(); err != nil {
		cancel()
		return nil, err
	}

	for _, n := range w.nodes {
		n.setSocketWriter(w)
	}

	return w, nil
}

func (w *Workflow) initNodes() error {
	w.nodes = make([]*node, len(w.graph.OrderedNodes))
	w.indexMap = make(map[string]int, len(w.graph.OrderedNodes))
	w.nameMap = make(map[string]int, len(w.graph.OrderedNodes))

	for i, nodeCfg := range w.graph.OrderedNodes {
		if _, ok := w.indexMap[nodeCfg.ID]; ok {
			return fmt.Errorf("duplicate node ID found: %s", nodeCfg.ID)
		}
		if _, ok := w.nameMap[nodeCfg.Name]; ok {
			return fmt.Errorf("duplicate node Name found: %s", nodeCfg.Name)
		}

		builder := LoadPluginBuilder(nodeCfg.Plugin)
		if builder == nil {
			return fmt.Errorf("plugin builder not found for type: %s", nodeCfg.Plugin)
		}

		ext, err := builder.Build(nodeCfg.Config)
		if err != nil {
			return fmt.Errorf("failed to build plugin %s (node %s): %w", nodeCfg.Plugin, nodeCfg.ID, err)
		}

		w.nodes[i] = newNode(w.ctx, nodeCfg.Name, ext)
		w.indexMap[nodeCfg.ID] = i
		w.nameMap[nodeCfg.Name] = i
	}
	return nil
}

func (w *Workflow) wireNodes() error {
	for _, edge := range w.graph.Config.Edges {
		fromIdx, ok := w.indexMap[edge.Source]
		if !ok {
			return fmt.Errorf("edge source node not found: %s", edge.Source)
		}
		toIdx, ok := w.indexMap[edge.Target]
		if !ok {
			return fmt.Errorf("edge target node not found: %s", edge.Target)
		}

		sourceNode := w.nodes[fromIdx]
		targetNode := w.nodes[toIdx]

		if edge.SourcePort == 0 {
			sourceNode.addNextNode(edge.Type, targetNode)
		} else {
			sourceNode.addNextPortNode(edge.Type, targetNode, edge.SourcePort)
		}
	}
	return nil
}

// Write captures packets emitted by plugins and routes them to the workflow output channel.
// It implements backpressure by dropping packets if the egress buffer is full.
func (w *Workflow) Write(packet *protocol.Packet) {
	defer func() {
		if r := recover(); r != nil {
			protocol.ReleasePacket(packet)
		}
	}()
	if w.state.Load() != int32(WorkflowStateRunning) || w.ctx.Err() != nil {
		protocol.ReleasePacket(packet)
		return
	}
	select {
	case w.output <- packet:
	case <-w.ctx.Done():
		protocol.ReleasePacket(packet)
	default:
		// Packet-drop strategy to prevent node blocking under heavy load
		slog.WarnContext(w.ctx, "Workflow output channel blocked, dropping packet", "type", packet.Type)
		protocol.ReleasePacket(packet)
	}
}

// Output returns the read-only channel for workflow egress packet.
func (w *Workflow) Output() <-chan *protocol.Packet {
	return w.output
}

// Start kicks off the read loops and start lifecycle hooks for all nodes.
func (w *Workflow) Start() error {
	if !w.state.CompareAndSwap(int32(WorkflowStatePending), int32(WorkflowStateStarting)) {
		return fmt.Errorf("workflow cannot be started (current state: %d)", w.state.Load())
	}

	startedIndex := -1
	for i, n := range w.nodes {
		beforeTime := time.Now().UnixMilli()
		if err := n.start(); err != nil {
			slog.ErrorContext(w.ctx, "workflow node start failed", "nodeName", n.name, "error", err)
			w.clearStartError(startedIndex)
			w.state.Store(int32(WorkflowStateStopped))
			return fmt.Errorf("failed to start node %s: %w", n.name, err)
		}
		startedIndex = i
		onStartTime := time.Now().UnixMilli() - beforeTime
		slog.InfoContext(w.ctx, "workflow node start success", "nodeName", n.name, "timeMs", onStartTime)
	}

	// Only mark as fully running if everything successfully initialized
	w.state.Store(int32(WorkflowStateRunning))

	for _, n := range w.nodes {
		n.ready()
	}
	return nil
}

func (w *Workflow) clearStartError(index int) {
	for i := index; i >= 0; i-- {
		n := w.nodes[i]
		slog.InfoContext(w.ctx, "workflow node rollback stop", "nodeName", n.name)
		n.stop()
	}
}

// Stop gracefully shuts down all nodes and the workflow context.
func (w *Workflow) Stop() {
	if w.state.Swap(int32(WorkflowStateStopped)) == int32(WorkflowStateStopped) {
		return
	}

	if w.cancel != nil {
		w.cancel()
	}

	for _, n := range w.nodes {
		n.stop()
	}

	close(w.output)
	for msg := range w.output {
		protocol.ReleasePacket(msg)
	}
}

// Pause sets workflow to paused state
func (w *Workflow) Pause() error {
	if !w.state.CompareAndSwap(int32(WorkflowStateRunning), int32(WorkflowStatePaused)) {
		return fmt.Errorf("workflow cannot be paused (current state: %d)", w.state.Load())
	}
	for _, nd := range w.nodes {
		nd.pause()
	}
	slog.InfoContext(w.ctx, "Workflow paused")
	return nil
}

// Resume sets workflow to active state
func (w *Workflow) Resume() error {
	if !w.state.CompareAndSwap(int32(WorkflowStatePaused), int32(WorkflowStateRunning)) {
		return fmt.Errorf("workflow cannot be resumed from current state")
	}
	for _, nd := range w.nodes {
		nd.resume()
	}
	slog.InfoContext(w.ctx, "Workflow resumed")
	return nil
}

// SendToHead routes an immutable schema object directly to the explicitly defined head node.
func (w *Workflow) SendToHead(item schema.ReadOnly) error {
	if err := w.ensureRunning(); err != nil {
		return err
	}

	headID := w.graph.Config.Head
	if headID == "" {
		return fmt.Errorf("this workflow has no strictly defined head node")
	}

	return w.deliverTo(headID, item)
}

// SendToNode routes an immutable schema object to a specific node by its ID.
func (w *Workflow) SendToNode(nodeID string, item schema.ReadOnly) error {
	if err := w.ensureRunning(); err != nil {
		return err
	}
	return w.deliverTo(nodeID, item)
}

// SendToNodeWithName routes an immutable schema object to a specific node by its name.
func (w *Workflow) SendToNodeWithName(name string, item schema.ReadOnly) error {
	if err := w.ensureRunning(); err != nil {
		return err
	}

	idx, ok := w.nameMap[name]
	if !ok {
		return fmt.Errorf("node Name %s not found in workflow", name)
	}

	w.nodes[idx].Input(item)
	return nil
}

func (w *Workflow) deliverTo(nodeID string, item schema.ReadOnly) error {
	idx, ok := w.indexMap[nodeID]
	if !ok {
		return fmt.Errorf("node ID %s not found in workflow", nodeID)
	}

	w.nodes[idx].Input(item)
	return nil
}

// Broadcast sends an immutable schema object to all nodes in the workflow.
func (w *Workflow) Broadcast(item schema.ReadOnly) error {
	if err := w.ensureRunning(); err != nil {
		return err
	}

	for _, n := range w.nodes {
		if rc, ok := item.(schema.RefCountable); ok {
			rc.Retain()
		}
		n.Input(item)
	}

	if rc, ok := item.(schema.RefCountable); ok {
		rc.Release()
	}
	return nil
}

func (w *Workflow) State() WorkflowState {
	return WorkflowState(w.state.Load())
}

func (w *Workflow) Context() context.Context {
	return w.ctx
}

func (w *Workflow) ensureRunning() error {
	if state := w.state.Load(); state != int32(WorkflowStateRunning) {
		return fmt.Errorf("workflow is not running (state: %d)", state)
	}
	return nil
}
