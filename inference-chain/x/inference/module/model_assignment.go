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
	data map[string]map[string][]*types.MLNodeInfo
}

func NewEpochMLNodeData() *EpochMLNodeData {
	return &EpochMLNodeData{
		data: make(map[string]map[string][]*types.MLNodeInfo),
	}
}

func (e *EpochMLNodeData) Set(modelId, participantAddr string, nodes []*types.MLNodeInfo) {
	if e.data[modelId] == nil {
		e.data[modelId] = make(map[string][]*types.MLNodeInfo)
	}
	e.data[modelId][participantAddr] = nodes
}

func (e *EpochMLNodeData) Append(modelId, participantAddr string, node *types.MLNodeInfo) {
	if e.data[modelId] == nil {
		e.data[modelId] = make(map[string][]*types.MLNodeInfo)
	}
	e.data[modelId][participantAddr] = append(e.data[modelId][participantAddr], node)
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

func (e *EpochMLNodeData) GetAllIndividualNodeWeights() []int64 {
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

func (e *EpochMLNodeData) GetAllParticipantWeights() []int64 {
	participantWeights := make(map[string]int64)
	for _, modelData := range e.data {
		for participantAddr, nodes := range modelData {
			for _, node := range nodes {
				participantWeights[participantAddr] += node.PocWeight
			}
		}
	}

	weights := make([]int64, 0, len(participantWeights))
	for _, weight := range participantWeights {
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

	// IMPORTANT: Maintain exact string format for blockchain determinism
	// Changing this format would alter all subsequent random selections
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
	var weight int64
	for _, modelData := range e.data {
		if nodes, ok := modelData[participantAddr]; ok {
			for _, node := range nodes {
				weight += node.PocWeight
			}
		}
	}
	return weight
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

		for _, mlNode := range originalMLNodes {
			mlNode.TimeslotAllocation = []bool{true, false} // [PRE_POC_SLOT, POC_SLOT]
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

				if slices.Contains(supportedModelsByNode[mlNode.NodeId], model.Id) {
					ma.LogInfo("Found supporting and unassigned ML node for model", types.EpochGroup, "flow_context", FlowContext, "step", "assign_node_to_model", "participant_index", p.Index, "model_id", model.Id, "node_id", mlNode.NodeId)
					modelMLNodes = append(modelMLNodes, mlNode)
					assignedMLNodes[mlNode.NodeId] = true
				}
			}

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

	eligibleNodesData := ma.filterEligibleMLNodes(upcomingEpoch, previousEpochData, currentEpochData)
	ma.LogInfo("Filtered eligible nodes for all models", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "filter_all_eligible", "num_models", len(eligibleNodesData.Models()))

	for _, modelId := range sortedModelIds {
		ma.LogInfo("Processing model for PoC allocation", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "model_loop_start", "model_id", modelId)
		ma.allocateMLNodePerPoCForModel(modelId, currentEpochData, eligibleNodesData, allocationFraction)
	}
}

// thresholdSet holds the calculated thresholds for participant and node weight filtering
type thresholdSet struct {
	participantMinNodeWeights map[string]int64 // per-participant minimum node weight (25% rule)
	globalMaxNodeWeight       int64            // global outlier threshold (IQR method)
}

func (ma *ModelAssigner) calculateThresholds(currentEpochData *EpochMLNodeData) thresholdSet {
	allParticipantsWeights := currentEpochData.GetAllParticipantWeights()
	participantWeightThreshold := calculateParticipantWeightThreshold70Percent(allParticipantsWeights)
	ma.LogInfo("Calculated participant weight threshold (70% rule)", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "calculate_participant_threshold", "threshold", participantWeightThreshold, "total_participants", len(allParticipantsWeights))

	participantMinNodeWeightThresholds := calculatePerParticipantThreshold(currentEpochData, participantWeightThreshold)
	ma.LogInfo("Calculated per-participant node thresholds (25% rule)", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "calculate_per_participant_thresholds", "total_participants", len(participantMinNodeWeightThresholds))

	allNodesWeights := currentEpochData.GetAllIndividualNodeWeights()
	globalMaxNodeWeightThreshold := calculateNodeWeightThresholdIQR(allNodesWeights)
	ma.LogInfo("Calculated node weight threshold (IQR method)", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "calculate_node_threshold", "threshold", globalMaxNodeWeightThreshold, "total_nodes", len(allNodesWeights))

	return thresholdSet{
		participantMinNodeWeights: participantMinNodeWeightThresholds,
		globalMaxNodeWeight:       globalMaxNodeWeightThreshold,
	}
}

// filterNodesByThresholds applies effective threshold filtering to nodes for a participant
func filterNodesByThresholds(nodes []*types.MLNodeInfo, participantAddr string, thresholds thresholdSet) []*types.MLNodeInfo {
	threshold := calculateEffectiveNodeThreshold(
		thresholds.participantMinNodeWeights[participantAddr],
		thresholds.globalMaxNodeWeight,
	)
	return filterNodesByWeight(nodes, threshold)
}

func buildEligibleParticipantSet(currentEpochData *EpochMLNodeData, thresholds thresholdSet) map[string]bool {
	eligibleParticipantAddrs := make(map[string]bool)
	for _, modelData := range currentEpochData.data {
		for participantAddr, nodes := range modelData {
			if eligibleParticipantAddrs[participantAddr] {
				continue
			}
			filteredNodes := filterNodesByThresholds(nodes, participantAddr, thresholds)
			if len(filteredNodes) > 0 {
				eligibleParticipantAddrs[participantAddr] = true
			}
		}
	}
	return eligibleParticipantAddrs
}

// filterEligibleMLNodes filters which nodes are eligible for POC_SLOT=true allocation across all models.
//
// PURPOSE:
// Determines which ML nodes can be allocated POC_SLOT=true (serve inference during PoC phase).
// Uses multi-phase filtering to ensure sufficient PoC validation participation while filtering outliers.
//
// FILTERING PHASES:
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
) *EpochMLNodeData {
	allParticipantsHashStr := currentEpochData.GetAllParticipantsHash()

	// Step 1: Calculate all thresholds (70% + 25% rule, IQR outlier detection)
	thresholds := ma.calculateThresholds(currentEpochData)

	// Step 2: Build set of eligible participants (those with nodes passing weight thresholds)
	eligibleParticipantAddrs := buildEligibleParticipantSet(currentEpochData, thresholds)

	// Step 3: Apply thresholds and sample participants per model
	eligibleNodesData := NewEpochMLNodeData()
	for _, modelId := range currentEpochData.Models() {
		participantNodes := currentEpochData.GetForModel(modelId)
		sortedParticipantAddrs := sortedKeys(participantNodes)

		var filteredParticipantAddrs []string
		for _, addr := range sortedParticipantAddrs {
			if eligibleParticipantAddrs[addr] {
				filteredParticipantAddrs = append(filteredParticipantAddrs, addr)
			}
		}

		// Sample N/2+1 participants with history for rotation (deterministic per epoch+model)
		eligibleParticipantsPerModel := ma.sampleEligibleParticipantsWithHistory(
			filteredParticipantAddrs,
			previousEpochData,
			modelId,
			upcomingEpoch,
			allParticipantsHashStr,
		)

		for _, participantAddr := range eligibleParticipantsPerModel {
			currentNodes := participantNodes[participantAddr]
			filteredNodes := filterNodesByThresholds(currentNodes, participantAddr, thresholds)

			for _, node := range filteredNodes {
				eligibleNodesData.Append(modelId, participantAddr, node)
			}
		}
	}

	return eligibleNodesData
}

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

	eligibleModelNodes := eligibleNodesData.GetForModel(modelId)
	eligibleParticipantAddrs := sortedKeys(eligibleModelNodes)

	ma.LogInfo("Built participant list", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "build_participants", "model_id", modelId, "num_participants", len(eligibleParticipantAddrs))

	if len(eligibleParticipantAddrs) == 0 {
		ma.LogInfo("No participants with eligible nodes for this model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "no_participants", "model_id", modelId)
		return
	}

	var currentWeight int64
	currentParticipantIdx := 0
	allocatedInRound := false

	for currentWeight < targetPoCWeight {
		participantAddr := eligibleParticipantAddrs[currentParticipantIdx]
		nodes := eligibleNodesData.GetForParticipant(modelId, participantAddr)

		nextMLNode := getSmallestMLNodeWithPOCSLotFalse(nodes)

		if nextMLNode == nil {
			currentParticipantIdx = (currentParticipantIdx + 1) % len(eligibleParticipantAddrs)

			if currentParticipantIdx == 0 {
				if !allocatedInRound {
					ma.LogInfo("Completed full round without allocation, exiting", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "exit_no_nodes", "model_id", modelId, "current_weight", currentWeight, "target_weight", targetPoCWeight)
					break
				}
				allocatedInRound = false
			}
			continue
		}

		nextMLNode.TimeslotAllocation[1] = true
		currentWeight += nextMLNode.PocWeight
		allocatedInRound = true

		ma.LogInfo("Allocated node to PoC slot", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "allocate_node", "model_id", modelId, "participant", participantAddr, "node_id", nextMLNode.NodeId, "node_weight", nextMLNode.PocWeight, "current_weight", currentWeight, "target_weight", targetPoCWeight)

		currentParticipantIdx = (currentParticipantIdx + 1) % len(eligibleParticipantAddrs)

		if currentParticipantIdx == 0 {
			allocatedInRound = false
		}
	}

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

// calculateWeightThreshold calculates minimum weight threshold to reach targetPercent of total weight.
// Returns (w - 1) where w reaches targetPercent. Returns 0 if all weights needed.
func calculateWeightThreshold(weights []int64, targetPercent int) int64 {
	if len(weights) == 0 || len(weights) == 1 {
		return 0
	}

	totalWeight := int64(0)
	for _, w := range weights {
		totalWeight += w
	}
	targetWeight := (totalWeight * int64(targetPercent)) / 100

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
			if i == len(sorted)-1 {
				return 0
			}
			// Return threshold as w-1 to include items at exactly this weight
			return w - 1
		}
	}

	return 0
}

// calculateParticipantWeightThreshold70Percent calculates the minimum participant weight threshold
// to ensure participants with top 70% of total weight are included.
//
// Returns the weight threshold such that participants with weight > threshold sum to >= 70% of total weight.
// Returns 0 if all participants are needed (edge cases: 0, 1 participant, or cumulative includes all).
func calculateParticipantWeightThreshold70Percent(weights []int64) int64 {
	return calculateWeightThreshold(weights, 70)
}

// calculatePerParticipantThreshold calculates node weight thresholds for top 70% participants.
// For each participant, ensures top 25% of their nodes (by weight) are included.
func calculatePerParticipantThreshold(epochData *EpochMLNodeData, participantWeightThreshold int64) map[string]int64 {
	result := make(map[string]int64)

	uniqueParticipants := make(map[string]bool)
	for _, modelData := range epochData.data {
		for participantAddr := range modelData {
			uniqueParticipants[participantAddr] = true
		}
	}

	for participantAddr := range uniqueParticipants {
		participantWeight := epochData.GetParticipantWeight(participantAddr)

		if participantWeight < participantWeightThreshold {
			continue
		}

		nodeWeights := make([]int64, 0)
		for _, modelData := range epochData.data {
			if nodes, ok := modelData[participantAddr]; ok {
				for _, node := range nodes {
					nodeWeights = append(nodeWeights, node.PocWeight)
				}
			}
		}

		result[participantAddr] = calculateWeightThreshold(nodeWeights, 25)
	}

	return result
}

// calculateNodeWeightThresholdIQR calculates outlier threshold using IQR method (Q3 + 1.5*IQR).
// Uses integer arithmetic for blockchain determinism.
func calculateNodeWeightThresholdIQR(weights []int64) int64 {
	if len(weights) == 0 {
		return 0
	}
	if len(weights) == 1 {
		return weights[0]
	}

	sortedWeights := make([]int64, len(weights))
	copy(sortedWeights, weights)
	slices.Sort(sortedWeights)

	n := len(sortedWeights)
	q1Index := n / 4
	q3Index := (n * 3) / 4

	if q3Index >= n {
		q3Index = n - 1
	}

	q1 := sortedWeights[q1Index]
	q3 := sortedWeights[q3Index]
	iqr := q3 - q1

	// 1.5*IQR = IQR + IQR/2
	threshold := q3 + iqr + (iqr / 2)

	return threshold
}

// filterNodesByWeight filters nodes with weight <= threshold. threshold=0 means no filtering.
// Returns nodes sorted ascending for deterministic allocation.
func filterNodesByWeight(nodes []*types.MLNodeInfo, threshold int64) []*types.MLNodeInfo {
	filtered := make([]*types.MLNodeInfo, 0, len(nodes))

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

// calculateEffectiveNodeThreshold returns the more restrictive (lower) threshold.
// threshold=0 means "no filtering".
func calculateEffectiveNodeThreshold(participantThreshold, globalThreshold int64) int64 {
	if participantThreshold == 0 {
		return globalThreshold
	}
	if globalThreshold == 0 {
		return participantThreshold
	}
	return min(participantThreshold, globalThreshold)
}

// sampleEligibleParticipantsWithHistory deterministically samples N/2+1 participants with history.
func (ma *ModelAssigner) sampleEligibleParticipantsWithHistory(
	sortedParticipantAddrs []string,
	previousEpochData *EpochMLNodeData,
	modelId string,
	upcomingEpoch types.Epoch,
	allParticipantsHashStr string,
) []string {
	participantsWithHistory := make([]string, 0)
	for _, participantAddr := range sortedParticipantAddrs {
		previousValidationWeight := previousEpochData.GetForParticipant(modelId, participantAddr)

		if previousValidationWeight == nil {
			continue
		}

		participantsWithHistory = append(participantsWithHistory, participantAddr)
	}

	if len(participantsWithHistory) == 0 || upcomingEpoch.Index == 0 {
		return []string{}
	}

	seed := fmt.Sprintf("filter_%d_%s_%s", upcomingEpoch.Index, allParticipantsHashStr, modelId)
	hash := sha256.Sum256([]byte(seed))
	seedInt := int64(binary.BigEndian.Uint64(hash[:8]))
	rng := rand.New(rand.NewSource(seedInt))

	ma.LogInfo("Generated deterministic seed for participant selection", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "generate_filter_seed", "model_id", modelId, "seed_string", seed, "seed_int", seedInt)

	shuffledParticipants := make([]string, len(participantsWithHistory))
	copy(shuffledParticipants, participantsWithHistory)
	rng.Shuffle(len(shuffledParticipants), func(i, j int) {
		shuffledParticipants[i], shuffledParticipants[j] = shuffledParticipants[j], shuffledParticipants[i]
	})

	numEligible := min(len(sortedParticipantAddrs)/2+1, len(shuffledParticipants))
	eligibleParticipantsPerModel := make([]string, 0, numEligible)
	for i := 0; i < numEligible && i < len(shuffledParticipants); i++ {
		eligibleParticipantsPerModel = append(eligibleParticipantsPerModel, shuffledParticipants[i])
	}

	ma.LogInfo("Selected eligible participants", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "select_eligible_participants", "model_id", modelId, "total_participants", len(participantsWithHistory), "eligible_participants", numEligible)

	return eligibleParticipantsPerModel
}

func supportedModelsByNode(hardwareNodes *types.HardwareNodes, governanceModels []*types.Model) map[string][]string {
	governanceModelsMap := make(map[string]bool)
	for _, model := range governanceModels {
		governanceModelsMap[model.Id] = true
	}

	supportedModelsByNode := make(map[string][]string)
	for _, node := range hardwareNodes.HardwareNodes {
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
