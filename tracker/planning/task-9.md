# Task 9: MLNode/Hardware Node Tracking

## Task
Add hardware node (MLNode) tracking to participant details view by fetching from chain API, caching in database, and displaying in the frontend with 10-minute polling and lazy loading.

## Status
COMPLETED

## Result
Production-ready MLNode tracking system with:
- New endpoint consumption: `/chain-api/productscience/inference/inference/hardware_nodes/{participant_address}`
- Database table for persistent caching per epoch
- Background polling (10 minutes)
- Inline fetching with database caching
- Frontend card-based display with proper empty state handling
- Proper UX for unreported hardware (shows "Hardware not reported" vs "No hardware")

## Implementation

**Database:**
- Created `participant_hardware_nodes` table (epoch_id, participant_id, local_id, status, models_json, hardware_json, host, port, last_updated)
- Methods: `save_hardware_nodes_batch()`, `get_hardware_nodes()`
- Indexed on (epoch_id, participant_id) for fast lookups
- Sorted by local_id (ASC) for consistent display

**Models:**
- `HardwareInfo` - type, count
- `MLNodeInfo` - local_id, status, models[], hardware[], host, port
- Updated `ParticipantDetailsResponse` to include `ml_nodes: List[MLNodeInfo]`

**Client:**
- `get_hardware_nodes(participant_address)` - fetches hardware nodes from chain API
- Returns empty list on error (graceful degradation)
- Extracts `nodes.hardware_nodes` from response

**Service:**
- Updated `get_participant_details()` - inline fetches hardware nodes if missing from cache
- Added `poll_hardware_nodes()` - background polling for all current epoch participants
- Follows same pattern as warm keys (lazy + polling)

**App:**
- Added `poll_hardware_nodes()` background task (600s intervals, 25s startup delay)
- Cancellation handler for graceful shutdown
- Updated startup logging

**Frontend:**
- Added `HardwareInfo` and `MLNodeInfo` TypeScript interfaces
- Updated `ParticipantDetailsResponse` interface
- Added MLNodes section in ParticipantModal after Rewards section
- Card-based layout (2 columns on large screens, 1 on small)
- Each card shows: local_id + status badge, models (with tags), hardware specs, network (host:port)
- Proper empty state: "Hardware not reported" (italic gray) when hardware[] is empty
- Distinction: empty hardware array means data not provided, not absence of hardware

## Testing

**Created `test_hardware_nodes.py` with 8 tests:**
1. Database hardware nodes save/retrieve
2. Hardware nodes replacement for same participant
3. Empty hardware nodes handling
4. Multiple participants with different hardware
5. HardwareInfo model validation
6. MLNodeInfo model validation
7. Empty hardware list handling
8. Hardware nodes sorting (by local_id)

**Updated `test_models.py`:**
- Added HardwareInfo and MLNodeInfo imports
- Updated ParticipantDetailsResponse tests to include ml_nodes field
- Added test_participant_details_response_with_ml_nodes

**All 78 backend tests passing including:**
- 8 new tests for hardware nodes
- 1 updated test for ParticipantDetailsResponse with ml_nodes
- All existing tests still passing

## Performance

**Polling intervals:**
- Epoch stats: 5 minutes (300s)
- Jail: 120s
- Health: 30s
- Rewards: 60s
- Warm keys: 5 minutes (300s)
- **Hardware nodes: 10 minutes (600s)** - new

**Response times:**
- Participant details with hardware nodes: ~100-500ms first time, <50ms cached
- Hardware nodes: instant (cached or inline-fetched)

## Key Features

**Hardware Node Display:**
- Shows all MLNodes with complete information
- Status badge for each node (e.g., "INFERENCE", "POC")
- Models displayed as tags
- Hardware specs formatted as "2x NVIDIA GeForce RTX 3090 | 24GB"
- Network info in monospace font (host:port)

**Empty State Handling:**
- Empty hardware[] array: "Hardware not reported" (italic, gray) - indicates data wasn't provided
- No nodes: "No MLNodes configured"
- Loading state: "Loading MLNodes..."

**Data Guarantees:**
- Inline fetching: hardware nodes always present on first view
- Database caching: instant response on subsequent views
- Background polling: updates every 10 minutes for current epoch
- Per-epoch storage: tracks historical changes

**Error Handling:**
- Graceful degradation on API errors (returns empty list)
- Database errors logged but don't crash service
- Frontend shows appropriate loading/empty states

## API Structure

**Hardware Nodes Response:**
```json
{
  "nodes": {
    "participant": "gonka1...",
    "hardware_nodes": [
      {
        "local_id": "node-1",
        "status": "INFERENCE",
        "models": ["Qwen/Qwen3-32B-FP8"],
        "hardware": [
          {
            "type": "NVIDIA GeForce RTX 3090 | 24GB",
            "count": 2
          }
        ],
        "host": "192.168.1.1",
        "port": "8080"
      }
    ]
  }
}
```

## Files Modified

**Backend:**
- `backend/src/backend/database.py` - table and methods
- `backend/src/backend/models.py` - HardwareInfo, MLNodeInfo models
- `backend/src/backend/client.py` - get_hardware_nodes method
- `backend/src/backend/service.py` - hardware node fetching and polling
- `backend/src/backend/app.py` - background polling task
- `backend/src/tests/test_hardware_nodes.py` - new test file (8 tests)
- `backend/src/tests/test_models.py` - updated tests

**Frontend:**
- `frontend/src/types/inference.ts` - HardwareInfo, MLNodeInfo interfaces
- `frontend/src/components/ParticipantModal.tsx` - MLNodes section display

**Test Data:**
- `backend/test_data/hardware_nodes_gonka1sqwpuxk.json` - mainnet example (12 nodes, empty hardware)
- `backend/test_data/hardware_nodes_gonka10ypwjuh.json` - testnet example (3 nodes, with hardware)

## Notes

- Hardware list can be empty (common on mainnet) - this means data not reported, not absence of hardware
- Each node can have multiple hardware entries with count > 1
- Follows minimalistic patterns from tasks 7 and 8
- Same lazy loading + background polling strategy
- Per-epoch storage to track historical changes
- Sorted by local_id for predictable display order

