# Gonka Chain Observer Frontend

Minimalistic React + TypeScript frontend for Gonka Chain Observer.

## Features

- Real-time inference statistics dashboard
- Auto-refresh every 30 seconds with countdown
- Epoch selector for historical data (last 10 epochs)
- Interactive participant table with clickable rows
- Participant details modal showing:
  - Validator consensus key
  - Inference URL
  - Complete inference statistics
  - Jail and health status
- Visual highlighting for problem participants
- Responsive design with Tailwind CSS

## Setup

```bash
make setup-frontend
```

## Run

```bash
make run-app
```

Frontend starts at `http://localhost:3000`

## Development

```bash
cd frontend
npm run dev
```

## Build

```bash
npm run build
```

Production build outputs to `dist/` directory.

