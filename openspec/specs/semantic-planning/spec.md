---
change_name: "semantic-planner"
schema: spec
version: 0.0.1
---

# Semantic Planning

## Purpose

Defines how TaskPlanner decomposes a raw request into semantic transactions rather than fine-grained nodes. Each transaction is a self-contained unit with its own goal and dependency list, and the plan records the chosen execution architecture so each transaction's Commander can behave accordingly.

## Requirements

### Requirement: Planner decomposes a request into semantic transactions

TaskPlanner SHALL decompose a raw request into a set of semantic transactions, each a self-contained unit with its own goal (e.g. "write the doc", "generate the video"). This replaces fine-grained node decomposition: transactions are the units a Commander executes.

#### Scenario: request splits into transactions

- **WHEN** TaskPlanner processes a request
- **THEN** it produces a `TaskPlan` whose `Transactions` each carry a semantic `Goal`

### Requirement: Transactions carry dependencies

Each transaction SHALL list the transactions that must complete before it (its `Deps`). Independent transactions SHALL have empty deps; dependent ones SHALL reference their prerequisites.

#### Scenario: dependent transactions reference prerequisites

- **WHEN** two transactions have an ordering constraint
- **THEN** the later transaction's `Deps` contains the earlier transaction's ID

### Requirement: Plan carries the chosen architecture

`TaskPlan.Architecture` SHALL be set at plan time to `react` or `plan_execute`, selecting how each transaction's Commander behaves.

#### Scenario: ReAct is selected at plan time

- **WHEN** a request selects the ReAct agent
- **THEN** the plan's `Architecture` is `react` and each transaction's Commander uses the reasoning loop

#### Scenario: Plan-and-Execute is selected at plan time

- **WHEN** a request selects Plan-and-Execute
- **THEN** the plan's `Architecture` is `plan_execute` and each transaction's Commander dispatches deterministically
