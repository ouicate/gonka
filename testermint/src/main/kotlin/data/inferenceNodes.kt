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
    val intendedStatus: String,
    val currentStatus: String,
    val pocIntendedStatus: String,
    val pocCurrentStatus: String,
    val lockCount: Int,
    val failureReason: String,
    val statusTimestamp: String,
    val adminState: AdminState? = null,
    val epochModels: Map<String, EpochModel>?,
    val epochMlNodes: Map<String, EpochMlNode>?,
)

fun NodeState.normalized(): NodeState =
    copy(
        intendedStatus = normalizeNodeStatus(intendedStatus),
        currentStatus = normalizeNodeStatus(currentStatus),
        pocIntendedStatus = normalizeNodeStatus(pocIntendedStatus),
        pocCurrentStatus = normalizeNodeStatus(pocCurrentStatus),
    )

fun NodeState.normalizedCurrentStatus(): String = normalizeNodeStatus(currentStatus)

fun NodeState.normalizedIntendedStatus(): String = normalizeNodeStatus(intendedStatus)

private fun normalizeNodeStatus(status: String?): String {
    if (status.isNullOrBlank()) return "UNKNOWN"
    val upper = status.trim().uppercase(Locale.US)
    val cleaned = sequenceOf(
        "INFERENCE_NODE_STATUS_",
        "INFERENCE_NODE_STATE_",
        "NODE_STATUS_",
        "NODE_STATE_",
        "STATUS_",
        "STATE_",
    ).fold(upper) { acc, prefix ->
        if (acc.startsWith(prefix)) acc.removePrefix(prefix) else acc
    }
    val finalValue = cleaned.replace('-', '_')
    return when (finalValue) {
        "0", "UNKNOWN", "UNSPECIFIED", "" -> "UNKNOWN"
        "1", "INFERENCE", "READY", "ACTIVE" -> "INFERENCE"
        "2", "POC" -> "POC"
        "3", "MAINTENANCE" -> "MAINTENANCE"
        else -> finalValue
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
