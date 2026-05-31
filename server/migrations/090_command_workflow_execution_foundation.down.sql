-- Migration: 090_command_workflow_execution_foundation
-- Roll back workspace-scoped CommandDeck workflow execution records.

DROP TABLE IF EXISTS command_workflow_execution;
