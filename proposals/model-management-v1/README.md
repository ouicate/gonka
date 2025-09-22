# Model Management Proposal

## Overview

This proposal describes the model management architecture in the decentralized inference system, focusing on how MLNodes track and support AI models throughout the system lifecycle. The architecture uses a three-tier approach: broker-based node tracking, on-chain hardware node storage, and epoch-based model snapshots for consistent inference routing.

## System Architecture

### 1. Broker Node Tracking

The **Broker** in `decentralized-api/broker/broker.go` serves as the central coordinator for MLNode operations. During startup, nodes are loaded through `LoadNodeToBroker` and registered via `RegisterNode` commands. Each node maintains a `Models` map containing supported AI model identifiers and their configuration arguments.

The broker continuously synchronizes with the blockchain through `syncNodes()`, which runs every 60 seconds via `nodeSyncWorker`. This process calculates differences between local node configurations and on-chain state using `calculateNodesDiff`, then submits updates through `SubmitHardwareDiff` messages.

### 2. On-Chain Hardware Node Storage

Hardware nodes are persisted on the blockchain through the **HardwareNodes** structure in `inference-chain/x/inference/keeper/hardware_node.go`. The `MsgSubmitHardwareDiff` handler in `msg_server_submit_hardware_diff.go` validates that all node models exist in the governance registry before accepting updates.

Each `HardwareNode` contains:
- `local_id`: Unique identifier within participant
- `status`: Current operational status (INFERENCE, POC, TRAINING, STOPPED, FAILED)  
- `models`: Array of supported model identifiers
- `hardware`: GPU/CPU specifications
- `host`/`port`: Network configuration

### 3. Epoch-Based Model Snapshots

During epoch transitions, the system creates snapshots of hardware node configurations through `setModelsForParticipants` in `inference-chain/x/inference/module/model_assignment.go`. These snapshots are stored in **EpochGroupData** structures, organized hierarchically with parent groups containing all models and subgroups for specific models.

The broker populates epoch-specific data in `NodeState.EpochModels` and `NodeState.EpochMLNodes` through `UpdateNodeWithEpochData`. This ensures consistent model assignments throughout an epoch, even if hardware configurations change mid-epoch.

## Key Questions and Scenarios

### 1. MLNodes Without Governance Model Support

**Question**: What happens if an MLNode doesn't support any governance-approved models? Do we accept PoC batches from such nodes?

**Current Behavior**: The system handles this scenario through a multi-layered approach:

- **Model Assignment Phase**: In `model_assignment.go`, nodes without governance model support are excluded from model-specific epoch subgroups. The `supportedModelsByNode` function filters nodes that don't match any governance models, and such nodes receive empty `supportedModels` arrays.

- **PoC Participation**: Nodes without governance models can still participate in PoC operations. The `postGeneratedBatches` handler in `decentralized-api/internal/server/mlnode/post_generated_batches_handler.go` accepts PoC batches regardless of model support. However, these nodes won't be assigned to inference subgroups.

- **Legacy Weight Distribution**: The `distributeLegacyWeight` function in `model_assignment.go` handles PoC batches submitted without NodeId (legacy behavior), distributing PoC weight among nodes that don't support governance models.

**Implications**: Non-governance nodes contribute to network security through PoC mining but cannot serve inference requests, creating a two-tier system where governance compliance is required for inference revenue but not for PoC participation.

### 2. Model Update Timing and Epoch Recording

**Question**: When a node updates its supported models, it will be recorded to the epoch only later. When will the broker change which model is requested?

**Current Behavior**: Model updates follow a delayed activation pattern:

- **Immediate Broker Update**: When nodes update their model configurations, the broker immediately reflects these changes in local `Node.Models` maps and submits them to the blockchain via `SubmitHardwareDiff`.

- **Blockchain Recording**: The chain immediately stores updated hardware configurations in `HardwareNodes`, but these changes don't affect active inference routing.

- **Epoch Activation**: Model changes only become effective during the next epoch transition when `setModelsForParticipants` creates new epoch snapshots. The broker continues using `NodeState.EpochModels` for inference routing until the next epoch.

- **Broker Model Selection**: The `InferenceUpNodeCommand` in `node_worker_commands.go` prioritizes epoch-assigned models over broker-configured models. Model request changes occur only when new epoch data is populated via `UpdateNodeWithEpochData`.

**Timeline**: Model updates are immediate for PoC participation but delayed for inference participation until the next epoch boundary, ensuring consistency during active inference periods.

### 3. MLNode Disabling and On-Chain Recording

**Question**: When we disable an MLNode, do we record it on-chain in hardware nodes?

**Current Behavior**: Node disabling is handled through multiple mechanisms:

- **Administrative Disabling**: Nodes can be disabled through the admin API using `NodeAdminCommand` in `node_admin_commands.go`. This sets `NodeState.AdminState.Enabled = false` and updates the epoch when disabling takes effect.

- **Status Propagation**: Disabled nodes have their `IntendedStatus` set to `HardwareNodeStatus_STOPPED`. The reconciliation process ensures the physical node transitions to stopped state.

- **On-Chain Recording**: The `syncNodes` function includes disabled nodes in hardware diffs with `status = STOPPED`. The `SubmitHardwareDiff` message updates the on-chain `HardwareNode.status` field, creating a permanent record of the node's disabled state.

- **Epoch Exclusion**: Disabled nodes are excluded from epoch group formation through `ShouldBeOperational` checks in `NodeState`, preventing them from receiving inference assignments or PoC tasks.

**Persistence**: Node disabling is recorded both locally in broker state and persistently on-chain through hardware node status updates, ensuring consistent exclusion from network operations.

## Current System Behavior: Inference Execution and Punishment

### Inference Execution with Inaccessible APIs

**Current Behavior**: The system publishes inference start requests regardless of executor API accessibility:

- `GetRandomExecutor` in `query_get_random_executor.go` selects participants based on epoch group membership and PoC slot availability, without checking API health
- The inference request is published to the blockchain via `StartInference` before attempting to contact the executor
- In `post_chat_handler.go`, if the executor API is unreachable, the inference fails but the on-chain request remains active
- Failed requests result in inference expiration through `handleExpiredInference()`, which issues refunds and increments the executor's `MissedRequests` counter
- No fallback mechanism exists - each inference request is bound to a single selected executor

### Participant Punishment for Missed Inferences

**Current Behavior**: The system tracks missed inferences and applies financial penalties:

- **Tracking**: Each expired inference increments `executor.CurrentEpochStats.MissedRequests` in `handleExpiredInference()`
- **Downtime Slashing**: At epoch end, `CheckAndSlashForDowntime()` calculates missed request percentage and slashes collateral if it exceeds `DowntimeMissedPercentageThreshold` (default 5%)
- **Slash Amount**: Participants lose `SlashFractionDowntime` (default 10%) of their total collateral for excessive downtime
- **Validation Penalties**: Participants with invalid inference results have their `InvalidatedInferences` count increased and face potential status changes to `INVALID`
- **Status Calculation**: The `calculateStatus()` function uses z-score analysis and consecutive failure probability to determine if participants should be marked as `INVALID`

### Jailing and Exclusion Mechanisms

**Current Behavior**: The system implements a jailing mechanism through the collateral module:

- **Jailing Status**: Participants can be marked as jailed through `SetJailed()` in the collateral keeper, with status checked via `IsJailed()`
- **Reward Exclusion**: In `CalculateParticipantBitcoinRewards()`, participants with `ParticipantStatus_INVALID` are explicitly excluded from PoC weight calculations and receive zero `RewardCoins`
- **Work Exclusion**: Invalid participants are excluded from epoch formation and cannot receive inference assignments
- **Recovery**: Jailed participants can be released through `RemoveJailed()`, but the specific recovery conditions and processes are not automatically defined
- **Persistence**: Jailed status is stored on-chain and persists across epochs until explicitly removed

## Implementation References

**Core Files**:
- `decentralized-api/broker/broker.go` - Central node coordination and synchronization
- `inference-chain/x/inference/keeper/msg_server_submit_hardware_diff.go` - Hardware update validation
- `inference-chain/x/inference/module/model_assignment.go` - Epoch-based model assignment
- `decentralized-api/broker/node_worker_commands.go` - Model loading and inference commands
- `inference-chain/proto/inference/inference/hardware_node.proto` - Hardware node data structures
- `inference-chain/proto/inference/inference/epoch_group_data.proto` - Epoch snapshot structures

**Key Functions**:
- `Broker.syncNodes()` - Hardware synchronization
- `setModelsForParticipants()` - Epoch model assignment  
- `UpdateNodeWithEpochData()` - Epoch data population
- `InferenceUpNodeCommand.Execute()` - Model loading for inference
- `ShouldBeOperational()` - Node operational status checks

## Required Implementation Changes

### 1. Remove Legacy PoC Distribution and Enforce Model Compliance

**Problem**: The current system has two issues: (1) `distributeLegacyWeight()` distributes PoC weight from legacy batches (submitted without NodeId) to all hardware nodes regardless of governance model support, and (2) MLNodes without any supported governance models still receive PoC weight and are included in epoch snapshots, allowing non-compliant nodes to participate in consensus.

**Solution**: Eliminate the legacy PoC distribution mechanism entirely by removing `distributeLegacyWeight()` calls and legacy MLNode handling. Implement governance model validation during epoch formation to exclude non-compliant MLNodes from PoC weight distribution and epoch assignments. Add epoch snapshot validation in `SubmitPocBatch()` to reject batches from nodes not included in current epoch formation.

