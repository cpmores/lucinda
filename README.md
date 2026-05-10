# lucinda

**Lucinda** is a distributed AI-Operating System purpose-built for the edge.

Unlike traditional operating systems that manage CPU/RAM for processes, or Kubernetes which orchestrates containerized microservices, Lucinda is designed to manage **Inference Capabilities** and **Conversation Contexts** across a decentralized network of consumer-grade hardware (phones, PCs, and local servers).

## Core Philosophy

+ **Context Stability**: Maintaining seamless LLM conversation state across devices.
+ **Intelligent Task Routing**: Effectively dispatching user requests based on real-time hardware rather than static configurations.
+ **Resource Democratization**: Turning idle "dark compute" on home device into a unified, private AI cluster.
+ **Modular Service**: Enabling developers or cloud service provider to provice safe and sharable services and resources.

## Architecture && Components

### Server

Different servers for different apis facing users, it receives `ChatRequest` and sends `ChatResponse` back.

### Monitor

Handle nodes' status, provide information for those modules need to make decisions, including *RouterController*, *Scheduler* and *ContextManager*.

### RouteController

`RouteController` manages **task wrapping**, **task dividing**, **task routing** and **task reducing**, it receives `ChatRequest` and returns `ChatResponse` to *Server* 

+ **TaskWrapper**: wrap `ChatRequest` into `Task`
+ **TaskDivider**: divide `Task` into `SubTask`, then build an `TaskPlan`
+ **TaskRouter**: 
  + **route**: According to `TaskPlan`, route `SubTask` to proper nodes, and produce `RouterPlan`
  + **post**: post `RoutedTask` to specific nodes, then get channels `RoutedResults`

+ **TaskReducer**: Reduce `RoutedResult` to a full `ChatResponse`, then send back to *Server*

### Scheduler 

**Scheduler** receives `SubTask` and builds an `ExecutionPlan`  

### Executor

**Executor** receives `ExectionPlan`, then generate `RoutedResult` back to Boss Node. 

### Transport

This layer deals with basic connections between nodes.

**Transporter** in this module manages **internel node connection** and **raw `NodeMessage` sending**.

### Provider

`Provider` is the interface for agents.

### Toolbox

**Toolbox** provides some safe but additional operations for agents. 

### ContextManager

**ContextManager** manages context for agents. 
