---
change_name: "semantic-planner"
schema: spec
version: 0.0.1
---

# Commander Orchestration

## Purpose

Defines the multi-Commander execution model: one Commander instance runs per transaction, independent transactions execute concurrently, and dependent transactions run in dependency order. Each transaction's result is collected and combined into the final user answer.

## Requirements

### Requirement: One Commander per transaction

The execution layer SHALL create one Commander instance per transaction, each running against its own transaction's goal. This is the multi-Commander model: the number of Commanders equals the number of transactions.

#### Scenario: N transactions produce N Commanders

- **WHEN** a plan has three transactions
- **THEN** three Commander instances are spawned, one per transaction

### Requirement: Independent transactions run in parallel

Transactions with no dependencies SHALL be executed concurrently by their Commanders.

#### Scenario: independent transactions run concurrently

- **WHEN** two transactions have no dependency between them
- **THEN** their Commanders run at the same time

### Requirement: Dependent transactions run in dependency order

A transaction SHALL be executed only after all its dependencies have produced their results.

#### Scenario: dependent transaction waits

- **WHEN** transaction B depends on transaction A
- **THEN** B's Commander does not start until A's Commander completes

### Requirement: Per-transaction results are collected into the final answer

The orchestration SHALL collect every transaction's result and combine them into the final user answer, streamed via the existing answer-streaming path.

#### Scenario: all transactions complete and combine

- **WHEN** all transactions finish
- **THEN** their results are collected into a single final answer and streamed to the user
