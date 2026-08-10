---
change_name: "react-system"
schema: spec
version: 0.0.1
---

# React Loop

## Purpose

Defines TaskCommander's ReAct loop for plans with `Architecture == react`: reasoning against a trajectory, issuing decision tasks through the standard board Publish-Lease path, observing results, enforcing a max-steps cap, and handling reasoning and decision-task failures — while emitting user-visible telemetry.

## Requirements

### Requirement: Commander runs a reasoning loop on ReAct plans

For a plan with `Architecture == react`, TaskCommander SHALL ask its reasoning LLM for a structured decision (continue with a task, or done with an answer) instead of walking a static DAG.

#### Scenario: first decision starts the loop

- **WHEN** a ReAct plan is ingested
- **THEN** the commander calls the reasoning LLM and acts on the returned decision

### Requirement: Commander issues decision tasks through the board

A `continue` decision SHALL produce a task that flows through the standard TaskReady → board Publish-Lease → executor path, so remote execution and capability matching work identically for ReAct actions.

#### Scenario: continue decision issues a task

- **WHEN** the reasoning LLM returns `continue` with a task spec
- **THEN** the commander publishes TaskReady for that task and the board assigns it

### Requirement: Commander observes results and reasons again

The commander SHALL observe each completed action (via TaskTraced) and feed the result back into the trajectory for the next reasoning call.

#### Scenario: result triggers the next reasoning step

- **WHEN** a ReAct decision-task reaches the Done traced state
- **THEN** the commander appends its output to the trajectory and calls the reasoning LLM again

### Requirement: Commander enforces a max-steps cap

The ReAct loop SHALL terminate after a bounded number of iterations to prevent infinite loops.

#### Scenario: cap reached terminates the plan

- **WHEN** the number of reasoning iterations reaches the configured maximum
- **THEN** the commander finalizes the plan with the collected outputs

### Requirement: Commander falls back when the reasoning LLM fails

If the reasoning call errors or returns an unparseable decision, the commander SHALL finalize the plan with the outputs collected so far rather than stall.

#### Scenario: decision failure falls back gracefully

- **WHEN** the reasoning LLM errors or returns an invalid decision
- **THEN** the commander finalizes the plan with collected outputs and reports success

### Requirement: Commander fails the plan on a decision-task failure

If an action issued by the reasoning loop reaches the Failed traced state, the plan SHALL terminate with an error.

#### Scenario: failed action terminates the plan

- **WHEN** a ReAct decision-task reaches the Failed traced state
- **THEN** the commander terminates the plan with an error result

### Requirement: Reasoning steps emit user-visible telemetry

Each ReAct iteration SHALL emit a status frame so the client sees the commander thinking and acting, without leaking the raw reasoning text.

#### Scenario: iteration is visible to the user

- **WHEN** the commander starts a reasoning step or issues an action
- **THEN** a status frame is emitted with the iteration number and action summary
