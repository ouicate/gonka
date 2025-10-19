# Task 2: React Frontend Template

## Task
Create minimalistic React + TypeScript frontend for Gonka Chain Observer with Vite, Docker support, and consolidated configuration.

## Result
Working frontend at `http://localhost:3000` fetching from backend via `docker compose up`. Top-level config and Makefile for simplified project structure.

## Structure
```
frontend/
├── src/
│   ├── App.tsx           # Demo component
│   ├── main.tsx          # React entry
│   └── vite-env.d.ts     # TypeScript types
├── package.json          # npm deps
├── tsconfig.json         # TypeScript config
├── vite.config.ts        # Vite config
├── Dockerfile            # Multi-stage build
└── index.html            # Entry point

Root consolidation:
├── config.env.template   # Backend + frontend env vars
├── Makefile              # setup-backend, setup-frontend, run-app
└── .gitignore            # All ignore rules
```

## Approach
- Minimal: Vite + React + TypeScript standard template
- Standard: npm package manager, modern tooling
- Simple: Single config, single Makefile, Docker deployment
- Clean: Multi-stage build (Node + nginx), CORS-enabled backend
- Consolidated: Top-level config, Makefile, gitignore


