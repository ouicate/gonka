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

// EpochMLNodeData stores ML node information indexed by [modelId][participantAddress]
// Used for tracking current, previous, and eligible ML nodes across epochs
type EpochMLNodeData struct {
	data map[string]map[string][]*types.MLNodeInfo
}

// NewEpochMLNodeData creates a new EpochMLNodeData instance
func NewEpochMLNodeData() *EpochMLNodeData {
	return &EpochMLNodeData{
		data: make(map[string]map[string][]*types.MLNodeInfo),
	}
}

// Set stores ML nodes for a specific model and participant
func (e *EpochMLNodeData) Set(modelId, participantAddr string, nodes []*types.MLNodeInfo) {
	if e.data[modelId] == nil {
		e.data[modelId] = make(map[string][]*types.MLNodeInfo)
	}
	e.data[modelId][participantAddr] = nodes
}

// GetForModel returns all participants' nodes for a given model
func (e *EpochMLNodeData) GetForModel(modelId string) map[string][]*types.MLNodeInfo {
	return e.data[modelId]
}

// GetForParticipant returns nodes for a specific participant and model
func (e *EpochMLNodeData) GetForParticipant(modelId, participantAddr string) []*types.MLNodeInfo {
	if e.data[modelId] == nil {
		return nil
	}
	return e.data[modelId][participantAddr]
}

// HasModel checks if the model exists in the data
func (e *EpochMLNodeData) HasModel(modelId string) bool {
	return e.data[modelId] != nil
}

// Models returns all model IDs
func (e *EpochMLNodeData) Models() []string {
	models := make([]string, 0, len(e.data))
	for modelId := range e.data {
		models = append(models, modelId)
	}
	return models
}

// PreviousEpochData stores ValidationWeight from previous epoch indexed by [modelId][participantAddress]
type PreviousEpochData struct {
	data map[string]map[string]*types.ValidationWeight
}

// NewPreviousEpochData creates a new PreviousEpochData instance
func NewPreviousEpochData() *PreviousEpochData {
	return &PreviousEpochData{
		data: make(map[string]map[string]*types.ValidationWeight),
	}
}

// Set stores ValidationWeight for a specific model and participant
func (p *PreviousEpochData) Set(modelId, participantAddr string, vw *types.ValidationWeight) {
	if p.data[modelId] == nil {
		p.data[modelId] = make(map[string]*types.ValidationWeight)
	}
	p.data[modelId][participantAddr] = vw
}

// GetForModel returns all participants' ValidationWeights for a given model
func (p *PreviousEpochData) GetForModel(modelId string) map[string]*types.ValidationWeight {
	return p.data[modelId]
}

// GetForParticipant returns ValidationWeight for a specific participant and model
func (p *PreviousEpochData) GetForParticipant(modelId, participantAddr string) *types.ValidationWeight {
	if p.data[modelId] == nil {
		return nil
	}
	return p.data[modelId][participantAddr]
}

// HasModel checks if the model exists in the data
func (p *PreviousEpochData) HasModel(modelId string) bool {
	return p.data[modelId] != nil
}

// Models returns all model IDs
func (p *PreviousEpochData) Models() []string {
	models := make([]string, 0, len(p.data))
	for modelId := range p.data {
		models = append(models, modelId)
	}
	return models
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
			// No hardware nodes - just set empty arrays
			ma.LogInfo("No hardware nodes found for participant, skipping model assignment.", types.EpochGroup, "flow_context", FlowContext, "step", "no_hardware_nodes", "participant_index", p.Index)
			p.Models = make([]string, 0)
			p.MlNodes = make([]*types.ModelMLNodes, 0)
			continue
		}

		// Get the original MLNodes from the first array (index 0) - populated by task 5.8
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

		// Track which MLNodes have been assigned
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
					continue // MLNode already assigned to another model
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

		// Add remaining unassigned MLNodes as overflow array (if any exist)
		var unassignedMLNodes []*types.MLNodeInfo
		for _, mlNode := range originalMLNodes {
			if !assignedMLNodes[mlNode.NodeId] {
				unassignedMLNodes = append(unassignedMLNodes, mlNode)
			}
		}
		ma.LogInfo("Unassigned MLNodes", types.EpochGroup, "flow_context", FlowContext, "step", "unassigned_nodes", "participant_index", p.Index, "unassigned_nodes", unassignedMLNodes)

		// Update participant with reorganized MLNode arrays and supported models
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

