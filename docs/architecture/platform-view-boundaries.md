# Platform And View Boundaries

Mulwiki follows Multica's package boundary pattern while keeping Mulwiki's own product and server architecture. This document is the source of truth for adding durable product capabilities, Web UI, and a future desktop app.

## Principle

All core business capabilities should be designed in this order:

1. Server API
2. CLI
3. Web UI
4. Desktop UI

The server API is the authoritative capability layer. The CLI is the automation, scripting, daemon, CI, and advanced-user entry point. Web and desktop are human interaction surfaces that call the same API contracts.

Before adding a feature, answer:

- Which server API owns this capability?
- How does the CLI expose it for scripts, CI, and agents?
- Does the CLI support `--output json` where automation needs structured output?
- Does the Web UI call the same API rather than reimplementing business behavior?
- Can a future desktop UI reuse the same view and API code?

Web-only business capabilities are only acceptable when the behavior is purely presentational.

## Package Responsibilities

`packages/core` owns shared API clients, query keys, query options, hooks, realtime helpers, and shared types. It must not import from app shells.

`packages/ui` owns reusable UI primitives, design tokens, theme hooks, and low-level components. It must not know about Mulwiki business routes or server APIs.

`packages/views` owns page-level, product-specific React views. Views may use `packages/core` and `packages/ui`, but they must stay platform-neutral:

- no `next/*` imports
- no Electron imports
- no direct Node.js filesystem or process APIs
- no browser routing assumptions beyond injected navigation adapters

`apps/web` owns the Next.js shell:

- route files
- layouts
- Next routing adapters
- metadata and app-router behavior
- server/client component boundaries
- Web-only proxy and deployment configuration

Route files in `apps/web/app` should be thin adapters that unwrap Next params and render a view from `packages/views`.

Future `apps/desktop` should own the Electron/Vite shell:

- native window lifecycle
- app menu and tray behavior
- desktop login/setup affordances
- daemon bootstrap integration
- desktop-specific filesystem dialogs

Desktop-specific native behavior should be passed into shared views through props, providers, or platform adapters. It should not be imported directly by `packages/views`.

## Navigation Boundary

Shared views use `@mulwiki/views/navigation` for navigation. App shells provide the concrete implementation.

For Web, `apps/web/platform/navigation.tsx` adapts Next.js `Link` and `useRouter` into the shared navigation interface.

For a future desktop app, `apps/desktop` should provide an equivalent adapter backed by its router or shell navigation model.

Shared views should use:

- `useAppNavigation()` for imperative navigation
- `AppLink` for links

They should not import `next/link`, `next/navigation`, or desktop router APIs directly.

## Feature Checklist

When adding or changing a product capability:

1. Put durable behavior in the server API and service layer.
2. Add or update the CLI command for automation.
3. Add shared API/query code in `packages/core`.
4. Add page-level business UI in `packages/views`.
5. Keep `apps/web/app` as a route adapter.
6. Add structured CLI output when the command may be consumed by agents or scripts.
7. Add tests at the lowest meaningful layer, then smoke the Web route when UI behavior changes.

This keeps Mulwiki usable from CLI, Web, daemon workflows, and a future desktop app without duplicating business logic across surfaces.
