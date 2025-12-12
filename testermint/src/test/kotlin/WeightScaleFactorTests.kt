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

    private fun collectWeights(participants: List<LocalInferencePair>): Map<String, Map<String, Int>> {
        return participants.associate { p ->
            p.name to p.api.getNodes().associate { n ->
                val weight = n.state.epochMlNodes?.values?.firstOrNull()?.pocWeight ?: 0
                n.node.id to weight
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

        // 3. Initial weight scale factor is 1. (Already configured via spec)
        val initialParams = genesis.getParams()
        // Assuming the param is correctly propagated and readable. 
        // Note: nullable Decimal in AppExport, so check for null or 1.0
        val initialScale = initialParams.pocParams.weightScaleFactor
        assertThat(initialScale?.toDouble() ?: 1.0).isEqualTo(1.0)

        // Capture initial weights
        val initialWeights = collectWeights(allParticipants)
        logSection("Initial Weights Captured: $initialWeights")

        // 4. Change weight_scale_factor to 2.5 via gov proposal
        logSection("Submitting Proposal to change weight_scale_factor to 2.5")
        
        val modifiedParams = initialParams.copy(
            pocParams = initialParams.pocParams.copy(
                weightScaleFactor = Decimal.fromDouble(2.5)
            )
        )
        
        // Custom proposal flow to wait for End of PoC
        val proposalId = genesis.submitGovernanceProposal(
             GovernanceProposal(
                metadata = "http://example.com",
                deposit = "1000${inferenceConfig.denom}", // Assumes min deposit is 1000
                title = "Update Weight Scale Factor",
                summary = "Update weight scale factor to 2.5",
                expedited = false,
                messages = listOf(UpdateParams(params = modifiedParams))
            )
        ).also {
             if (it.code != 0) throw RuntimeException("Proposal failed: ${it.rawLog}")
        }.getProposalId()!!
        
        // Fund it
        genesis.makeGovernanceDeposit(proposalId, 1000).also {
            require(it.code == 0) { "Deposit failed: ${it.rawLog}" }
        }

        // Wait a tiny bit (3-5 seconds) so the thing is actually on chain and proceed to vote
        logSection("Waiting for 5 seconds before voting")
        Thread.sleep(5000)

        // Capture allocation status during this epoch (before we advance)
        val allocationStatus = allParticipants.associate { p ->
             p.name to p.api.getNodes().associate { node ->
                 val isAllocated = node.state.epochMlNodes
                    ?.any { (_, value) -> value.timeslotAllocation.getOrNull(1) == true }
                    ?: false
                 node.node.id to isAllocated
             }
        }
        logSection("Allocation Status: $allocationStatus")

        // Query and log gov params before voting
        val currentGovParams = genesis.node.getGovParams().params
        logSection("Current Gov Params: $currentGovParams")

        // Vote with all participants
        logSection("Voting Yes")
        allParticipants.forEach {
            it.voteOnProposal(proposalId, "yes").also { resp ->
                require(resp.code == 0) { "Vote failed: ${resp.rawLog}" }
            }
        }

        // Wait for voting period to end and proposal to pass
        val govParams = genesis.node.getGovParams().params
        val votingPeriodEnd = Instant.now().plus(govParams.votingPeriod) // Approximation, better to read proposal submit time
        // But since we just submitted, this is close enough or we can wait for status change.
        
        // Wait until proposal is passed or voting period ends
        // We can just sleep or check proposal status loop
        logSection("Waiting for proposal to pass")
        while (true) {
            val proposal = genesis.node.getGovernanceProposals().proposals.first { it.id == proposalId }
            if (proposal.status == 3) { // 3 = PROPOSAL_STATUS_PASSED
                logSection("Proposal Passed")
                break
            }
            if (proposal.status == 4) { // 4 = PROPOSAL_STATUS_REJECTED
                throw RuntimeException("Proposal Rejected")
            }
            Thread.sleep(1000)
        }
        
        // Log weights at each epoch for a few epochs
        val weightHistory = mutableListOf<Map<String, Map<String, Int>>>()
        weightHistory.add(initialWeights)

        // Wait for next epoch for weights to update (and maybe one more to be safe/observe history)
        // We need to check if the change happened.
        // Let's loop for 2 epochs.
        for (i in 1..2) {
             genesis.waitForStage(EpochStage.SET_NEW_VALIDATORS)
             val currentWeights = collectWeights(allParticipants)
             val epochIndex = genesis.getEpochData().latestEpoch.index
             logSection("Epoch $epochIndex Weights: $currentWeights")
             weightHistory.add(currentWeights)
        }
        
        // 5. Observe weight changes - use the last collected weights (should be after update)
        val newWeights = weightHistory.last()
        logSection("New Weights (Last Epoch): $newWeights")
        
        
        allParticipants.forEach { participant ->
            val key = participant.name
            val participantInitialWeights = initialWeights[key]!!
            val participantNewWeights = newWeights[key]!!
            val participantAllocation = allocationStatus[key]!!
            
            participant.api.getNodes().forEach { node ->
                val nodeId = node.node.id
                val initialWeight = participantInitialWeights[nodeId] ?: 0
                val newWeight = participantNewWeights[nodeId] ?: 0
                val wasAllocated = participantAllocation[nodeId] ?: false
                
                logSection("Node $nodeId: Initial=$initialWeight, New=$newWeight, Allocated=$wasAllocated")
                
                if (wasAllocated) {
                    // Preserved nodes should keep their weight (approx)
                    // Allowing some small variation if any, but ideally equal?
                    // "keep their weight"
                    assertThat(newWeight.toDouble()).isCloseTo(initialWeight.toDouble(), org.assertj.core.data.Percentage.withPercentage(5.0))
                        .`as`("Allocated node $nodeId should keep weight")
                } else {
                    // PoC participants should scale by 2.5
                    // Note: Initial weight might be different from base weight if they had history?
                    // But assuming steady state, weight * 2.5.
                    val expectedWeight = initialWeight * 2.5
                    assertThat(newWeight.toDouble()).isCloseTo(expectedWeight, org.assertj.core.data.Percentage.withPercentage(5.0))
                         .`as`("PoC node $nodeId should scale weight by 2.5")
                }
            }
        }
    }
}