// allocateMLNodesForPoC orchestrates the allocation process for PoC slots across all participants
// It queries previous epoch data, filters eligible nodes, and applies allocation logic
func (ma *ModelAssigner) allocateMLNodesForPoC(ctx context.Context, upcomingEpoch types.Epoch, participants []*types.ActiveParticipant) {
	ma.LogInfo("Starting ML node allocation for PoC slots", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "start", "num_participants", len(participants))

	// Default allocation percentage: 50%
	allocationPercentage := types.DecimalFromFloat(50.0)

	// Phase 1: Build previous epoch data map
	previousEpochData := NewPreviousEpochData()

	// Collect unique model IDs
	uniqueModels := make(map[string]bool)
	for _, participant := range participants {
		for _, modelId := range participant.Models {
			uniqueModels[modelId] = true
		}
	}
	ma.LogInfo("Collected unique models", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "collect_unique_models", "num_unique_models", len(uniqueModels))

	// Query previous epoch data for each model
	if upcomingEpoch.Index > 0 {
		for modelId := range uniqueModels {
			previousEpochGroupData, found := ma.keeper.GetEpochGroupData(ctx, upcomingEpoch.Index-1, modelId)
			if found {
				for _, vw := range previousEpochGroupData.ValidationWeights {
					previousEpochData.Set(modelId, vw.MemberAddress, vw)
				}
				ma.LogInfo("Loaded previous epoch data for model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "load_prev_epoch_data", "model_id", modelId, "num_validation_weights", len(previousEpochGroupData.ValidationWeights))
			}
		}
	}

	// Phase 2: Build current epoch data map
	currentEpochData := NewEpochMLNodeData()
	for _, participant := range participants {
		for modelIdx, modelId := range participant.Models {
			if modelIdx >= len(participant.MlNodes) {
				ma.LogInfo("Model index out of bounds, skipping", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "model_index_oob", "participant_index", participant.Index, "model_id", modelId, "model_idx", modelIdx)
				continue
			}
			currentEpochData.Set(modelId, participant.Index, participant.MlNodes[modelIdx].MlNodes)
		}
	}
	ma.LogInfo("Built current epoch data map", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "build_current_epoch_data", "num_models", len(currentEpochData.Models()))

	// Phase 3: Filter eligible nodes
	eligibleNodesData := ma.filterEligibleMLNodes(previousEpochData, currentEpochData)
	ma.LogInfo("Filtered eligible nodes for all models", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "filter_all_eligible", "num_models", len(eligibleNodesData.Models()))

	// Phase 4: Loop by model and allocate
	for modelId := range uniqueModels {
		ma.LogInfo("Processing model for PoC allocation", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "model_loop_start", "model_id", modelId)
		ma.allocateMLNodePerPoCForModel(ctx, upcomingEpoch, modelId, participants, eligibleNodesData, allocationPercentage)
	}

	ma.LogInfo("Finished ML node allocation for all participants", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "end")
}

// filterEligibleMLNodes filters which nodes are eligible for allocation across all models and participants
// Currently returns all nodes (stub implementation)
func (ma *ModelAssigner) filterEligibleMLNodes(
	previousEpochData *PreviousEpochData,
	currentEpochData *EpochMLNodeData,
) *EpochMLNodeData {
	ma.LogInfo("Starting eligible node filtering", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "filter_start")

	// Create filtered data
	eligibleNodesData := NewEpochMLNodeData()

	// Process each model
	for _, modelId := range currentEpochData.Models() {
		participantNodes := currentEpochData.GetForModel(modelId)

		// Process each participant for this model
		for participantAddr, currentNodes := range participantNodes {
			// Get previous epoch ValidationWeight (if exists)
			previousValidationWeight := previousEpochData.GetForParticipant(modelId, participantAddr)

			// Extract nodes that were on duty in previous epoch (POC_SLOT=true) for future filtering logic
			if previousValidationWeight != nil {
				var nodesOnDuty []string
				for _, node := range previousValidationWeight.MlNodes {
					if len(node.TimeslotAllocation) > 1 && node.TimeslotAllocation[1] {
						nodesOnDuty = append(nodesOnDuty, node.NodeId)
					}
				}
				ma.LogInfo("Extracted nodes on duty from previous epoch", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "extract_nodes_on_duty", "participant_index", participantAddr, "model_id", modelId, "nodes_on_duty", nodesOnDuty)
			}

			// Stub: include all current nodes as eligible
			eligibleNodesData.Set(modelId, participantAddr, currentNodes)
		}
	}

	ma.LogInfo("Finished eligible node filtering", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "filter_end", "num_models", len(eligibleNodesData.Models()))
	return eligibleNodesData
}

// allocateMLNodePerPoCForModel applies allocation logic to eligible nodes for a specific model
func (ma *ModelAssigner) allocateMLNodePerPoCForModel(
	ctx context.Context,
	upcomingEpoch types.Epoch,
	modelId string,
	participants []*types.ActiveParticipant,
	eligibleNodesData *EpochMLNodeData,
	percentage *types.Decimal,
) {
	ma.LogInfo("Starting allocation for model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "model_allocation_start", "model_id", modelId)

	// Process each participant for this model
	for _, participant := range participants {
		// Check if participant supports this model
		supportsModel := false
		for _, supportedModelId := range participant.Models {
			if supportedModelId == modelId {
				supportsModel = true
				break
			}
		}
		if !supportsModel {
			continue
		}

		ma.LogInfo("Processing participant for model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "participant_start", "participant_index", participant.Index, "model_id", modelId)

		// Get eligible nodes for this participant-model
		eligibleNodes := eligibleNodesData.GetForParticipant(modelId, participant.Index)
		ma.LogInfo("Retrieved eligible nodes", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "get_eligible", "participant_index", participant.Index, "model_id", modelId, "eligible_nodes", len(eligibleNodes))

		if len(eligibleNodes) == 0 {
			ma.LogInfo("No eligible nodes for allocation", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "no_eligible_nodes", "participant_index", participant.Index, "model_id", modelId)
			continue
		}

		// Create deterministic random seed from epoch ID, participant address, and model ID
		seed := fmt.Sprintf("%d_%s_%s", upcomingEpoch.Index, participant.Index, modelId)
		hash := sha256.Sum256([]byte(seed))
		seedInt := int64(binary.BigEndian.Uint64(hash[:8]))
		ma.LogInfo("Generated deterministic seed for random shuffling", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "generate_seed", "participant_index", participant.Index, "model_id", modelId, "seed_string", seed, "seed_int", seedInt)

		// Create random generator with deterministic seed for this model
		rng := rand.New(rand.NewSource(seedInt))

		// Create shuffled node indices for deterministic random order
		nodeIndices := make([]int, len(eligibleNodes))
		for i := range nodeIndices {
			nodeIndices[i] = i
		}
		rng.Shuffle(len(nodeIndices), func(i, j int) {
			nodeIndices[i], nodeIndices[j] = nodeIndices[j], nodeIndices[i]
		})
		ma.LogInfo("Shuffled node indices for model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "shuffle_nodes", "participant_index", participant.Index, "model_id", modelId, "shuffled_indices", nodeIndices)

		// Calculate how many nodes can serve inference using decimal math
		totalNodes := len(eligibleNodes)
		percentageDecimal := percentage.ToDecimal()
		nodesToInferenceDecimal := percentageDecimal.Mul(decimal.NewFromInt(int64(totalNodes))).Div(decimal.NewFromInt(100))
		nodesToInference := int(nodesToInferenceDecimal.IntPart())
		ma.LogInfo("Calculated node allocation for inference", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "calculate_allocation", "participant_index", participant.Index, "model_id", modelId, "total_nodes", totalNodes, "percentage", percentageDecimal.String(), "nodes_to_inference", nodesToInference)

		// Set POC_SLOT to true for the first nodesToInference shuffled nodes
		var inferenceNodeIds []string
		var pocOnlyNodeIds []string
		for i, nodeIdx := range nodeIndices {
			mlNode := eligibleNodes[nodeIdx]
			if i < nodesToInference {
				if len(mlNode.TimeslotAllocation) > 1 {
					mlNode.TimeslotAllocation[1] = true // Set POC_SLOT to true (serve inference)
					ma.LogInfo("Setting POC_SLOT=true for node", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "set_poc_slot", "participant_index", participant.Index, "model_id", modelId, "node_id", mlNode.NodeId)
				}
				inferenceNodeIds = append(inferenceNodeIds, mlNode.NodeId)
			} else {
				pocOnlyNodeIds = append(pocOnlyNodeIds, mlNode.NodeId)
			}
		}

		// Log the allocation for debugging
		ma.LogInfo("Applied node allocation for participant-model", types.EpochGroup,
			"flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "allocation_summary",
			"participantIndex", participant.Index,
			"modelId", modelId,
			"totalNodes", totalNodes,
			"nodesToInference", nodesToInference,
			"inferenceNodeIds", inferenceNodeIds,
			"nodesToPoC", totalNodes-nodesToInference,
			"pocOnlyNodeIds", pocOnlyNodeIds)
	}

	ma.LogInfo("Finished allocation for model", types.EpochGroup, "flow_context", FlowContext, "sub_flow_context", SubFlowContext, "step", "model_allocation_end", "model_id", modelId)
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
