# Lessons

## Task Acquisition Model

- Do not assume Agent self-claiming is the default task model for this project.
- The authoritative model is delegation: an operator/coordinator assigns or delegates work to a target agent or agent type; the target agent reads/accepts delegated work and then submits results.
- Avoid introducing `claim`, `ClaimNext`, `task:pull:*`, `lease` or `claimed_by_*` as core ownership concepts unless the user explicitly reintroduces them.

## OpenViking Code Sync Boundary

- Do not sync repository code into OpenViking by writing file contents or repository indexes as memory.
- Code repositories must be registered through OpenViking's native resource ingestion path, using the Git remote URL and `POST /api/v1/resources` semantics.
- AITask CLI should proxy OpenViking operations through the core backend; the backend owns system OpenViking credentials and project settings.
