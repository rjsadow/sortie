# Launchpad Overview

## What is Launchpad?

Launchpad is an open-source web application that provides a centralized
portal for launching internal applications within large organizations.
Instead of maintaining bookmarks, wikis, or scattered documentation,
users access a single page to find and launch any application they need.

## Core Principles

- **Simple** - Clean UI, minimal learning curve
- **Effective** - Fast access to any application
- **Reliable** - Works consistently, handles edge cases gracefully
- **Secure** - Supports enterprise authentication standards

## Target Users

- Employees in large organizations with many internal tools
- IT administrators managing application catalogs
- Organizations seeking to improve tool discoverability

## Key Features (MVP)

1. **Application Grid** - Visual display of all available applications
2. **Search** - Quickly find apps by name or description
3. **Categories** - Logical grouping of related applications
4. **Responsive Design** - Works on desktop and mobile devices
5. **Configuration-Driven** - Apps defined in JSON, easy to maintain

## Architecture

```text
┌─────────────────────────────────────────┐
│           User's Browser                │
│  ┌───────────────────────────────────┐  │
│  │         Launchpad UI              │  │
│  │  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐ │  │
│  │  │App 1│ │App 2│ │App 3│ │App N│ │  │
│  │  └──┬──┘ └──┬──┘ └──┬──┘ └──┬──┘ │  │
│  └─────┼───────┼───────┼───────┼─────┘  │
└────────┼───────┼───────┼───────┼────────┘
         │       │       │       │
         ▼       ▼       ▼       ▼
    ┌────────┐ ┌────────┐ ┌────────┐
    │Internal│ │Internal│ │External│
    │ App 1  │ │ App 2  │ │  App   │
    └────────┘ └────────┘ └────────┘
```

Launchpad is a **static** application - it links to other applications
but does not host them.

## Brand Guidelines

- Primary Color: `#398D9B` (Teal)
- Secondary Color: `#4AB7C3` (Light Teal)
- Font: Chalet NewYorkNineteenEighty
- See `/launchpad - Brand Guide.pdf` for complete guidelines
