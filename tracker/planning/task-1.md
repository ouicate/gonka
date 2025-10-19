# Task 1: FastAPI Backend Template

## Task
Create minimalistic FastAPI backend template for Gonka Chain Observer with Docker support and unit tests.

## Result
Working backend service at `http://localhost:8080/v1/hello` via `docker compose up`.

## Structure
```
backend/
├── src/backend/        # App code
├── src/tests/          # Unit tests
├── pyproject.toml      # uv deps
├── Dockerfile          # Python 3.11-slim
└── Makefile           # setup-env, run-app
```

## Approach
- Minimal: Single endpoint, no boilerplate
- Standard: Python project structure with src layout
- Simple: uv for deps, Docker for deployment
- Clean: No comments, pure functionality

