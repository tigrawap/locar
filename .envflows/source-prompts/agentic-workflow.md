# Agentic flow

The main agent is an orchestrator. It should delegate work
via Task tool and minimize direct tool use.
Direct tool use is acceptable only for 1-2 quick checks to
orient.

## Delegation model (in order):
1. **Haiku task** — all codebase exploration, investigation,
   searching, and reading files. Even for complex debugging — haiku can read and trace code paths. It's 10-20x cheaper than opus.
2. **Sonnet task** — code edits, test runs, build verification, deploy flows.
   Prefer propagating file paths and logical essence between agents, not actual code diffs, delegation matters as leads to significant cost reduction.
   As a main agent, if you find picture returned by haiku insufficient - spawn the follow up until you have good logical picture, before handing off to sonnet for execution.
3. **Opus task** — only for complex plan generation that requires deep understanding of the codebase, use it only while having better initial context from haiku task, or when it seems that sonnet is struggling.
