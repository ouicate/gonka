import com.productscience.*
import com.productscience.data.*
import com.productscience.assertions.assertThat
import org.assertj.core.api.Assertions.assertThat
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Timeout
import org.tinylog.kotlin.Logger
import java.util.concurrent.TimeUnit

@Timeout(value = 15, unit = TimeUnit.MINUTES)
class NodeDisableInferenceTests : TestermintTest() {

    @Test
    fun `test node disable inference default state`() {
        // 1. Setup genesis with 2 ML nodes
        val config = inferenceConfig.copy(
            additionalDockerFilesByKeyName = mapOf(
                GENESIS_KEY_NAME to listOf("docker-compose-local-mock-node-2.yml")
            ),
            nodeConfigFileByKeyName = mapOf(
                GENESIS_KEY_NAME to "node_payload_mock-server_genesis_2_nodes.json"
            ),
        )
        // We need 3 participants: Genesis + 2 Joiners (default initCluster provides Genesis + 2 Joiners)
        val (cluster, genesis) = initCluster(config = config, reboot = true, resetMlNodes = false)

        // 2. Verify active participants and Genesis ML nodes
        genesis.waitForStage(EpochStage.SET_NEW_VALIDATORS)
        val participants = genesis.api.getActiveParticipants().activeParticipants
        assertThat(participants).hasSize(3)

        val genesisParticipant = participants.getParticipant(genesis)
        assertThat(genesisParticipant).isNotNull
        // Genesis should have 2 ML nodes (implied by configuration)
        // We can check this via getNodes() as well
        
        val nodes = genesis.api.getNodes()
        val genesisNodeInfo = nodes.first { it.node.id == genesis.node.id }
        // The node state might not directly list 'mlNodes' count in 'activeParticipants' object structure depending on deserialization
        // But let's check 'mlNodes' property of ActiveParticipant if available.
        // Based on participants.kt, ActiveParticipant has 'mlNodes: List<MlNodes>' where MlNodes wraps List<MlNode>
        assertThat(genesisParticipant?.mlNodes?.firstOrNull()?.mlNodes).hasSize(2)

        logSection("Verifying inference allocation")
        // Check that one of genesis's ML nodes is allocated to serve inference
        // Allocation is usually visible in node state or via active participants epoch info
        // SchedulingTests.kt checks node.state.epochMlNodes
        
        val genesisAllocated = genesisNodeInfo.state.epochMlNodes?.any { (_, mlNodeState) ->
            // Check timeslot allocation for inference (usually index 1 or similar, depending on logic)
            // SchedulingTests uses: x.timeslotAllocation.getOrNull(1) == true
            mlNodeState.timeslotAllocation.getOrNull(1) == true
        } ?: false
        
        assertThat(genesisAllocated).isTrue()
            .`as`("One of Genesis ML nodes should be allocated for inference")

        // 3. Wait for INFERENCE phase and disable join-1
        logSection("Waiting for Inference Window")
        genesis.waitForNextInferenceWindow()
        
        val join1 = cluster.joinPairs[0]
        logSection("Disabling join-1: ${join1.node.id}")
        val disableResponse = genesis.api.disableNode(join1.node.id)
        assertThat(disableResponse.nodeId).isEqualTo(join1.node.id)

        // 4. Wait for beginning of PoC stage and make ~15 inference requests
        logSection("Waiting for PoC start")
        genesis.waitForStage(EpochStage.START_OF_POC)
        
        logSection("Sending 15 inference requests")
        val requests = 15
        // Assuming runParallelInferencesWithResults is available and imports are correct
        val inferences = runParallelInferencesWithResults(
            genesis, 
            count = requests, 
            maxConcurrentRequests = 5
        )
        
        assertThat(inferences).hasSize(requests)
        assertThat(inferences).allMatch { 
            it.status == InferenceStatus.VALIDATED.value || it.status == InferenceStatus.FINISHED.value 
        }
        logSection("All 15 inferences succeeded")

        // 5. Wait for end of PoC and check if join-1 could claim rewards
        logSection("Waiting for End of PoC (New Validators)")
        genesis.waitForStage(EpochStage.SET_NEW_VALIDATORS, offset = 3)
        
        // Try to claim rewards for join-1
        logSection("Attempting to claim rewards for join-1")
        val currentEpoch = genesis.getEpochData().epochIndex
        // We want to claim for the epoch where PoC just happened? 
        // Usually rewards are claimed for previous epochs. 
        // initCluster puts us at some epoch. We waited for stages.
        // Let's try to claim for the current epoch - 1 or use the seed logic.
        
        val seed = join1.api.getConfig().currentSeed
        val claimMsg = MsgClaimRewards(
            creator = join1.node.getColdAddress(),
            seed = seed.seed,
            epochIndex = seed.epochIndex,
        )
        
        val initialBalance = join1.node.getSelfBalance()
        logSection("Join-1 Balance before claim: $initialBalance")
        
        val claimResponse = join1.submitMessage(claimMsg)
        assertThat(claimResponse).isSuccess()
        
        val finalBalance = join1.node.getSelfBalance()
        logSection("Join-1 Balance after claim: $finalBalance")
        
        // If join-1 was disabled during inference/PoC, did it participate?
        // The disable happens in Inference window. 
        // If it was disabled, it might not have participated in validation or PoC?
        // However, the test goal says "nodes have mlnodes in inference state by default, even if they are turned off from the network"
        // If the ML node was still active (by default?), maybe it gets rewards?
        // Or maybe the test just checks "could claim", implying the tx succeeds.
        
        // If the balance increases, it got rewards.
        if (finalBalance > initialBalance) {
            Logger.info("Join-1 successfully claimed rewards.")
        } else {
            Logger.info("Join-1 claimed but no rewards received (or 0).")
        }
    }
}

