package inference

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"slices"

	"github.com/productscience/inference/x/inference/types"
	"github.com/shopspring/decimal"
)

const (
	FlowContext    = "model_assignment"
	SubFlowContext = "allocate_mlnodes_for_poc"
)

func sortedKeys[K ~string, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// EpochMLNodeData stores ML node information indexed by [modelId][participantAddress]
type EpochMLNodeData struct {
	data               map[string]map[string][]*types.MLNodeInfo
	partcipantsWeights map[string]int64
}

func NewEpochMLNodeData() *EpochMLNodeData {
	return &EpochMLNodeData{
		data:               make(map[string]map[string][]*types.MLNodeInfo),
		partcipantsWeights: make(map[string]int64),
	}
}

func (e *EpochMLNodeData) Set(modelId, participantAddr string, nodes []*types.MLNodeInfo) {
	if e.data[modelId] == nil {
		e.data[modelId] = make(map[string][]*types.MLNodeInfo)
	}
	e.data[modelId][participantAddr] = nodes
	for _, node := range nodes {
		e.partcipantsWeights[participantAddr] += node.PocWeight
	}
}

func (e *EpochMLNodeData) Append(modelId, participantAddr string, node *types.MLNodeInfo) {
	if e.data[modelId] == nil {
		e.data[modelId] = make(map[string][]*types.MLNodeInfo)
	}
	e.data[modelId][participantAddr] = append(e.data[modelId][participantAddr], node)
	e.partcipantsWeights[participantAddr] += node.PocWeight
}

func (e *EpochMLNodeData) GetForModel(modelId string) map[string][]*types.MLNodeInfo {
	return e.data[modelId]
}

func (e *EpochMLNodeData) GetForParticipant(modelId, participantAddr string) []*types.MLNodeInfo {
	if e.data[modelId] == nil {
		return nil
	}
	return e.data[modelId][participantAddr]
}

func (e *EpochMLNodeData) Models() []string {
	return sortedKeys(e.data)
}

func (e *EpochMLNodeData) GetAllNodesWeight() []int64 {
	weights := make([]int64, 0)
	for _, modelData := range e.data {
		for _, nodes := range modelData {
			for _, node := range nodes {
				weights = append(weights, node.PocWeight)
			}
		}
	}
	return weights
}

func (e *EpochMLNodeData) GetAllNodesWeightForParticipant() []int64 {
	weights := make([]int64, 0)
	for _, weight := range e.partcipantsWeights {
		weights = append(weights, weight)
	}
	return weights
}

func (e *EpochMLNodeData) GetAllParticipantsHash() string {
	uniqueParticipants := make(map[string]bool)
	for _, modelData := range e.data {
		for participantAddr := range modelData {
			uniqueParticipants[participantAddr] = true
		}
	}

	sortedParticipants := sortedKeys(uniqueParticipants)

	allParticipantsStr := fmt.Sprintf("%v", sortedParticipants)
	allParticipantsHash := sha256.Sum256([]byte(allParticipantsStr))
	return fmt.Sprintf("%x", allParticipantsHash[:8])
}

func (e *EpochMLNodeData) GetTotalWeightForModel(modelId string) int64 {
	var total int64
	participantNodes := e.GetForModel(modelId)
	for _, nodes := range participantNodes {
		for _, node := range nodes {
			total += node.PocWeight
		}
	}
	return total
}

func (e *EpochMLNodeData) GetParticipantWeight(participantAddr string) int64 {
	if _, ok := e.partcipantsWeights[participantAddr]; !ok {
		return 0
	}
	return e.partcipantsWeights[participantAddr]
}

type ModelAssigner struct {
	types.InferenceLogger
	keeper KeeperForModelAssigner
}

func NewModelAssigner(keeper KeeperForModelAssigner, logger types.InferenceLogger) *ModelAssigner {
	return &ModelAssigner{
		keeper:          keeper,
		InferenceLogger: logger,
	}
}

type KeeperForModelAssigner interface {
	GetGovernanceModelsSorted(ctx context.Context) ([]*types.Model, error)
	GetHardwareNodes(ctx context.Context, participantId string) (*types.HardwareNodes, bool)
	GetActiveParticipants(ctx context.Context, epochId uint64) (val types.ActiveParticipants, found bool)
	GetEpochGroupData(ctx context.Context, epochIndex uint64, modelId string) (val types.EpochGroupData, found bool)
	GetParams(ctx context.Context) types.Params
}

func (ma *ModelAssigner) setModelsForParticipants(ctx context.Context, participants []*types.ActiveParticipant, upcomingEpoch types.Epoch) {
	// TODO: We may need to populate throughput in MLNodeInfo using the model's ThroughputPerNonce
	// This would ensure consistent throughput calculations based on governance model parameters
	// rather than relying on hardware node declarations alone.
	ma.LogInfo("Starting model and slot assignment for participants", types.EpochGroup, "flow_context", FlowContext, "step", "start", "num_participants", len(participants), "epoch_index", upcomingEpoch.Index)

	// Get governance models to iterate through
	governanceModels, err := ma.keeper.GetGovernanceModelsSorted(ctx)
	if err != nil {
		ma.LogError("setModelsForParticipants: Unable to get governance models", types.EpochGroup, "error", err.Error(), "flow_context", FlowContext)
		return
	}
	ma.LogInfo("Retrieved governance models", types.EpochGroup, "flow_context", FlowContext, "step", "get_governance_models", "num_models", len(governanceModels))

	for _, p := range participants {
		ma.LogInfo("Processing participant", types.EpochGroup, "flow_context", FlowContext, "step", "participant_loop_start", "participant_index", p.Index)
		hardwareNodes, found := ma.keeper.GetHardwareNodes(ctx, p.Index)
		if !found {
			ma.LogInfo("No hardware nodes found for participant, skipping model assignment.", types.EpochGroup, "flow_context", FlowContext, "step", "no_hardware_nodes", "participant_index", p.Index)
			p.Models = make([]string, 0)
			p.MlNodes = make([]*types.ModelMLNodes, 0)
			continue
		}

		var originalMLNodes []*types.MLNodeInfo
		if len(p.MlNodes) > 0 && p.MlNodes[0] != nil {
			originalMLNodes = p.MlNodes[0].MlNodes
		}
		ma.LogInfo("Original MLNodes", types.EpochGroup, "flow_context", FlowContext, "step", "pre_legacy_distribution", "participant_index", p.Index, "ml_nodes", originalMLNodes)

		// Set PRE_POC_SLOT to true and POC_SLOT to false for all MLNodes (default to mining PoC)
		for _, mlNode := range originalMLNodes {
			// Initialize timeslot allocation vector: [PRE_POC_SLOT=true, POC_SLOT=false]
			mlNode.TimeslotAllocation = []bool{true, false} // index 0=PRE_POC_SLOT, index 1=POC_SLOT
		}
		ma.LogInfo("Initialized all ML nodes to PRE_POC_SLOT=true, POC_SLOT=false", types.EpochGroup, "flow_context", FlowContext, "step", "init_slots", "participant_index", p.Index)

		assignedMLNodes := make(map[string]bool)
		var supportedModels []string
		var newMLNodeArrays []*types.ModelMLNodes

		supportedModelsByNode := supportedModelsByNode(hardwareNodes, governanceModels)
		for nodeId, supportedModels := range supportedModelsByNode {
			ma.LogInfo("Supported models by node", types.EpochGroup, "flow_context", FlowContext, "step", "supported_models_by_node", "node_id", nodeId, "supported_models", supportedModels)
		}

		// For each governance model, pick the available MLNodes that have the model as first supported model
		for _, model := range governanceModels {
			ma.LogInfo("Attempting to assign ML node for model", types.EpochGroup, "flow_context", FlowContext, "step", "model_assignment_loop", "participant_index", p.Index, "model_id", model.Id)
			var modelMLNodes []*types.MLNodeInfo

			for _, mlNode := range originalMLNodes {
				if assignedMLNodes[mlNode.NodeId] {
					ma.LogInfo("Skipping already assigned ML node", types.EpochGroup, "flow_context", FlowContext, "step", "node_already_assigned", "participant_index", p.Index, "model_id", model.Id, "node_id", mlNode.NodeId)
					continue
				}

				// Check if this MLNode supports the current governance model
				if slices.Contains(supportedModelsByNode[mlNode.NodeId], model.Id) {
					ma.LogInfo("Found supporting and unassigned ML node for model", types.EpochGroup, "flow_context", FlowContext, "step", "assign_node_to_model", "participant_index", p.Index, "model_id", model.Id, "node_id", mlNode.NodeId)
					// Add this MLNode to the current model's array
					modelMLNodes = append(modelMLNodes, mlNode)
					assignedMLNodes[mlNode.NodeId] = true
				}
			}

			// Only add the model and MLNode array if we found supporting MLNodes
			if len(modelMLNodes) > 0 {
				supportedModels = append(supportedModels, model.Id)
				newMLNodeArrays = append(newMLNodeArrays, &types.ModelMLNodes{MlNodes: modelMLNodes})
				ma.LogInfo("Assigned ML nodes to model", types.EpochGroup, "flow_context", FlowContext, "step", "model_assignment_complete", "participant_index", p.Index, "model_id", model.Id, "assigned_nodes", modelMLNodes)
			} else {
				ma.LogInfo("No available ML nodes support this model", types.EpochGroup, "flow_context", FlowContext, "step", "no_supporting_nodes", "participant_index", p.Index, "model_id", model.Id)
			}
		}

		var unassignedMLNodes []*types.MLNodeInfo
		for _, mlNode := range originalMLNodes {
			if !assignedMLNodes[mlNode.NodeId] {
				unassignedMLNodes = append(unassignedMLNodes, mlNode)
			}
		}
		ma.LogInfo("Unassigned MLNodes", types.EpochGroup, "flow_context", FlowContext, "step", "unassigned_nodes", "participant_index", p.Index, "unassigned_nodes", unassignedMLNodes)

		p.MlNodes = newMLNodeArrays
		p.Models = supportedModels
		p.Weight = RecalculateWeight(p)
		ma.LogInfo("Participant models and ML nodes updated", types.EpochGroup, "flow_context", FlowContext, "step", "participant_updated", "participant_index", p.Index, "supported_models", p.Models, "ml_nodes", p.MlNodes)
	}
	ma.LogInfo("Finished model assignment for all participants", types.EpochGroup, "flow_context", FlowContext, "step", "model_assignment_complete")

	// Allocate ML nodes for PoC after all participants have their models assigned
	ma.allocateMLNodesForPoC(ctx, upcomingEpoch, participants)
	ma.LogInfo("Finished PoC allocation for all participants", types.EpochGroup, "flow_context", FlowContext, "step", "end")
}

func (ma *ModelAssigner) allocateMLNodesForPoC(ctx context.Context, upcomingEpoch types.Epoch, participants []*types.ActiveParticipant) {
	ma.LogInfo("Starting ML node allocation for PoC slots", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "start", "num_participants", len(participants))

	allocationFraction := ma.keeper.GetParams(ctx).EpochParams.PocSlotAllocation
	if allocationFraction == nil || allocationFraction.ToDecimal().IsZero() {
		ma.LogInfo("PocSlotAllocation is nil or 0, using default 0.5", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "default_allocation")
		allocationFraction = &types.Decimal{Value: 5, Exponent: -1}
	}

	// Phase 1: Build previous epoch data map
	previousEpochData := NewEpochMLNodeData()

	uniqueModels := make(map[string]bool)
	for _, participant := range participants {
		for _, modelId := range participant.Models {
			uniqueModels[modelId] = true
		}
	}
	ma.LogDebug("Collected unique models", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "collect_unique_models", "num_unique_models", len(uniqueModels))

	sortedModelIds := sortedKeys(uniqueModels)
	if upcomingEpoch.Index > 0 {
		for _, modelId := range sortedModelIds {
			previousEpochGroupData, found := ma.keeper.GetEpochGroupData(ctx, upcomingEpoch.Index-1, modelId)
			if found {
				for _, vw := range previousEpochGroupData.ValidationWeights {
					// Extract and store MlNodes directly
					previousEpochData.Set(modelId, vw.MemberAddress, vw.MlNodes)
				}
				ma.LogInfo("Loaded previous epoch data for model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "load_prev_epoch_data", "model_id", modelId, "num_validation_weights", len(previousEpochGroupData.ValidationWeights))
			}
		}
	}

	totalCurrentEpochWeight := int64(0)
	currentEpochData := NewEpochMLNodeData()
	for _, participant := range participants {
		for modelIdx, modelId := range participant.Models {
			if modelIdx >= len(participant.MlNodes) {
				ma.LogWarn("Model index out of bounds, skipping", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "model_index_oob", "participant_index", participant.Index, "model_id", modelId, "model_idx", modelIdx)
				continue
			}
			currentEpochData.Set(modelId, participant.Index, participant.MlNodes[modelIdx].MlNodes)
		}
		totalCurrentEpochWeight += participant.Weight
	}
	ma.LogInfo("Built current epoch data map", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "build_current_epoch_data", "num_models", len(currentEpochData.Models()))

	// Phase 3: Filter eligible nodes for all models
	eligibleNodesData := ma.filterEligibleMLNodes(upcomingEpoch, previousEpochData, currentEpochData, totalCurrentEpochWeight)
	ma.LogInfo("Filtered eligible nodes for all models", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "filter_all_eligible", "num_models", len(eligibleNodesData.Models()))

	// Phase 4: Allocate ML nodes for PoC for each model
	for _, modelId := range sortedModelIds {
		ma.LogInfo("Processing model for PoC allocation", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "model_loop_start", "model_id", modelId)
		ma.allocateMLNodePerPoCForModel(modelId, currentEpochData, eligibleNodesData, allocationFraction)
	}
}

// filterEligibleMLNodes filters which nodes are eligible for POC_SLOT=true allocation across all models.
//
// PURPOSE:
// Determines which ML nodes can be allocated POC_SLOT=true (serve inference during PoC phase).
// Uses multi-phase filtering to ensure sufficient PoC validation participation while filtering outliers.
//
// THREE FILTERING PHASES:
//
// Phase 1 - Top Participant Participation (70% + 25% rule):
//
//	Ensures participants with top 70% of weight have at least 25% of their nodes participating.
//	Calculates per-participant minimum node weight thresholds to include their top 25% nodes.
//
// Phase 2 - Outlier Node Filtering (IQR method):
//
//	Filters out suspiciously large nodes using statistical outlier detection (Q3 + 1.5*IQR).
//	Prevents single large nodes from dominating the eligible set.
//
// Phase 3 - Voting Constraint Check (<30% non-voting):
//
//	Ensures at least 70% of total weight can vote in PoC validation.
//	Tracks participants that become "non-voting" and limits them to <30% of total weight.
//
// KEY CONCEPTS:
//   - Eligible node: Can have POC_SLOT=true (serve inference during PoC phase)
//   - Voting participant: Has some nodes with POC_SLOT=false (can participate in PoC validation)
//   - Non-voting participant: All nodes have POC_SLOT=true (cannot participate in PoC validation)
//
// SAMPLING:
//
//	Selects N/2+1 participants with previous epoch history deterministically per model to rotate eligibility.
func (ma *ModelAssigner) filterEligibleMLNodes(
	upcomingEpoch types.Epoch,
	previousEpochData *EpochMLNodeData,
	currentEpochData *EpochMLNodeData,
	totalCurrentEpochWeight int64,
) *EpochMLNodeData {
	allParticipantsHashStr := currentEpochData.GetAllParticipantsHash()

	// Step 1: Ensure top participants have sufficient node participation (70% + 25% rule)
	// To guarantee validation during PoC we want > 70% of voting power to vote during PoC
	// We consider a participant to vote during PoC if at least 25% of its MLNode's weight are voting
	// Uncapped weight is used to calculate the threshold
	allParticipantsWeights := currentEpochData.GetAllNodesWeightForParticipant()
	participantWeightThreshold := calculateParticipantWeightThreshold70Percent(allParticipantsWeights)
	ma.LogInfo("Calculated participant weight threshold (70% rule)", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "calculate_participant_threshold", "threshold", participantWeightThreshold, "total_participants", len(allParticipantsWeights))

	// Calculate minimum node weight thresholds for top participants (25% rule)
	participantMinNodeWeightThresholds := calculatePerParticipantThreshold(currentEpochData, participantWeightThreshold)
	ma.LogInfo("Calculated per-participant node thresholds (25% rule)", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "calculate_per_participant_thresholds", "total_participants", len(participantMinNodeWeightThresholds))

	// Step 2: Filter suspiciously big nodes using IQR method
	// To filter suspiciously big nodes and set POC_SLOT=false for them
	allNodesWeights := currentEpochData.GetAllNodesWeight()
	globalMaxNodeWeightThreshold := calculateNodeWeightThresholdIQR(allNodesWeights)
	ma.LogInfo("Calculated node weight threshold (IQR method)", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "calculate_node_threshold", "threshold", globalMaxNodeWeightThreshold, "total_nodes", len(allNodesWeights))

	// Step 3: Track non-voting weight to ensure at least 70% can vote
	// Maximum allowed weight for participants that become non-voting (all nodes eligible)
	maxAllowedNonVotingWeight := decimal.NewFromInt(30).Div(decimal.NewFromInt(100)).Mul(decimal.NewFromInt(totalCurrentEpochWeight)).IntPart()
	totalNonVotingWeight := int64(0)

	eligibleNodesData := NewEpochMLNodeData()
	for _, modelId := range currentEpochData.Models() {
		participantNodes := currentEpochData.GetForModel(modelId)
		sortedParticipantAddrs := sortedKeys(participantNodes)

		// Sample N/2+1 participants with previous epoch history for rotation
		// This ensures we rotate which participants are eligible for PoC allocation each epoch
		// while maintaining deterministic selection based on epoch index and participant hash
		eligibleParticipantsPerModel := ma.sampleEligibleParticipantsWithHistory(
			sortedParticipantAddrs,
			previousEpochData,
			modelId,
			upcomingEpoch,
			allParticipantsHashStr,
		)

		// Process eligible participants
		for _, participantAddr := range eligibleParticipantsPerModel {
			currentNodes := participantNodes[participantAddr]

			// Calculate maximum allowed node weight using both per-participant and global thresholds
			// Combines Phase 1 (participant 25% rule) and Phase 2 (global IQR outlier detection)
			// Uses the more restrictive threshold to filter out large nodes
			maxAllowedNodeWeight := calculateEffectiveNodeThreshold(
				participantMinNodeWeightThresholds[participantAddr],
				globalMaxNodeWeightThreshold,
			)
			filteredNodes := filterNodesByWeight(currentNodes, maxAllowedNodeWeight)

			// Add nodes to eligible set one by one, checking voting constraints for each
			totalParticipantWeight := currentEpochData.GetParticipantWeight(participantAddr)
			for _, node := range filteredNodes {
				currentParticipantWeight := eligibleNodesData.GetParticipantWeight(participantAddr)
				eligibleNodesWeightIfAdded := currentParticipantWeight + node.PocWeight

				// Phase 3: Check if adding this node would violate voting constraints
				// Ensures we don't exceed 30% non-voting weight limit
				canAllocate, updatedWeight := canAllocateParticipantNode(
					eligibleNodesWeightIfAdded,
					totalParticipantWeight,
					totalNonVotingWeight,
					maxAllowedNonVotingWeight,
				)
				if !canAllocate {
					// Stop adding nodes for this participant - would violate constraints
					break
				}
				totalNonVotingWeight = updatedWeight
				eligibleNodesData.Append(modelId, participantAddr, node)
			}
		}
	}

	return eligibleNodesData
}

// allocateMLNodePerPoCForModel applies weight-based allocation logic across all participants for a specific model
func (ma *ModelAssigner) allocateMLNodePerPoCForModel(
	modelId string,
	currentEpochData *EpochMLNodeData,
	eligibleNodesData *EpochMLNodeData,
	fraction *types.Decimal,
) {
	ma.LogInfo("Starting allocation for model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "model_allocation_start", "model_id", modelId)

	totalWeight := currentEpochData.GetTotalWeightForModel(modelId)

	fractionDecimal := fraction.ToDecimal()
	targetPoCWeightDecimal := fractionDecimal.Mul(decimal.NewFromInt(totalWeight))
	targetPoCWeight := targetPoCWeightDecimal.IntPart()

	ma.LogInfo("Calculated target weight for model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "calculate_target_weight", "model_id", modelId, "total_weight", totalWeight, "fraction", fractionDecimal.String(), "target_weight", targetPoCWeight)

	// Phase B: Get eligible participants (sorted alphabetically for determinism)
	eligibleModelNodes := eligibleNodesData.GetForModel(modelId)
	eligibleParticipantAddrs := sortedKeys(eligibleModelNodes)

	ma.LogInfo("Built participant list", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "build_participants", "model_id", modelId, "num_participants", len(eligibleParticipantAddrs))

	if len(eligibleParticipantAddrs) == 0 {
		ma.LogInfo("No participants with eligible nodes for this model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "no_participants", "model_id", modelId)
		return
	}

	// Phase C: Round-robin allocation
	var currentWeight int64
	currentParticipantIdx := 0
	allocatedInRound := false

	for currentWeight < targetPoCWeight {
		participantAddr := eligibleParticipantAddrs[currentParticipantIdx]
		nodes := eligibleNodesData.GetForParticipant(modelId, participantAddr)

		nextMLNode := getSmallestMLNodeWithPOCSLotFalse(nodes)

		if nextMLNode == nil {
			// No eligible nodes for this participant, try next
			currentParticipantIdx = (currentParticipantIdx + 1) % len(eligibleParticipantAddrs)

			// Check if we completed full round without allocation
			if currentParticipantIdx == 0 {
				if !allocatedInRound {
					ma.LogInfo("Completed full round without allocation, exiting", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "exit_no_nodes", "model_id", modelId, "current_weight", currentWeight, "target_weight", targetPoCWeight)
					break
				}
				allocatedInRound = false
			}
			continue
		}

		// Allocate this node
		nextMLNode.TimeslotAllocation[1] = true
		currentWeight += nextMLNode.PocWeight
		allocatedInRound = true

		ma.LogInfo("Allocated node to PoC slot", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "allocate_node", "model_id", modelId, "participant", participantAddr, "node_id", nextMLNode.NodeId, "node_weight", nextMLNode.PocWeight, "current_weight", currentWeight, "target_weight", targetPoCWeight)

		currentParticipantIdx = (currentParticipantIdx + 1) % len(eligibleParticipantAddrs)

		if currentParticipantIdx == 0 {
			allocatedInRound = false
		}
	}

	// Phase D: Log allocation summary per participant
	for _, participantAddr := range eligibleParticipantAddrs {
		nodes := eligibleNodesData.GetForParticipant(modelId, participantAddr)
		var allocatedCount int
		var allocatedWeight int64
		var allocatedNodeIds []string

		for _, node := range nodes {
			if len(node.TimeslotAllocation) > 1 && node.TimeslotAllocation[1] {
				allocatedCount++
				allocatedWeight += node.PocWeight
				allocatedNodeIds = append(allocatedNodeIds, node.NodeId)
			}
		}

		ma.LogInfo("Participant allocation summary", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "participant_summary", "model_id", modelId, "participant", participantAddr, "total_nodes", len(nodes), "allocated_nodes", allocatedCount, "allocated_weight", allocatedWeight, "allocated_node_ids", allocatedNodeIds)
	}

	ma.LogInfo("Finished allocation for model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "model_allocation_end", "model_id", modelId, "achieved_weight", currentWeight, "target_weight", targetPoCWeight, "total_weight", totalWeight)
}

// getSmallestMLNodeWithPOCSLotFalse returns the smallest node (by PocWeight) that has POC_SLOT=false
// Returns nil if no such node exists
func getSmallestMLNodeWithPOCSLotFalse(nodes []*types.MLNodeInfo) *types.MLNodeInfo {
	var smallest *types.MLNodeInfo
	for _, node := range nodes {
		if len(node.TimeslotAllocation) > 1 && !node.TimeslotAllocation[1] {
			if smallest == nil || node.PocWeight < smallest.PocWeight {
				smallest = node
			}
		}
	}
	return smallest
}

// calculateParticipantWeightThreshold70Percent calculates the minimum participant weight threshold
// to ensure participants with top 70% of total weight are included.
//
// Returns the weight threshold such that participants with weight > threshold sum to >= 70% of total weight.
// Returns 0 if all participants are needed (edge cases: 0, 1 participant, or cumulative includes all).
//
// The returned threshold is (w - 1) where w is the weight that reaches the 70% target.
// We subtract 1 to ensure we include participants at exactly weight w, not exclude them.
func calculateParticipantWeightThreshold70Percent(weights []int64) int64 {
	if len(weights) == 0 {
		return 0
	}
	if len(weights) == 1 {
		return 0
	}

	// Calculate total and target weight (70%)
	totalWeight := int64(0)
	for _, w := range weights {
		totalWeight += w
	}
	targetWeight := (totalWeight * 70) / 100

	// Sort descending
	sorted := make([]int64, len(weights))
	copy(sorted, weights)
	slices.SortFunc(sorted, func(a, b int64) int {
		if a > b {
			return -1
		}
		if a < b {
			return 1
		}
		return 0
	})

	// Accumulate until reaching target
	sum := int64(0)
	for i, w := range sorted {
		sum += w
		if sum >= targetWeight {
			// If this is the last weight, all participants included, no threshold needed
			if i == len(sorted)-1 {
				return 0
			}
			// Return threshold as w-1 to include participants at exactly this weight
			// We want participants with weight >= w, so threshold is w-1
			return w - 1
		}
	}

	return 0
}

// calculatePerParticipantThreshold calculates minimum node weight threshold for each participant
// to ensure their top 25% nodes (by weight) are included in eligible set.
//
// Only calculates thresholds for participants meeting the participantWeightThreshold (top 70% participants).
// For each qualifying participant, finds the minimum weight threshold where nodes with weight > threshold
// sum to >= 25% of participant's total node weight.
//
// Returns map of participantAddr -> minimum node weight threshold.
// Returns 0 for a participant if all their nodes are needed (edge cases: 0, 1 node, or cumulative includes all).
//
// The returned threshold is (w - 1) where w is the weight that reaches the 25% target.
// We subtract 1 to ensure we include nodes at exactly weight w, not exclude them.
func calculatePerParticipantThreshold(epochData *EpochMLNodeData, participantWeightThreshold int64) map[string]int64 {
	result := make(map[string]int64)

	// Calculate threshold only for participants meeting the weight threshold
	for participantAddr, participantWeight := range epochData.partcipantsWeights {
		// Only calculate for participants with weight >= threshold (top 70%)
		if participantWeight < participantWeightThreshold {
			continue
		}
		// Collect all node weights for this participant across all models
		nodeWeights := make([]int64, 0)
		totalWeight := int64(0)
		for _, modelData := range epochData.data {
			if nodes, ok := modelData[participantAddr]; ok {
				for _, node := range nodes {
					nodeWeights = append(nodeWeights, node.PocWeight)
					totalWeight += node.PocWeight
				}
			}
		}

		// Calculate 25% threshold
		if len(nodeWeights) == 0 {
			result[participantAddr] = 0
			continue
		}
		if len(nodeWeights) == 1 {
			result[participantAddr] = 0
			continue
		}

		targetWeight := (totalWeight * 25) / 100

		// Sort descending
		sorted := make([]int64, len(nodeWeights))
		copy(sorted, nodeWeights)
		slices.SortFunc(sorted, func(a, b int64) int {
			if a > b {
				return -1
			}
			if a < b {
				return 1
			}
			return 0
		})

		// Accumulate until reaching target
		sum := int64(0)
		threshold := int64(0)
		for i, w := range sorted {
			sum += w
			if sum >= targetWeight {
				if i == len(sorted)-1 {
					// All nodes needed, no threshold
					threshold = 0
				} else {
					// Return threshold as w-1 to include nodes at exactly this weight
					threshold = w - 1
				}
				break
			}
		}

		result[participantAddr] = threshold
	}

	return result
}

// calculateNodeWeightThresholdIQR calculates outlier threshold using IQR (Interquartile Range) method
// Returns Q3 + 1.5*IQR, filtering nodes with anomalously high weights
// Uses pure integer arithmetic for blockchain determinism
func calculateNodeWeightThresholdIQR(weights []int64) int64 {
	if len(weights) == 0 {
		return 0
	}

	if len(weights) == 1 {
		return weights[0]
	}

	// Sort weights for quartile calculation
	sortedWeights := make([]int64, len(weights))
	copy(sortedWeights, weights)
	slices.Sort(sortedWeights)

	n := len(sortedWeights)

	// Calculate Q1 (25th percentile) and Q3 (75th percentile)
	// Using integer division - deterministic across all nodes
	q1Index := n / 4
	q3Index := (n * 3) / 4

	// Ensure indices are within bounds
	if q3Index >= n {
		q3Index = n - 1
	}

	q1 := sortedWeights[q1Index]
	q3 := sortedWeights[q3Index]

	// Calculate IQR (Interquartile Range)
	iqr := q3 - q1

	// Threshold = Q3 + 1.5*IQR (standard outlier detection)
	// Using integer math: 1.5*IQR = IQR + IQR/2
	threshold := q3 + iqr + (iqr / 2)

	return threshold
}

// filterNodesByWeight filters out nodes with weight greater than threshold.
// If threshold is 0, no filtering is applied (all nodes are included).
// Returns nodes sorted by weight (ascending).
func filterNodesByWeight(nodes []*types.MLNodeInfo, threshold int64) []*types.MLNodeInfo {
	filtered := make([]*types.MLNodeInfo, 0, len(nodes))

	// threshold=0 means "no filtering, include all nodes"
	if threshold == 0 {
		filtered = append(filtered, nodes...)
	} else {
		for _, node := range nodes {
			if node.PocWeight <= threshold {
				filtered = append(filtered, node)
			}
		}
	}

	slices.SortFunc(filtered, func(a, b *types.MLNodeInfo) int {
		if a.PocWeight < b.PocWeight {
			return -1
		}
		if a.PocWeight > b.PocWeight {
			return 1
		}
		return 0
	})
	return filtered
}

// calculateEffectiveNodeThreshold returns the more restrictive threshold between participant and global thresholds.
// A threshold of 0 means "no filtering" (include all nodes).
// Returns the minimum of the two thresholds, with special handling for 0 values.
func calculateEffectiveNodeThreshold(participantThreshold, globalThreshold int64) int64 {
	// If participant threshold is 0 (no filtering), use global threshold
	if participantThreshold == 0 {
		return globalThreshold
	}
	// If global threshold is 0 (no filtering), use participant threshold
	if globalThreshold == 0 {
		return participantThreshold
	}
	// Use minimum (more restrictive threshold)
	if participantThreshold < globalThreshold {
		return participantThreshold
	}
	return globalThreshold
}

// canAllocateParticipantNode checks if a node can be allocated without violating voting constraints.
//
// VOTING CONSTRAINTS:
// A participant can only vote in PoC validation if they have at least some nodes with POC_SLOT=false.
// If all a participant's nodes have POC_SLOT=true (all in eligible set), they become "non-voting".
//
// To ensure sufficient PoC validation, we limit non-voting participants to <30% of total weight.
// This guarantees at least 70% of weight can participate in PoC validation.
//
// PARAMETERS:
//   - eligibleNodesWeightIfAdded: Total weight of eligible nodes if we add the current node being considered
//   - totalParticipantWeight: Total weight of all the participant's nodes
//   - totalNonVotingWeight: Current sum of weights for all non-voting participants
//   - maxAllowedNonVotingWeight: Maximum allowed total weight for non-voting participants (30% threshold)
//
// RETURNS:
//   - canAllocate: Whether this node can be added to eligible set
//   - updatedNonVotingWeight: Updated total non-voting weight if node is allocated
func canAllocateParticipantNode(
	eligibleNodesWeightIfAdded, totalParticipantWeight int64,
	totalNonVotingWeight, maxAllowedNonVotingWeight int64,
) (canAllocate bool, updatedNonVotingWeight int64) {
	// Check if adding this node would make all participant's nodes eligible (participant becomes non-voting)
	if eligibleNodesWeightIfAdded >= totalParticipantWeight {
		// Check if adding this participant's weight to non-voting group would exceed 30% threshold
		if totalNonVotingWeight+totalParticipantWeight < maxAllowedNonVotingWeight {
			// Can allocate - participant becomes non-voting but total non-voting weight still under limit
			return true, totalNonVotingWeight + totalParticipantWeight
		}
		// Cannot allocate - would exceed non-voting weight limit
		return false, totalNonVotingWeight
	}
	// Can allocate - participant will still have nodes for voting (POC_SLOT=false)
	return true, totalNonVotingWeight
}

// sampleEligibleParticipantsWithHistory filters participants with previous epoch history
// and deterministically samples N/2+1 of them to be eligible for the current epoch
// Returns a slice of eligible participant addresses
func (ma *ModelAssigner) sampleEligibleParticipantsWithHistory(
	sortedParticipantAddrs []string,
	previousEpochData *EpochMLNodeData,
	modelId string,
	upcomingEpoch types.Epoch,
	allParticipantsHashStr string,
) []string {
	// Filter participants with previous epoch data
	participantsWithHistory := make([]string, 0)
	for _, participantAddr := range sortedParticipantAddrs {
		previousValidationWeight := previousEpochData.GetForParticipant(modelId, participantAddr)

		if previousValidationWeight == nil {
			continue
		}

		participantsWithHistory = append(participantsWithHistory, participantAddr)
	}

	// If no participants with history or epoch 0, return empty slice
	if len(participantsWithHistory) == 0 || upcomingEpoch.Index == 0 {
		return []string{}
	}

	// Create deterministic seed for shuffling participants for this model
	seed := fmt.Sprintf("filter_%d_%s_%s", upcomingEpoch.Index, allParticipantsHashStr, modelId)
	hash := sha256.Sum256([]byte(seed))
	seedInt := int64(binary.BigEndian.Uint64(hash[:8]))
	rng := rand.New(rand.NewSource(seedInt))

	ma.LogInfo("Generated deterministic seed for participant selection", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "generate_filter_seed", "model_id", modelId, "seed_string", seed, "seed_int", seedInt)

	// Shuffle participants deterministically
	shuffledParticipants := make([]string, len(participantsWithHistory))
	copy(shuffledParticipants, participantsWithHistory)
	rng.Shuffle(len(shuffledParticipants), func(i, j int) {
		shuffledParticipants[i], shuffledParticipants[j] = shuffledParticipants[j], shuffledParticipants[i]
	})

	// Select N/2 + 1 participants to be eligible
	numEligible := min(len(sortedParticipantAddrs)/2+1, len(shuffledParticipants))
	eligibleParticipantsPerModel := make([]string, 0, numEligible)
	for i := 0; i < numEligible && i < len(shuffledParticipants); i++ {
		eligibleParticipantsPerModel = append(eligibleParticipantsPerModel, shuffledParticipants[i])
	}

	ma.LogInfo("Selected eligible participants", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "select_eligible_participants", "model_id", modelId, "total_participants", len(participantsWithHistory), "eligible_participants", numEligible)

	return eligibleParticipantsPerModel
}

// Helper function to create a map of modelId to supported models
func supportedModelsByNode(hardwareNodes *types.HardwareNodes, governanceModels []*types.Model) map[string][]string {
	governanceModelsMap := make(map[string]bool)
	for _, model := range governanceModels {
		governanceModelsMap[model.Id] = true
	}

	supportedModelsByNode := make(map[string][]string)
	for _, node := range hardwareNodes.HardwareNodes {
		// keep only the models that are in the governanceModelsMap
		supportedModels := make([]string, 0)
		for _, model := range node.Models {
			if governanceModelsMap[model] {
				supportedModels = append(supportedModels, model)
			}
		}
		supportedModelsByNode[node.LocalId] = supportedModels
	}

	return supportedModelsByNode
}
