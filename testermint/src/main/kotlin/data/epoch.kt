package com.productscience.data

import com.google.gson.annotations.SerializedName
import java.util.Locale

data class EpochResponse(
    @SerializedName("block_height")
    val blockHeight: Long,
    @SerializedName("latest_epoch")
    val latestEpoch: LatestEpochDto,
    val phase: Any,  // Changed from EpochPhase to Any to handle both String and enum
    @SerializedName("epoch_stages")
    val epochStages: EpochStages,
    @SerializedName("next_epoch_stages")
    val nextEpochStages: EpochStages,
    @SerializedName("epoch_params")
    val epochParams: EpochParams,
    @SerializedName("is_confirmation_poc_active")
    val isConfirmationPocActive: Boolean = false,
    @SerializedName("active_confirmation_poc_event")
    val activeConfirmationPocEvent: ConfirmationPoCEvent? = null
) {
    // Helper function to get phase as enum, handling Int, Double, String, and enum values
    fun getPhaseAsEnum(): EpochPhase {
        return when (phase) {
            is EpochPhase -> phase
            is Int -> EpochPhase.values().find { it.value == phase } ?: EpochPhase.Inference
            is Double -> EpochPhase.values().find { it.value == phase.toInt() } ?: EpochPhase.Inference
            is Float -> EpochPhase.values().find { it.value == phase.toInt() } ?: EpochPhase.Inference
            is Number -> EpochPhase.values().find { it.value == phase.toInt() } ?: EpochPhase.Inference
            is String -> {
                val normalized = phase.trim().uppercase(Locale.US)
                normalized.toIntOrNull()?.let { value ->
                    return EpochPhase.values().find { it.value == value } ?: EpochPhase.Inference
                }

                return when {
                    normalized.contains("POC_GENERATE_WIND_DOWN") ||
                        normalized.contains("POC_GENERATION_WIND_DOWN") -> EpochPhase.PoCGenerateWindDown
                    normalized.contains("POC_VALIDATE_WIND_DOWN") ||
                        normalized.contains("POC_VALIDATION_WIND_DOWN") -> EpochPhase.PoCValidateWindDown
                    normalized.contains("POC_VALIDATE") ||
                        normalized.contains("POC_VALIDATION") -> EpochPhase.PoCValidate
                    normalized.contains("POC_GENERATE") ||
                        normalized.contains("POC_GENERATION") -> EpochPhase.PoCGenerate
                    normalized.contains("INFERENCE") -> EpochPhase.Inference
                    else -> EpochPhase.Inference // Default fallback
                }
            }
            else -> EpochPhase.Inference
        }
    }

    val safeForInference: Boolean =
        if (getPhaseAsEnum() == EpochPhase.Inference) {
            val blocksUntilEnd = nextEpochStages.pocStart - blockHeight
            blocksUntilEnd > 3
        } else {
            false
        }
}

data class LatestEpochDto(
    val index: Long,
    @SerializedName("poc_start_block_height")
    val pocStartBlockHeight: Long
)

enum class EpochPhase(val value: Int) {
    PoCGenerate(0),
    PoCGenerateWindDown(1),
    PoCValidate(2),
    PoCValidateWindDown(3),
    Inference(4)
}

data class EpochStages(
    @SerializedName("epoch_index")
    val epochIndex: Long,
    @SerializedName("poc_start")
    val pocStart: Long,
    @SerializedName("poc_generation_wind_down")
    val pocGenerationWindDown: Long,
    @SerializedName("poc_generation_end")
    val pocGenerationEnd: Long,
    @SerializedName("poc_validation_start")
    val pocValidationStart: Long,
    @SerializedName("poc_validation_wind_down")
    val pocValidationWindDown: Long,
    @SerializedName("poc_validation_end")
    val pocValidationEnd: Long,
    @SerializedName("set_new_validators")
    val setNewValidators: Long,
    @SerializedName("claim_money")
    val claimMoney: Long,
    @SerializedName("next_poc_start")
    val nextPocStart: Long,
    @SerializedName("poc_exchange_window")
    val pocExchangeWindow: EpochExchangeWindow,
    @SerializedName("poc_validation_exchange_window")
    val pocValExchangeWindow: EpochExchangeWindow
)

data class EpochExchangeWindow(
    val start: Long,
    val end: Long
)

data class ConfirmationPoCEvent(
    @SerializedName("epoch_index")
    val epochIndex: Long,
    @SerializedName("event_sequence")
    val eventSequence: Long,
    @SerializedName("trigger_height")
    val triggerHeight: Long,
    @SerializedName("generation_start_height")
    val generationStartHeight: Long,
    val phase: ConfirmationPoCPhase,
    @SerializedName("poc_seed_block_hash")
    val pocSeedBlockHash: String = ""
)

enum class ConfirmationPoCPhase(val value: Int) {
    CONFIRMATION_POC_INACTIVE(0),
    CONFIRMATION_POC_GRACE_PERIOD(1),
    CONFIRMATION_POC_GENERATION(2),
    CONFIRMATION_POC_VALIDATION(3),
    CONFIRMATION_POC_COMPLETED(4)
}
