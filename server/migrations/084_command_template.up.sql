-- Migration: 084_command_template
-- Create command_template table for approved command allowlist

CREATE TABLE command_template (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    command TEXT NOT NULL,
    description TEXT,
    category TEXT NOT NULL DEFAULT 'general',
    allowed_args TEXT[] DEFAULT '{}',
    working_dir_bound TEXT,
    risk_level TEXT NOT NULL DEFAULT 'low' CHECK (risk_level IN ('low', 'medium', 'high', 'blocked')),
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    is_builtin BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_command_template_workspace ON command_template(workspace_id);
CREATE INDEX idx_command_template_category ON command_template(category);

-- Seed the built-in command templates.
-- workspace_id '00000000-...' is a reserved marker. GetCommandTemplateByName
-- matches it as a fallback so built-in templates are resolvable by any
-- real workspace UUID. Each template can be overridden per-workspace by
-- inserting a row with the real workspace_id and the same name.
INSERT INTO command_template (workspace_id, name, command, description, category, risk_level, is_builtin) VALUES
    ('00000000-0000-0000-0000-000000000000', 'Git Status',     'git status',                'Show the working tree status',                     'git', 'low', true),
    ('00000000-0000-0000-0000-000000000000', 'Git Branch',     'git branch --show-current', 'Show the current branch name',                  'git', 'low', true),
    ('00000000-0000-0000-0000-000000000000', 'Git Rev-Parse',  'git rev-parse HEAD',        'Show the current commit hash',                    'git', 'low', true),
    ('00000000-0000-0000-0000-000000000000', 'Git Diff --Stat','git diff --stat',           'Show diff summary (names + insertions/deletions)', 'git', 'low', true);