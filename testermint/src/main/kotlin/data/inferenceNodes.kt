package com.productscience.data

import java.util.Locale

data class NodeResponse(val node: InferenceNode, val state: NodeState)

data class InferenceNode(
    val host: String,
    val inferenceSegment: String = "",
    val inferencePort: Int,
    val pocSegment: String = "",
    val pocPort: Int,
    val models: Map<String, ModelConfig>,
    val id: String,
    val maxConcurrent: Int,
    val nodeNum: Long? = null,
    val hardware: List<Hardware>? = null,
    val version: String? = null,
)

data class Hardware(
    val type: String,
    val count: Int
)

data class NodeState(
    val intendedStatus: Any?,
    val currentStatus: Any?,
    val pocIntendedStatus: Any?,
    val pocCurrentStatus: Any?,
    val lockCount: Int,
    val failureReason: String,
    val statusTimestamp: String,
    val adminState: AdminState? = null,
    val epochModels: Map<String, EpochModel>?,
    val epochMlNodes: Map<String, EpochMlNode>?,
)

fun NodeState.normalized(): NodeState {
    return copy(
        intendedStatus = normalizeNodeStatus(intendedStatus),
        currentStatus = normalizeNodeStatus(currentStatus),
        pocIntendedStatus = normalizeNodeStatus(pocIntendedStatus),
        pocCurrentStatus = normalizeNodeStatus(pocCurrentStatus),
    )
}

fun NodeState.normalizedCurrentStatus(): String = normalizeNodeStatus(currentStatus)

fun NodeState.normalizedIntendedStatus(): String = normalizeNodeStatus(intendedStatus)

private fun normalizeNodeStatus(status: Any?): String {
    when (status) {
        null -> return "UNKNOWN"
        is NodeState -> return normalizeNodeStatus(status.currentStatus)
        is Number -> {
            val numeric = status.toInt()
            return when (numeric) {
                0 -> "UNKNOWN"
                1 -> "INFERENCE"
                2 -> "POC"
                3 -> "MAINTENANCE"
                else -> numeric.toString()
            }
        }
        is Boolean -> return if (status) "INFERENCE" else "UNKNOWN"
    }

    val upper = status.toString().trim().uppercase(Locale.US)
    if (upper.isBlank()) return "UNKNOWN"

    val cleaned = sequenceOf(
        "INFERENCE_NODE_STATUS_",
        "INFERENCE_NODE_STATE_",
        "NODE_STATUS_",
        "NODE_STATE_",
        "STATUS_",
        "STATE_",
        "ENUM_",
    ).fold(upper) { acc, prefix ->
        if (acc.startsWith(prefix)) acc.removePrefix(prefix) else acc
    }.replace('-', '_')

    if (cleaned.isBlank()) return "UNKNOWN"

    val terminalToken = cleaned.substringAfterLast('_', cleaned)

    return when (terminalToken) {
        "0", "UNKNOWN", "UNSPECIFIED" -> "UNKNOWN"
        "1", "INFERENCE" -> "INFERENCE"
        "2", "POC" -> "POC"
        "TRAINING" -> "TRAINING"
        "STOPPED" -> "STOPPED"
        "FAILED" -> "FAILED"
        else -> when {
            cleaned.contains("INFERENCE") -> "INFERENCE"
            cleaned.contains("POC") -> "POC"
            cleaned.contains("TRAIN") -> "TRAINING"
            cleaned.contains("STOP") -> "STOPPED"
            cleaned.contains("FAIL") -> "FAILED"
            else -> cleaned
        }
    }
}

data class AdminState(
    val enabled: Boolean,
    val epoch: Long
)

data class ModelConfig(
    val args: List<String>
)

data class EpochModel(
    val proposedBy: String,
    val id: String,
    val unitsOfComputePerToken: Long,
    val hfRepo: String,
    val hfCommit: String,
    val modelArgs: List<String>,
    val vRam: Int,
    val throughputPerNonce: Long
)

data class EpochMlNode(
    val nodeId: String,
    val pocWeight: Int,
    val timeslotAllocation: List<Boolean>
)

data class NodeAdminStateResponse(
    val message: String,
    val nodeId: String
)
