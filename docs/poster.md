# Lucinda: A Computing-Power Aware Orchestration Framework for Distributed AI Agents

## Abstract

Existing LLM agent frameworks excel at macro-logic routing but fail to perceive the status of **underlying heterogeneous hardware** (e.g., edge-deployed Ollama vs. cloud APIs). This missing layer leads to severe **resource starvation or task blocking** in edge environments. This work presents **Lucinda**, a compute-aware distributed agent orchestrator that bridges this gap. By decoupling agent behaviors into a fine-grained **"Plan-Execute-Reduce"** pipeline, Lucinda dynamically schedules sub-tasks based on an **event-driven paradigm** and **real-time hardware telemetry**. In addition, a custom **TaskBoard with a publish-lease protocol** is introduced to prevent task contention and secure network stability, establishing an efficient, resource-optimized runtime for edge-cloud collaborative agents.

## 1. Introdcution

// ai agent 的快速发展， 越来越多的用户开始使用 AI agent 作为自己的生产力工具。

// 就目前而言，很多人选择在云端部署使用这些 models，可能有多种原因，主要有：1. 云服务商提供简单便捷的服务，2. 本地设备无法提供稳定的最小所需资源。

The rapid advancement of Artificial Intelligence (AI) agents has seamlessly integrated them into mainstream productivity workflows, serving as indispensable tools for daily oeprations. Currently, a vast majority of users choose for cloud-based deployment. This trend is driven by sort of reasons, but mainly two: first, cloud service providers offer turnkey, plug-and-play accessibility, which seems attractive for primary users; second, standard localized hardware typically fails to provision the stable, minimum required compute and memory resources to sustain heavy localized model initialization.

// 这种 agent 服务模式对于用户来说虽然非常便捷，但是同时也产生许多无法掩盖的问题。1. 隐私，2. 实时性，3. 自定义程度低 （举点例子）

However, while convenient, this absolute reliance on centralized cloud models induces servere and unignorable drawbacks:

+ **User Privacy Vulnerabilities**: Sensitive individual prompts, contexts or corporate datasets are continuously exposed to third-party infrastructure during transmission,  magnifying data risks.
+ **Degraded Real-Time Responsiveness**: Constant cloud round-trips introduce high network demands and  unignorable latency, bottlenecking interactive application efficiency.
+ **Low Customization Freedom**: Centrialized APIs restrict users to rigid, immutable configurations, preventing fine-grained parameter tuning or local context adaption.

// 市面上现有的框架：

1. 纯应用层 Agent 编排框架，LangChain，AutoGen 等，不会感知底层硬件，如果将其放在边缘设备上，它们只会盲目地创建任务、阻塞等待，而不理会 CPU/vRAM 等资源分配问题
2. 底层大模型推理引擎比如 ollama，vLLM 等，主要以单机静态服务为主，缺乏上层的复杂任务状态管理和生命周期调度

To resolve these limitations, shifting agent intelligence toward the edge is highly desirable, yet existing software infrastructures exhibit a severe architectural disconnect.

+ **Pure Application-Layer Framework** (e.g., LangChain, AutoGen) : Focus heavily on macro abstraction routing logic while remaining completely blind to the underlying physical substrate. When deployed on constrained edge  networks, they blindly spawn tasks and trigger blocking waits without assessing actual CPU or vRAM consumption.
+ **Local Inference Engines** (e.g., Ollama, vLLM) : Primarily act as static, single-node endpoint servers. They lack macro distributed task state management, lifecycle tracking, and collaborative scheduling capabilities across peer nodes.

// 而 Lucinda 为解决该问题而生，同时也是为未来能力更强、资源消耗更少的 agent 提供一个 General 的任务编排平台。

Lucinda is specially designed to eliminate this gap. It serves as generalized, resource-aware task orchestration runtime platform optimized to empoweer future resource-efficient, highly capable localized agents within a collaborative network.

## 2. System Design

![](./photos/Lucinda-poster.png)

// Lucinda 设备从用户处获取 Request, 并在 TaskWrapper 处包装成 Task 对象进行 Task 处理流程。Task Workflow 过程被分布在云-端网络之中，通过 TaskBoard 将其与单机解耦，使整个边缘网络加入到任务处理之中。在经过 “Plan-Execute-Reduce” 流水线后，系统将结果返回给用户，完成一个任务流程。

The basic workflow of Lucinda initiateds when a device intercepts a user ChatRequest, which the gateway immediately encapsulates into a structured Task object via the TaskWrapper. Instead of being confined to a standalone machine, the execution topology is distributed asynchronously across the edge-cloud network mesh. By utilizing a shared TaskBoard, single-node decoupling is accomplished, effectively recruiting the computational power of the entire local neighbourhood into the task procssing pool. Following the execution of the “Plan-Execute-Reduce” pipeline, the final aggregated artifact is streamed back to the user to complete the operational cycle.

