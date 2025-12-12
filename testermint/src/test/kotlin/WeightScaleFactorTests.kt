import com.productscience.*
import com.productscience.data.*
import com.productscience.assertions.assertThat
import org.assertj.core.api.Assertions.assertThat
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Timeout
import java.time.Instant
import java.util.concurrent.TimeUnit

@Timeout(value = 15, unit = TimeUnit.MINUTES)
class WeightScaleFactorTests : TestermintTest() {

    data class NodeState(
        val weight: Int,
        val isAllocated: Boolean
    )

    private fun collectState(participants: List<LocalInferencePair>): Map<String, Map<String, NodeState>> {
        return participants.associate { p ->
            p.name to p.api.getNodes().associate { n ->
                val weight = n.state.epochMlNodes?.values?.firstOrNull()?.pocWeight ?: 0
                val isAllocated = n.state.epochMlNodes
                    ?.any { (_, value) -> value.timeslotAllocation.getOrNull(1) == true }
                    ?: false
                n.node.id to NodeState(weight, isAllocated)
            }
        }
    }

    @Test
    fun `test weight scale factor updates`() {
        // 1. Configure the network with 3 participants, each having 3 mlnodes
        // Initial config: weight_scale_factor = 1
        val initialWeightSpec = spec {
            this[AppState::inference] = spec<InferenceState> {
                this[InferenceState::params] = spec<InferenceParams> {
                    this[InferenceParams::pocParams] = spec<PocParams> {
                        this[PocParams::weightScaleFactor] = Decimal.fromDouble(1.0)
                    }
                }
            }
        }

        val config = inferenceConfig.copy(
            genesisSpec = inferenceConfig.genesisSpec?.merge(initialWeightSpec) ?: initialWeightSpec
        )

        val (cluster, genesis) = initCluster(config = config, reboot = true, resetMlNodes = false)

        // Ensure 3 nodes per participant (default is 1, so add 2)
        logSection("Adding ML Nodes to reach 3 per participant")
        genesis.addNodes(2)
        cluster.joinPairs.forEach { it.addNodes(2) }

        // Wait for nodes to be registered and active
        genesis.waitForNextEpoch()
        
        // 2. Expect some nodes to be allocated (verify setup)
        logSection("Verifying Allocation")
        genesis.waitForStage(EpochStage.SET_NEW_VALIDATORS) // Wait for allocation to happen
        
        val allParticipants = listOf(genesis) + cluster.joinPairs
        
        // Check if we have nodes allocated
        var allocatedNodesCount = 0
        allParticipants.forEach { participant ->
            val nodes = participant.api.getNodes()
            assertThat(nodes).hasSize(3)
            nodes.forEach { node ->
                 // timeslotAllocation index 1 corresponds to inference slot usually (0 is training?)
                 // Need to confirm index. SchedulingTests uses index 1.
                 val isAllocated = node.state.epochMlNodes
                    ?.any { (_, value) -> value.timeslotAllocation.getOrNull(1) == true }
                    ?: false
                 if (isAllocated) allocatedNodesCount++
            }
        }
        assertThat(allocatedNodesCount).isGreaterThan(0).`as`("Some nodes should be allocated for inference")

        assertThat(allocatedNodesCount).isGreaterThan(0).`as`("Some nodes should be allocated for inference")

        // 3. Initial weight scale factor is 1. (Already configured via spec)
        val initialParams = genesis.getParams()
        val initialScale = initialParams.pocParams.weightScaleFactor
        assertThat(initialScale?.toDouble() ?: 1.0).isEqualTo(1.0)

        // Capture initial state (Epoch N)
        val stateHistory = mutableMapOf<Long, Map<String, Map<String, NodeState>>>()
        
        val initialEpoch = genesis.getEpochData().latestEpoch.index
        val initialState = collectState(allParticipants)
        stateHistory[initialEpoch] = initialState
        logSection("Initial State (Epoch $initialEpoch): $initialState")

        // 4. Change weight_scale_factor to 2.5 via gov proposal
        val currentGovParams = genesis.node.getGovParams().params
        logSection("Current Gov Params: $currentGovParams")

        val modifiedParams = initialParams.copy(
            pocParams = initialParams.pocParams.copy(
                weightScaleFactor = Decimal.fromDouble(2.5)
            )
        )

        logSection("Submitting Proposal to change weight_scale_factor to 2.5")

        // Use standard runProposal helper
        genesis.runProposal(cluster, UpdateParams(params = modifiedParams))
        
        logSection("Proposal Passed")
        
        // 5. Collect history for a few epochs to observe the change
        // We wait for SET_NEW_VALIDATORS of subsequent epochs
        for (i in 1..3) {
             genesis.waitForStage(EpochStage.SET_NEW_VALIDATORS)
             val currentEpoch = genesis.getEpochData().latestEpoch.index
             val currentState = collectState(allParticipants)
             stateHistory[currentEpoch] = currentState
             logSection("State at Epoch $currentEpoch: $currentState")
        }
        
        // 6. Verification
        // We need to find the transition where weights scaled.
        // The weight in Epoch N is determined by allocation in Epoch N-1 and params active at calculation time.
        
        var transitionFound = false
        
        val sortedEpochs = stateHistory.keys.sorted()
        for (i in 1 until sortedEpochs.size) {
            val prevEpoch = sortedEpochs[i-1]
            val currEpoch = sortedEpochs[i]
            
            val prevState = stateHistory[prevEpoch]!!
            val currState = stateHistory[currEpoch]!!
            
            // Check if we see scaling behavior in this transition
            // We check one node that was NOT allocated in prevEpoch. If it scaled ~2.5x, we found the transition.
            
            val candidateNode = allParticipants.firstNotNullOfOrNull { p ->
                prevState[p.name]?.entries?.find { (_, s) -> !s.isAllocated && s.weight > 0 }
            }
            
            if (candidateNode != null) {
                val (nodeId, prevNodeState) = candidateNode
                // Find this node in currState (need to find which participant it belongs to, or flatten map)
                val currNodeState = currState.values.firstNotNullOfOrNull { it[nodeId] }
                
                if (currNodeState != null) {
                    val ratio = currNodeState.weight.toDouble() / prevNodeState.weight.toDouble()
                    if (ratio > 2.0) { // Expecting 2.5
                        logSection("Found weight scaling transition between Epoch $prevEpoch and $currEpoch (Ratio: $ratio)")
                        verifyScaling(prevState, currState, 2.5)
                        transitionFound = true
                        break
                    }
                }
            }
        }
        
        assertThat(transitionFound).isTrue().`as`("Should have found a transition where weights scaled by 2.5")
    }
    
