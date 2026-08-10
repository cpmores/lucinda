---
change_name: "provider-behind-executor"
schema: spec
version: 0.0.1
---

# Commander Provider-Free

## Purpose

Ensures TaskCommander never resolves or invokes a provider directly. All LLM work — reasoning decisions and answer synthesis — is issued as tasks through the board and observed via the tracer. The commander has no dependency on `ProviderController` or `StreamRouter`.

## Requirements

### Requirement: Commander never calls a provider directly

TaskCommander SHALL NOT resolve or invoke any provider. All LLM work — reasoning decisions and answer synthesis — SHALL be issued as tasks through the board and observed via the tracer. The commander's dependencies SHALL NOT include `ProviderController` or `StreamRouter`.

#### Scenario: commander has no provider dependency

- **WHEN** the commander module resolves its dependencies
- **THEN** it does not require a ProviderController or StreamRouter

### Requirement: Commander issues reasoning as a board task

To obtain a ReAct decision, the commander SHALL publish a reason-marked `TaskReady` whose spec carries the reasoning prompt; the board SHALL route it to any node with a capable model, and a remote executor SHALL produce the decision text.

#### Scenario: reasoning crosses nodes

- **WHEN** the commander needs a decision and the local node lacks a capable model
- **THEN** the reasoning task is assigned to a remote node that serves the model and its result returns via the tracer

### Requirement: Commander parses the reasoning result

The commander SHALL parse the decision (continue / done) from the completed reasoning task's output.

#### Scenario: decision drives the next step

- **WHEN** a reason task completes
- **THEN** the commander parses its output and acts on the returned decision