**Changes Required**:

**File**: `inference-chain/x/inference/module/model_assignment.go`
- **Function**: `setModelsForParticipants()`
- **Current Logic**: Calls `distributeLegacyWeight()` to distribute PoC weight from legacy batches to all hardware nodes
- **New Logic**: 
  1. Remove the call to `distributeLegacyWeight(originalMLNodes, hardwareNodes, preservedNodes[p.Index])`
  2. Remove any legacy MLNodes (nodes with empty NodeId) from `originalMLNodes` before processing
  3. Filter remaining MLNodes using `supportedModelsByNode()` to exclude nodes without governance model support
  4. Log exclusions: `ma.LogInfo("Excluding MLNode without governance model support", "node_id", mlNode.NodeId)`
- **Result**: Only governance-compliant MLNodes with valid NodeIds proceed to epoch assignment

**File**: `inference-chain/x/inference/module/model_assignment.go`
- **Function**: `distributeLegacyWeight()`
- **Current Logic**: Distributes PoC weight from legacy batches to all hardware nodes
- **New Logic**: 
  1. **Remove this entire function** - legacy PoC distribution is no longer supported
  2. All PoC batches must be submitted with valid NodeId
  3. This eliminates the ability for nodes without governance model support to receive distributed PoC weight

**File**: `inference-chain/x/inference/module/model_assignment.go`
- **Function**: `supportedModelsByNode()`
- **Current Logic**: Returns map[nodeId][]modelId for all nodes
- **Enhancement**: Ensure function returns empty arrays `[]string{}` for nodes without any governance model matches
- **Validation**: Add explicit check that returned model arrays are non-empty before considering node as governance-compliant

**File**: `inference-chain/x/inference/keeper/msg_server_submit_poc_batch.go`
- **Function**: `SubmitPocBatch()`
- **Current Logic**: Accepts all PoC batches with valid `NodeId`
- **New Logic**: 
  1. **Require NodeId**: Reject any batch where `msg.NodeId` is empty (eliminates legacy batch support)
  2. After initial validation, query current epoch MLNode snapshots using `GetCurrentEpochGroupData()`
  3. Iterate through all subgroup `ValidationWeights` to find `msg.NodeId` in any `ml_nodes` array
  4. Return `types.ErrNodeNotInEpoch` if NodeId not found in current epoch snapshots
  5. This ensures only governance-compliant, epoch-assigned nodes can submit PoC batches

### 2. Model Alternative System

**Problem**: The current `IsValidGovernanceModel()` function in `msg_server_submit_hardware_diff.go` performs exact string matching between MLNode model identifiers and governance model IDs. This prevents model evolution scenarios where "Qwen/Qwen2.5-7B-Instruct" might be replaced by "Qwen/Qwen2.5-7B-Instruct-v2" without breaking existing MLNode configurations.

**Solution**: Extend the Model protobuf structure with an `alternative_models` array field, then implement a transitive model resolution system. Create a `ResolveModelId()` function that maps node model identifiers to canonical governance model IDs through alternative chains. Update all model validation points (`IsValidGovernanceModel()`, `supportedModelsByNode()`, `SubmitHardwareDiff()`) to use this resolution system, allowing nodes configured with alternative models to participate in governance model assignments.

**Changes Required**:

**File**: `inference-chain/proto/inference/inference/model.proto`
- **Message**: `Model`
- **Addition**: Add `repeated string alternative_models = 8;` field after existing fields
- **Regeneration**: Run `make proto-gen` to update `model.pb.go` with new field
- **Purpose**: Store array of model identifiers that can fulfill this governance model requirement

**File**: `inference-chain/x/inference/keeper/model.go`
- **Function**: `IsValidGovernanceModel(ctx context.Context, modelId string) bool`
- **Current Logic**: Simple lookup using `GetModel(ctx, modelId)`
- **New Logic**: 
  1. Direct match check: `GetModel(ctx, modelId)` 
  2. If not found, iterate through all governance models using `GetAllModels(ctx)`
  3. For each governance model, check if `modelId` exists in its `alternative_models` array
  4. Return true if found in any alternative array
- **Add Function**: `ResolveModelId(ctx context.Context, nodeModelId string) (string, bool)` - returns the canonical governance model ID that this node model can fulfill

**File**: `inference-chain/x/inference/module/model_assignment.go`
- **Function**: `supportedModelsByNode()`
- **Current Logic**: Checks `slices.Contains(node.Models, model.Id)` for exact matches
- **New Logic**: 
  1. Check direct match: `slices.Contains(node.Models, model.Id)`
  2. Check alternative matches: iterate through `node.Models` and call `ResolveModelId()` for each
  3. If any node model resolves to current governance `model.Id`, include in supported models