// Lucinda 系统主要分为四层结构：

1. Server 与 TaskWrapper ：接收用户请求并转换为 Task
2. Task Workflow Layer: Task 处理流水线，面向整个云-端网络
3. Task Management Layer：维护本节点的 Task 状态、Task 调度以及 Task 节点间传输
4. Infrastruction Layer: Eventbus, Transport, HardwareMonitor等基础组件，以及 agent 底层相关，如 ProviderController，Toolbox，ContextManager 等，提供云服务商服务接口

The architecture of Lucinda is strictly separated into four highly cohesive layers combined with specialized dynamic mechanisms:

### A. Four-Layer Architectural Blueprint

+ **Server and TaskWrapper Layer** : Functions as the network ingress, absorbing concurrent user payloads and constructing lifecycle-tracked Task contexts.
+ **Task Workflow Layer** : Governs the distributed multi-stage execution pipeline. The TaskPlanner segments complex inputs into a Directed Acyclic Graph (DAG) of micro-tasks: the TaskExecutor fires parallel agent executions or external tool invocations; the TaskReducer cleans and synthesizes intermediate tokens into a unified payload.
+ **Task Management Layer**: The orhestration center of each node. It manages local finite state machines (TaskStateManager). determines adaptive task allocations (TaskScheduler), and routes data packets across endpoints (TaskPostman).
+ **Infrastructure Layer** : The hardware and network layer. It provisions a lock-free EventBus for macro-level asynchronous signaling,  a peer-to-peer Transport subsystem for node connectivity, and a HardwareMonitor for live telemetry. It also aggregates agent-level foundational modules including the ProviderController (also for cloud API wrapping), the Toolbox (tool management for overall agents), and the session ContextManager, all of them have extension API exposed to Cloud Service Providers.

### B. TaskBoard with a Publish-Lease Protocol: Real-time Task Interview

To eliminate distributed race conditions without a heavy centralized lock manager, Lucinda implements a Publish-Lease protocol on the shared TaskBoard. When a sub-task DAG is generated, its nodes are injected onto the board in a Pending state. Peer workers run internel “Task Interviews”. matching task hardware demands against their current capability envelopes. Upon selection, the TaskBoard issues a temporary Lease bound to a strict Time-To-Live (TTL). The worker must continuously emit low-overhead heartbeats; if a node suffers sudden compute starvation or goes offline, the lease silently expires, and the TaskBoard automatically reverts the sub-task back to Pending for peer reclamation, achieving fault-tolerance.

### C. Service Module: Multi-Attribute Adaptive Dispatching Framework 

Borrowing principles from ploymorphic execution runtimes, the Service Module shifts away from rigid, label-based node routing. Instead, the identities of a specific node is defined by its module plugged, evaluated by ComponentRegistry (e.g. active tool availability, wrapper, planner, executor, reducer). Tasks are routed via a multi-variant matching function that dynamically balances task resource demands againtst live telemetry data, enabling nodes to adaptively transform roles based on shifting network demands. 

### D. Multi-Server and Provider Driver Support 

To sustain horizontal scalability across heterogeneous edge-cloud meshes, the ProviderController establishes an abstract driver interface. It wraps diverse execution runtimes (e.g. local Ollama instances or remote commercial APIs) into uniform compute interfaces. High-frequency, massive data stream, such as live raw LLM streaming tokens, are ingested and digested strictly within the private channels of the localized provider module, while only macro state transitions are broadcasted to the global Eventbus, safeguarding network stability across multi-server environments.

### E. Native Non-Blocking Asynchronous Task

To isolate resource-constrained host machines from synchronous thread starvation during prolonged LLM inference iterations, Lucinda features native asynchronous task tracking. The TaskStateManager maintains the DAG for the task plan, tracking the structural dependencies and real-time execution states of every sub-task node. Instead of blocking the runtime while waiting for upstream tokens to generate, it relies on non-blocking event notifications emitted by the EventBus. When an execution dependency is cleared, the state machine reactively advances the DAG cursor, triggering the next execution phase via lightweight Go channels. This ensures that the global state remains deterministic and fully decoupled from the physical execution lifecycles of individual workers.

 // TaskBoard with a Publish-Lease Protocol: Real-time Task Interview

//  Service Module: Multi-Attribute Adaptive Dispatching Framework

//  Muti-Server and Provider Driver Support

//  Native Native Non-Blocking Asynchronous Task

## 3. Evaluation and Case Study

### A. Qualitative Framework Comparison

### B. Heterogeneous Pipeline Coordination and Fault-Tolerance Case Study

1. Asynchronous DAG Generation
2. Capability CV and Task Interview
3. Polymorphic Dispatch and Distributed Leasing
4. Asynchronous Stream Ingestion
5. Dynamic Fault Recovery

## 4. Implementation &  Future Work