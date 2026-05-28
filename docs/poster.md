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

![](/home/cpmores/Documents/development/repo/lucinda/docs/photos/Lucinda-poster.png)

// Lucinda 设备从用户处获取 Request, 并在 TaskWrapper 处包装成 Task 对象进行 Task 处理流程。Task Workflow 过程被分布在云-端网络之中，通过 TaskBoard 将其与单机解耦，使整个边缘网络加入到任务处理之中。在经过 “Plan-Execute-Reduce” 流水线后，系统将结果返回给用户，完成一个任务流程。

// Lucinda 系统主要分为四层结构：

1. Server 与 TaskWrapper ：接收用户请求并转换为 Task
2. Task Workflow Layer: Task 处理流水线，面向整个云-端网络
3. Task Management Layer：维护本节点的 Task 状态、Task 调度以及 Task 节点间传输
4. Infrastruction Layer: Eventbus, Transport, HardwareMonitor等基础组件，以及 agent 底层相关，如 ProviderController，Toolbox，ContextManager 等，提供云服务商服务接口

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