# WebSocket Integration Timeout Issue in Testermint Mock Server

## Problem Statement

After adding WebSocket support to the testermint mock server (commit fe217d3747df6b57101235066401480222c764f2), all tests started failing with "Read timed out" error on `http://localhost:9002/admin/v1/nodes`.

**Key Observation:** 
- Tests PASS when WebSocket endpoint is completely disabled (was before)
- Tests FAIL when WebSocket endpoint (`/api/v1/pow/ws`) is present


```
ERROR:/genesis - AddNode - Error making request: url=http://localhost:9002/admin/v1/nodes: com.github.kittinunf.fuel.core.BubbleFuelError: Read timed out
	at com.github.kittinunf.fuel.core.FuelError$Companion.wrap(FuelError.kt:84)
	at com.github.kittinunf.fuel.core.DeserializableKt.response(Deserializable.kt:170)
	at com.github.kittinunf.fuel.core.requests.DefaultRequest.responseObject(DefaultRequest.kt:490)
	at com.productscience.ApplicationAPI.addNode$lambda$13(ApplicationAPI.kt:214)
	at com.productscience.HasConfig$DefaultImpls.wrapLog$lambda$1(Logging.kt:46)
	at com.productscience.LoggingKt.logContext(Logging.kt:18)
	at com.productscience.HasConfig$DefaultImpls.wrapLog(Logging.kt:34)
	at com.productscience.ApplicationAPI.wrapLog(ApplicationAPI.kt:29)
	at com.productscience.ApplicationAPI.addNode(ApplicationAPI.kt:210)
	at com.productscience.ApplicationAPI.setNodesTo(ApplicationAPI.kt:177)
	at com.productscience.MainKt.resetMlNodesToDefault(Main.kt:368)
	at com.productscience.MainKt.initialize(Main.kt:302)
	at com.productscience.DockerGroupKt.initCluster(DockerGroup.kt:441)
	at com.productscience.DockerGroupKt.initCluster$default(DockerGroup.kt:425)
	at BandwidthLimiterTests.bandwidth limiter with rate limiting(BandwidthLimiterTests.kt:29)
	at java.base/jdk.internal.reflect.DirectMethodHandleAccessor.invoke(DirectMethodHandleAccessor.java:103)
	at java.base/java.lang.reflect.Method.invoke(Method.java:580)
	at org.junit.platform.commons.util.ReflectionUtils.invokeMethod(ReflectionUtils.java:728)
	at org.junit.jupiter.engine.execution.MethodInvocation.proceed(MethodInvocation.java:60)
	at org.junit.jupiter.engine.execution.InvocationInterceptorChain$ValidatingInvocation.proceed(InvocationInterceptorChain.java:131)
	at org.junit.jupiter.engine.extension.TimeoutExtension.intercept(TimeoutExtension.java:156)
	at org.junit.jupiter.engine.extension.TimeoutExtension.interceptTestableMethod(TimeoutExtension.java:147)
	at org.junit.jupiter.engine.extension.TimeoutExtension.interceptTestMethod(TimeoutExtension.java:86)
	at org.junit.jupiter.engine.execution.InterceptingExecutableInvoker$ReflectiveInterceptorCall.lambda$ofVoidMethod$0(InterceptingExecutableInvoker.java:103)
	at org.junit.jupiter.engine.execution.InterceptingExecutableInvoker.lambda$invoke$0(InterceptingExecutableInvoker.java:93)
	at org.junit.jupiter.engine.execution.InvocationInterceptorChain$InterceptedInvocation.proceed(InvocationInterceptorChain.java:106)
	at org.junit.jupiter.engine.execution.InvocationInterceptorChain.proceed(InvocationInterceptorChain.java:64)
	at org.junit.jupiter.engine.execution.InvocationInterceptorChain.chainAndInvoke(InvocationInterceptorChain.java:45)
	at org.junit.jupiter.engine.execution.InvocationInterceptorChain.invoke(InvocationInterceptorChain.java:37)
	at org.junit.jupiter.engine.execution.InterceptingExecutableInvoker.invoke(InterceptingExecutableInvoker.java:92)
	at org.junit.jupiter.engine.execution.InterceptingExecutableInvoker.invoke(InterceptingExecutableInvoker.java:86)
	at org.junit.jupiter.engine.descriptor.TestMethodTestDescriptor.lambda$invokeTestMethod$7(TestMethodTestDescriptor.java:218)
	at org.junit.platform.engine.support.hierarchical.ThrowableCollector.execute(ThrowableCollector.java:73)
	at org.junit.jupiter.engine.descriptor.TestMethodTestDescriptor.invokeTestMethod(TestMethodTestDescriptor.java:214)
	at org.junit.jupiter.engine.descriptor.TestMethodTestDescriptor.execute(TestMethodTestDescriptor.java:139)
	at org.junit.jupiter.engine.descriptor.TestMethodTestDescriptor.execute(TestMethodTestDescriptor.java:69)
	at org.junit.platform.engine.support.hierarchical.NodeTestTask.lambda$executeRecursively$6(NodeTestTask.java:151)
	at org.junit.platform.engine.support.hierarchical.ThrowableCollector.execute(ThrowableCollector.java:73)
	at org.junit.platform.engine.support.hierarchical.NodeTestTask.lambda$executeRecursively$8(NodeTestTask.java:141)
	at org.junit.platform.engine.support.hierarchical.Node.around(Node.java:137)
	at org.junit.platform.engine.support.hierarchical.NodeTestTask.lambda$executeRecursively$9(NodeTestTask.java:139)
	at org.junit.platform.engine.support.hierarchical.ThrowableCollector.execute(ThrowableCollector.java:73)
	at org.junit.platform.engine.support.hierarchical.NodeTestTask.executeRecursively(NodeTestTask.java:138)
	at org.junit.platform.engine.support.hierarchical.NodeTestTask.execute(NodeTestTask.java:95)
```

## Root Cause Analysis

### Request Flow (from code):
1. Test calls `POST /admin/v1/nodes` to add a node
2. This blocks waiting for `RegisterNode` command response
3. `RegisterNode.Execute()` calls `TriggerStatusQuery()` at line 106 in `node_admin_commands.go`
4. `nodeStatusQueryWorker` receives trigger and calls `queryNodeStatus()`
5. **`queryNodeStatus()` makes HTTP GET to `/api/v1/state` on the mock server**
6. **This HTTP request times out**, causing the entire `addNode` call to block indefinitely
