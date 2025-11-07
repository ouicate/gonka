package com.productscience.mockserver

import com.productscience.mockserver.routes.configRoutes
import com.productscience.mockserver.routes.fileRoutes
import com.productscience.mockserver.routes.handlePowWebSocket
import com.productscience.mockserver.routes.healthRoutes
import com.productscience.mockserver.routes.inferenceRoutes
import com.productscience.mockserver.routes.powRoutes
import com.productscience.mockserver.routes.responseRoutes
import com.productscience.mockserver.routes.stateRoutes
import com.productscience.mockserver.routes.stopRoutes
import com.productscience.mockserver.routes.tokenizationRoutes
import com.productscience.mockserver.routes.trainRoutes
import com.productscience.mockserver.service.ResponseService
import com.productscience.mockserver.service.TokenizationService
import com.productscience.mockserver.service.WebhookService
import com.productscience.mockserver.service.WebSocketManager
import io.ktor.serialization.jackson.jackson
import io.ktor.server.application.Application
import io.ktor.server.application.call
import io.ktor.server.application.install
import io.ktor.server.engine.embeddedServer
import io.ktor.server.netty.Netty
import io.ktor.server.plugins.callloging.CallLogging
import io.ktor.server.plugins.contentnegotiation.ContentNegotiation
import io.ktor.server.request.httpMethod
import io.ktor.server.request.path
import io.ktor.server.response.respond
import io.ktor.server.routing.get
import io.ktor.server.routing.routing
import io.ktor.server.websocket.WebSockets
import io.ktor.server.websocket.pingPeriod
import io.ktor.server.websocket.timeout
import io.ktor.server.websocket.webSocket
import io.ktor.util.AttributeKey
import org.slf4j.LoggerFactory
import org.slf4j.event.Level
import java.time.Duration

// Define keys for services
val WebhookServiceKey = AttributeKey<WebhookService>("WebhookService")
val ResponseServiceKey = AttributeKey<ResponseService>("ResponseService")
val TokenizationServiceKey = AttributeKey<TokenizationService>("TokenizationService")
val WebSocketManagerKey = AttributeKey<WebSocketManager>("WebSocketManager")

fun main() {
    embeddedServer(
        Netty,
        port = 8080,
        host = "0.0.0.0",
        configure = {
            // Configure Netty with more worker threads to handle concurrent requests
            // This prevents WebSocket operations from blocking HTTP request handling
            connectionGroupSize = 2
            workerGroupSize = 8
            callGroupSize = 16
        },
        module = Application::module
    ).start(wait = true)
}

fun Application.module() {
    configureLogging()
    configureSerialization()
    configureWebSockets()
    configureServices()
    configureRouting()
}

fun Application.configureLogging() {
    install(CallLogging) {
        level = Level.DEBUG
        filter { _ -> true } // Log all requests
        format { call ->
            val status = call.response.status()
            val httpMethod = call.request.httpMethod.value
            val path = call.request.path()
            "Request: $httpMethod $path, Status: $status"
        }
    }
}

fun Application.configureServices() {
    // Create single instances of services to be used by all routes
    val responseService = ResponseService()
    val wsManager = WebSocketManager()
    val webhookService = WebhookService(responseService, wsManager)
    val tokenizationService = TokenizationService()

    // Register the services in the application's attributes
    attributes.put(WebhookServiceKey, webhookService)
    attributes.put(ResponseServiceKey, responseService)
    attributes.put(TokenizationServiceKey, tokenizationService)
    attributes.put(WebSocketManagerKey, wsManager)
}

fun Application.configureRouting() {
    // Get the services from the application's attributes
    val webhookService = attributes[WebhookServiceKey]
    val responseService = attributes[ResponseServiceKey]
    val tokenizationService = attributes[TokenizationServiceKey]
    val wsManager = attributes[WebSocketManagerKey]

    routing {
        // Server status endpoint
        get("/status") {
            call.respond(
                mapOf(
                    "status" to "ok",
                    "version" to "1.0.1",
                    "timestamp" to System.currentTimeMillis()
                )
            )
        }

        // Register all HTTP route handlers first
        // This ensures HTTP requests are matched before WebSocket upgrade checks
        stateRoutes()
        powRoutes(webhookService, wsManager)
        inferenceRoutes(responseService)
        trainRoutes()
        stopRoutes()
        healthRoutes()
        responseRoutes(responseService)
        tokenizationRoutes(tokenizationService)
        configRoutes(webhookService)
        fileRoutes() // Route for serving files
        
        // WebSocket endpoint for PoC - registered last to avoid interfering with HTTP routes
        // Note: WebSocket routes only match actual WebSocket upgrade requests, not regular HTTP requests
        webSocket("/api/v1/pow/ws") {
            handlePowWebSocket(this, wsManager)
        }
    }
}

fun Application.configureSerialization() {
    install(ContentNegotiation) {
        jackson()
    }
}

fun Application.configureWebSockets() {
    install(WebSockets) {
        pingPeriod = Duration.ofSeconds(30)  // Increased to reduce network overhead
        timeout = Duration.ofSeconds(60)      // More lenient timeout to prevent premature disconnects
        maxFrameSize = 1024 * 1024           // 1MB - sufficient for PoC batches
        masking = false                       // Disable masking for performance (server-to-server)
        contentConverter = null               // Use raw frames, no conversion overhead
    }
}
