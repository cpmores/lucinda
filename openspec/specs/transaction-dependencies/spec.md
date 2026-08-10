---
change_name: "semantic-planner"
schema: spec
version: 0.0.1
---

# Transaction Dependencies

## Purpose

Defines how transaction dependencies govern execution: a transaction's Commander is gated on its dependencies completing, dependency outputs are fed into the downstream transaction's goal context, and cycles in the transaction graph are rejected at plan time.

## Requirements

### Requirement: A transaction starts only after its dependencies complete

The orchestrator SHALL gate a transaction's Commander on the completion of every transaction in its `Deps`.

#### Scenario: gated start

- **WHEN** a transaction has one or more dependencies
- **THEN** its Commander starts only after all dependency results are available

### Requirement: Dependency outputs feed the downstream context

A transaction's Commander SHALL receive its dependencies' outputs as input context for its goal, so the dependent transaction can build on its prerequisites (e.g. "generate similar music" receives "the searched music").

#### Scenario: dependency result is included

- **WHEN** a dependent transaction's Commander starts
- **THEN** the completed dependency outputs are present in its goal context

### Requirement: Dependency cycles are rejected at plan time

The planner SHALL reject a transaction graph containing a cycle rather than produce a plan that can never complete.

#### Scenario: cycle is rejected

- **WHEN** the planner detects a dependency cycle among transactions
- **THEN** it returns a plan error and does not produce an executable plan
