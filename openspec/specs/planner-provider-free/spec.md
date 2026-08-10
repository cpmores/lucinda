---
change_name: "provider-behind-executor"
schema: spec
version: 0.0.1
---

# Planner Provider-Free

## Purpose

Ensures TaskPlanner never resolves or invokes a provider directly. Semantic decomposition is issued as a board task and observed via the tracer, and the plan is built from the observed result. The planner has no dependency on `ProviderController`.

## Requirements

### Requirement: Planner never calls a provider directly

TaskPlanner SHALL NOT resolve or invoke any provider. Semantic decomposition SHALL be issued as a board task and observed via the tracer. The planner's dependencies SHALL NOT include `ProviderController`.

#### Scenario: planner has no provider dependency

- **WHEN** the planner module resolves its dependencies
- **THEN** it does not require a ProviderController

### Requirement: Planner issues decomposition as a board task

To decompose a request, the planner SHALL publish a reason-marked `TaskReady` whose spec carries the decomposition prompt; the board SHALL route it to any node with a capable model, and a remote executor SHALL produce the structured transactions JSON.

#### Scenario: decomposition crosses nodes

- **WHEN** the planner needs a decomposition and the local node lacks a capable model
- **THEN** the decomposition task is assigned to a remote node that serves the model and its result returns via the tracer

### Requirement: Planner builds the plan from the observed result

The planner SHALL parse the returned transactions JSON into a `TaskPlan` and publish `TaskPlanned` once the decomposition task completes.

#### Scenario: plan is built asynchronously

- **WHEN** a decomposition task completes
- **THEN** the planner parses its output into a `TaskPlan` and publishes `TaskPlanned`