- **Result**: Returns map[nodeId][]governanceModelId where governance models include those fulfilled by alternatives

**File**: `inference-chain/x/inference/keeper/msg_server_submit_hardware_diff.go`
- **Function**: `SubmitHardwareDiff()`
- **Current Logic**: `if !k.IsValidGovernanceModel(ctx, modelId) { return nil, types.ErrInvalidModel }`
- **New Logic**: Replace with enhanced `IsValidGovernanceModel()` that includes alternative resolution
- **No interface change**: Existing validation logic continues to work, now with expanded model acceptance

**File**: `inference-chain/x/inference/keeper/model.go`
- **Add Function**: `GetModelAlternatives(ctx context.Context, governanceModelId string) []string`
- **Purpose**: Return all alternative model IDs for a given governance model
- **Usage**: Support debugging and administrative queries about model relationships

### 3. Comprehensive Node Disabling

**Problem**: The current system has inconsistent node disabling behavior. While `AdminState.Enabled = false` in broker prevents local operations, disabled nodes are still included in on-chain epoch formation through `setModelsForParticipants()` and can receive PoC weight regardless of their operational status. Disabled nodes with `HardwareNode.status = STOPPED` or `FAILED` can still be assigned PoC weight during epoch formation, and nodes that remain inactive during PoC phases can still participate in consensus calculations.

**Solution**: Implement a comprehensive node status filtering system that operates at three levels: (1) Filter hardware nodes by status during epoch formation before any processing, (2) Cross-reference MLNode IDs with hardware node status during weight distribution to exclude disabled nodes, (3) Update broker synchronization to explicitly mark disabled nodes for removal in hardware diffs, and (4) Add hardware node status validation in PoC batch submission to reject batches from disabled nodes.

**Changes Required**:

**File**: `inference-chain/x/inference/module/model_assignment.go`
- **Function**: `setModelsForParticipants()`
- **Current Logic**: Processes all hardware nodes from `GetHardwareNodes(ctx, p.Index)` without status filtering
- **New Logic**: 
  1. After retrieving hardware nodes with `GetHardwareNodes(ctx, p.Index)`, filter hardware nodes
  2. Create `activeHardwareNodes` by excluding nodes where `node.Status == types.HardwareNodeStatus_STOPPED || node.Status == types.HardwareNodeStatus_FAILED`
  3. Use `activeHardwareNodes` instead of `hardwareNodes` for all subsequent processing
  4. Log exclusions: `ma.LogInfo("Excluding disabled hardware node from epoch", "participant", p.Index, "node_id", node.LocalId, "status", node.Status)`

**File**: `inference-chain/x/inference/module/model_assignment.go`
- **Function**: `distributeLegacyWeight()`
- **Current Logic**: Distributes weight among all `originalMLNodes` without status checks
- **New Logic**: 
  1. Before weight distribution, cross-reference MLNode IDs with hardware node status from `hardwareNodes.HardwareNodes`
  2. Skip weight distribution for MLNodes whose corresponding hardware node has `STOPPED` or `FAILED` status
  3. Recalculate total weight distribution excluding disabled nodes
  4. Log weight exclusions: `ma.LogInfo("Excluding disabled MLNode from PoC weight", "node_id", mlNode.NodeId, "hardware_status", hardwareNode.Status)`

**File**: `decentralized-api/broker/broker.go`
- **Function**: `syncNodes()`
- **Current Logic**: Submits all local nodes to chain via `calculateNodesDiff()` regardless of admin state
- **New Logic**: 
  1. Before calculating node differences with `calculateNodesDiff()`, filter local nodes based on admin state
  2. In `calculateNodesDiff()`, exclude nodes where `!nodeWithState.State.ShouldBeOperational(latestEpoch, currentPhase)`
  3. For disabled nodes, explicitly add them to `diff.Removed` to update on-chain status to `STOPPED`
  4. This ensures next epoch formation excludes disabled nodes

**File**: `inference-chain/x/inference/keeper/msg_server_submit_poc_batch.go`
- **Function**: `SubmitPocBatch()`
- **Enhancement**: After validating NodeId exists in epoch snapshots (from requirement #1), also verify the corresponding hardware node status is not `STOPPED` or `FAILED`
- **Implementation**: Query hardware nodes and check status before accepting PoC batch
- **Error**: Return `types.ErrNodeDisabled` for batches from disabled nodes

**Implementation Priority**:
1. **PoC Batch Validation** - Immediate security concern requiring strict enforcement
2. **Node Disabling** - Consistency issue affecting network participation
3. **Model Alternative System** - Feature enhancement for operational flexibility

**Validation Requirements**:
- All changes must maintain backward compatibility during transition periods
- Model alternative chains must prevent circular references
- Disabled node exclusion must be atomic across all network operations
