---
change_name: "react-system"
schema: spec
version: 0.0.1
---

# React Planning

## Purpose

Defines how TaskPlanner selects the execution architecture for a request: it decides from configuration whether a plan runs as Plan-Execute or ReAct, and records the decision plus the original goal on `TaskPlan` so the commander can branch on it.

## Requirements

### Requirement: Planner selects the execution architecture from configuration

TaskPlanner SHALL decide whether a plan runs as Plan-Execute or ReAct based on configuration (default Plan-Execute) and SHALL set `TaskPlan.Architecture` accordingly. The commander SHALL branch on this field to choose the execution mode.

#### Scenario: ReAct architecture is configured

- **WHEN** the configuration selects ReAct for a request
- **THEN** TaskPlanner produces a plan whose `Architecture` is `react`

#### Scenario: default architecture is Plan-Execute

- **WHEN** no ReAct configuration is present
- **THEN** TaskPlanner produces a plan whose `Architecture` is `plan_execute`

### Requirement: ReAct plans carry the original goal

A plan SHALL record the original user request in `TaskPlan.Goal` so the reasoning LLM can be prompted with it on every iteration.

#### Scenario: goal equals the raw request

- **WHEN** TaskPlanner plans a ReAct request
- **THEN** `plan.Goal` equals the raw prompt of the request
