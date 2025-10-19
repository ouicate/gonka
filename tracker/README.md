# Gonka Chain Observer

Minimalistic full-stack application for observing Gonka Chain.

## Structure

- `backend/` - FastAPI backend (Python 3.11)
- `frontend/` - React + TypeScript frontend (Vite)
- `config.env` - Environment configuration
- `Makefile` - Setup and run commands
- `docker-compose.yaml` - Traefik reverse proxy + services

## Setup

```bash
make setup-env
```

## Run

```bash
make run-app
```

Application available at `http://localhost`:
- Frontend: `http://localhost/`
- Backend: `http://localhost/api/v1/hello`

## Test

```bash
make test-all
```

- `test-backend` - Backend unit tests
- `test-integration` - Live service tests
- `test-all` - Complete test suite

## Development

Backend:
```bash
cd backend
source .venv/bin/activate
uvicorn src.backend.app:app --reload --host 0.0.0.0 --port 8000
```

Frontend:
```bash
cd frontend
npm run dev
```