    private fun verifyScaling(
        prevState: Map<String, Map<String, NodeState>>, 
        currState: Map<String, Map<String, NodeState>>, 
        scaleFactor: Double
    ) {
        prevState.forEach { (participantName, participantNodes) ->
            val currParticipantNodes = currState[participantName]!!
            
            participantNodes.forEach { (nodeId, prevNodeState) ->
                val currNodeState = currParticipantNodes[nodeId]!!
                
                logSection("Verifying Node $nodeId: PrevAlloc=${prevNodeState.isAllocated}, PrevWeight=${prevNodeState.weight}, CurrWeight=${currNodeState.weight}")
                
                if (prevNodeState.weight == 0) return@forEach // Skip zero weights

                if (prevNodeState.isAllocated) {
                    // Preserved nodes should keep their weight
                    // Allowing 10% tolerance as weights might fluctuate slightly due to other factors?
                    // Or strictly equal? Let's use 5%.
                    assertThat(currNodeState.weight.toDouble())
                        .isCloseTo(prevNodeState.weight.toDouble(), org.assertj.core.data.Percentage.withPercentage(5.0))
                        .`as`("Allocated node $nodeId should keep weight")
                } else {
                    // PoC nodes should scale
                    val expectedWeight = prevNodeState.weight * scaleFactor
                    assertThat(currNodeState.weight.toDouble())
                         .isCloseTo(expectedWeight, org.assertj.core.data.Percentage.withPercentage(5.0))
                         .`as`("PoC node $nodeId should scale weight by $scaleFactor")
                }
            }
        }
    }
}

