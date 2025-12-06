
import com.productscience.initCluster
import com.productscience.logSection
import com.productscience.data.StakeValidatorStatus
import org.assertj.core.api.Assertions.assertThat
import org.junit.jupiter.api.Test

// TODO: add to power tests file?
class PoCBatchFilterTest : TestermintTest() {
    @Test
    fun `poc batch filtering removes invalid nonces and reduces power`() {
        val (_, genesis) = initCluster(reboot = true)
        logSection("Setting ${genesis.name} PoC response with 3 invalid nonces out of 10")

        // Create a list of 10 dists: 7 valid (< 999), 3 invalid (> 999)
        // nonceDistCutoff is 999.0 in the chain code
        val validDist = List(7) { 0.5 } // 0.5 is valid
        val invalidDist = List(3) { 1000.0 } // 1000.0 is invalid (> 999.0)
        val customDist = validDist + invalidDist
        
        // Total weight 10, but customDist provided
        genesis.setPocResponse(weight = 10, customDist = customDist)
        
        genesis.waitForNextEpoch()
        genesis.node.waitForNextBlock(1)

        logSection("Verifying power is 7 instead of 10")
        val validator = genesis.node.getStakeValidator()
        
        assertThat(validator.tokens).isEqualTo(7)
        assertThat(validator.status).isEqualTo(StakeValidatorStatus.BONDED.value)
    }
}

