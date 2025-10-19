# Task 5: Jail Status and Node Health Tracking

## Task
Implement jail status tracking and node health monitoring for all participants with database persistence, background polling, and frontend visualization.

## Status
COMPLETED

## Result
Production-ready jail and health tracking system with:
- Jail status tracking per epoch with historical database storage
- Node health monitoring with automatic health checks
- Database tables indexed by participant for future historical analysis
- Background polling (jail: 120s, health: 30s)
- Inline fetching guarantees data is always present
- Frontend columns showing jail status badges and health dots
- All 52 tests passing (including 8 new tests for jail/health)

## Actual Structure
```
backend/src/backend/
├── database.py          # Extended with jail_status and node_health tables
├── client.py            # Validator fetching, signing info, health checks
├── models.py            # Extended ParticipantStats with jail/health fields
├── service.py           # Jail/health fetching with inline fallback
└── app.py               # Three background polling tasks

backend/src/tests/
├── test_database.py     # Added 4 tests for jail/health storage
└── test_client.py       # Added 4 tests for validators and health checks

frontend/src/
├── types/inference.ts   # Extended Participant with jail/health fields
└── components/
    └── ParticipantTable.tsx  # Added Jail and Health columns
```

## Implementation

### Phase 1: Database Layer
Created two new tables with participant indexing:

**jail_status table:**
- Stores per-epoch jail status for each participant
- Fields: epoch_id, participant_index, is_jailed, jailed_until, ready_to_unjail, valcons_address
- Indexed by participant_index for historical queries
- Enables future feature: plot participant jail history over time

**node_health table:**
- Stores latest health check result per participant
- Fields: participant_index, is_healthy, last_check, error_message, response_time_ms
- Tracks health status and response times

Methods added:
- `save_jail_status_batch()` - batch save jail statuses
- `get_jail_status()` - retrieve by epoch/participant
- `save_node_health_batch()` - batch save health results
- `get_node_health()` - retrieve by participant

### Phase 2: Client Layer
Extended GonkaClient with validator and health methods:

**Validator querying:**
- `get_all_validators()` - fetch all validators with pagination
- `get_signing_info()` - fetch slashing info for valcons address
- `pubkey_to_valcons()` - convert ed25519 pubkey to bech32 valcons address
  - Implements full BIP-0173 bech32 encoding
  - Converts consensus pubkey to validator consensus address

**Health checking:**
- `check_node_health()` - HTTP GET to /health endpoint with 5s timeout
- Returns: is_healthy, error_message, response_time_ms
- Handles connection failures gracefully

### Phase 3: Models Layer
Extended ParticipantStats with optional fields:
```python
is_jailed: Optional[bool] = None
jailed_until: Optional[str] = None
ready_to_unjail: Optional[bool] = None
node_healthy: Optional[bool] = None
node_health_checked_at: Optional[str] = None
```

### Phase 4: Service Layer
Implemented intelligent caching with inline fallback:

**Key method: `merge_jail_and_health_data()`**
- Checks database for cached jail/health data
- If missing: fetches inline immediately
- Ensures data is ALWAYS present on every request
- Accepts epoch_id, participants, height, active_participants

**Jail status fetching:**
- Queries all validators via chain-api
- Filters for active participants only
- Checks signing info for jailed_until timestamp
- Compares with current time for ready_to_unjail status
- Maps validators to participants via consensus pubkey

**Health checking:**
- Iterates through all active participants
- Checks /health endpoint with 5-second timeout
- Records success/failure and response time
- Stores most recent status per participant

### Phase 5: Background Polling
Added three independent polling tasks:

**poll_current_epoch() - every 30s:**
- Fetches current epoch inference statistics
- Updates participant stats cache

**poll_jail_status() - every 120s:**
- Fetches validator set and signing info
- Updates jail status cache for current epoch
- Starts after 10s delay to stagger with health

**poll_node_health() - every 30s:**
- Checks health endpoints for all participants
- Updates node health cache
- Starts after 5s delay to stagger with epoch polling

All tasks run continuously and handle errors gracefully without stopping the application.

### Phase 6: Frontend Integration
Added two minimal columns to participant table:

**Jail Status Column:**
- Red badge "JAILED" if is_jailed=true
- Green badge "ACTIVE" if is_jailed=false
- Gray "-" if unknown

**Health Column:**
- Green dot for node_healthy=true
- Red dot for node_healthy=false
- Gray dot for unknown status

Design follows existing minimal aesthetic with subtle badges and status indicators.

### Phase 7: Docker Configuration
Updated docker-compose.yaml:
- Added `--providers.docker.watch=true` to Traefik
- Added `restart: unless-stopped` for Traefik
- Ensures automatic container discovery on restarts

## Key Implementation Details

### Always-Present Data Guarantee
The system guarantees jail and health data is always present through a two-tier approach:

1. **Background Polling (Primary):**
   - Jail: updates every 120 seconds
   - Health: updates every 30 seconds
   - Keeps cache fresh for fast responses

2. **Inline Fetching (Fallback):**
   - If cache is empty: fetch data immediately during request
   - First request after startup: fetches inline
   - Cache miss scenarios: fetches inline
   - Ensures zero null values in API responses

### Jail Status Detection
**Logic flow:**
1. Fetch all validators from chain-api
2. Filter for validators with tokens > 0
3. Match validators to participants via consensus pubkey
4. For jailed validators:
   - Fetch signing info via cosmos slashing module
   - Check jailed_until timestamp
   - Calculate ready_to_unjail (current_time > jailed_until)
5. Store per epoch for historical tracking

### Health Check Implementation
**Health check protocol:**
- Target: `{inference_url}/health`
- Timeout: 5 seconds
- Success: HTTP 200 status
- Failure: any error or non-200 status
- Records response time on success
- Stores error message on failure

### Historical Data Storage
Jail status stored per epoch enables future features:
- Plot participant jail history over epochs
- Analyze jail patterns and durations
- Track unjail readiness over time
- Index by participant_index for efficient queries

## Testing
All 52 tests passing:

**New tests (8 total):**
- 4 database tests for jail/health storage and retrieval
- 2 client tests for bech32 encoding
- 2 client tests for health checking

**Test coverage:**
- Unit tests with mocked data
- Integration tests with live Gonka Chain API
- Database persistence verification
- Client method correctness

## Behavior Verification

**Data presence guarantee:**
- Tested with empty cache: data fetched inline
- Tested with populated cache: uses cached data
- Verified 0 participants with null jail/health fields
- Confirmed background tasks continue updating cache

**Frontend behavior:**
- Auto-refresh every 30 seconds (current epoch only)
- Clean countdown display
- Jail and Health columns always populated
- Responsive to manual refresh

## Configuration

No new environment variables required. Uses existing:
- `INFERENCE_URLS` - Gonka Chain API endpoints
- `CACHE_DB_PATH` - SQLite database path

## Performance Characteristics

**Jail status polling:**
- Frequency: 120 seconds
- API calls per poll: ~2 (validators + signing info)
- Duration: ~2-3 seconds total
- Impact: minimal

**Health polling:**
- Frequency: 30 seconds
- API calls per poll: 28 (one per participant)
- Duration: ~5-10 seconds total (parallel execution)
- Impact: low

**Inline fetching:**
- Only on cache miss
- Adds ~5-15 seconds to first request
- Subsequent requests use cache (fast)

## Future Enhancements

The participant-indexed jail_status table enables:
- Historical jail timeline visualization
- Jail frequency analysis per participant
- Unjail success rate tracking
- Correlation analysis with missed requests

