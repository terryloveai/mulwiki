# Multica Alignment Plan

Mulwiki should borrow Multica's architecture boundaries without copying its infrastructure choices.

## Adopt

- Chi route groups as the server composition root.
- Session auth followed by workspace membership resolution and role checks.
- One handler struct for shared HTTP dependencies, with domain-specific methods kept thin.
- Service/store code for lifecycle and persistence rules that are shared by HTTP handlers and daemon execution.
- React Query for server state, with stable query keys and query options in `packages/core`.
- Page-level UI in `packages/views`, leaving `apps/web/app` files as route adapters.

## Defer

- PostgreSQL or Redis infrastructure.
- Desktop-specific process/window orchestration.
- GORM or a second persistence abstraction over SQLite.
- Runtime-level model selection. Mulwiki keeps model selection at the Agent level.
- Zustand or another client store for API-owned data.

## Current Priorities

1. Make the Auth -> Workspace -> Role middleware chain real for workspace routes.
2. Persist workspace membership and create an owner membership when a workspace is created.
3. Require daemon identity on daemon-facing routes before mutating runtime or task state.
4. Move claim/complete/fail lifecycle rules into service code.
5. Persist agent task session/message pointers so daemon restarts and UI task detail views have durable state.
6. Split frontend API query definitions from page rendering and migrate route pages into `packages/views` incrementally.

## Invariants

- A workspace-scoped handler must not query by slug only when `workspace_id` is already available in request context.
- A daemon-facing mutation must include a daemon identity and must only mutate records it owns or can claim atomically.
- A task state transition must be monotonic unless the operation is an explicit retry.
- A route component in `apps/web/app` should not own reusable data-fetching or page-level UI logic.
