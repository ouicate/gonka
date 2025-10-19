# Task 8: Warm Key Field

## Task
Add warm key tracking to participant view by fetching authz grants, validating 24 required permissions, and displaying sorted warm keys.

## Status
COMPLETED

## Result
Production-ready warm key tracking system with:
- New field in participant modal displayed after URL field
- Fetches authz grants from `/cosmos/authz/v1beta1/grants/granter/{granter}`
- Validates all 24 required permissions for inference operations
- Supports multiple warm keys per participant
- Sorts by grant expiration (newer on top)
- Background polling (5 minutes)
- Inline fetching with persistent database caching
- Direct participant linking via URL parameters

## Implementation

**Database:**
- Created `participant_warm_keys` table (epoch_id, participant_id, grantee_address, granted_at)
- Methods: `save_warm_keys_batch()`, `get_warm_keys()`
- Indexed on (epoch_id, participant_id) for fast lookups

**Client:**
- `get_authz_grants(granter)` - fetches all grants with pagination
- Validates 24 required message types:
  - MsgStartInference, MsgFinishInference, MsgClaimRewards, MsgValidation
  - MsgSubmitPocBatch, MsgSubmitPocValidation, MsgSubmitSeed, MsgBridgeExchange
  - MsgSubmitTrainingKvRecord, MsgJoinTraining, MsgJoinTrainingStatus, MsgTrainingHeartbeat
  - MsgSetBarrier, MsgClaimTrainingTaskForAssignment, MsgAssignTrainingTask
  - MsgSubmitNewUnfundedParticipant, MsgSubmitHardwareDiff
  - MsgInvalidateInference, MsgRevalidateInference
  - MsgSubmitDealerPart, MsgSubmitVerificationVector, MsgRequestThresholdSignature
  - MsgSubmitPartialSignature, MsgSubmitGroupKeyValidationSignature
- Uses pagination with offset (100 grants per page)
- Returns only grantees with ALL 24 permissions

**Models:**
- `WarmKeyInfo` - grantee_address, granted_at
- Updated `ParticipantDetailsResponse` to include `warm_keys: List[WarmKeyInfo]`

**Service:**
- `get_participant_details()` - inline fetches warm keys if missing from cache
- `poll_warm_keys()` - background polling for all participants in current epoch
- Sorting by granted_at descending (newer first)

**App:**
- `poll_warm_keys()` background task (5 min intervals, 20s startup delay)
- Cancellation handler for graceful shutdown

**Router:**
- No changes needed (uses updated `ParticipantDetailsResponse`)

**Frontend:**
- Display warm keys after URL field, before Weight
- Shows grantee address (monospace font)
- Shows "Granted: [timestamp]" for each key
- Multiple warm keys displayed as list
- Loading state while fetching
- Empty state: "No warm keys configured"

## Testing

**Created `test_warm_keys.py` with 7 tests:**
- Database warm keys operations (save/retrieve)
- Warm keys replacement (delete old, insert new)
- Sorting by granted_at (descending)
- WarmKeyInfo model validation
- Authz grants parsing structure
- Empty warm keys handling
- Multiple participants with different warm keys

**All 69 backend tests passing including:**
- 7 new warm keys tests
- 2 updated model tests for ParticipantDetailsResponse

## Performance

**Polling intervals:**
- Warm keys: 5 minutes (300s)
- Epoch stats: 5 minutes
- Jail: 120s
- Health: 30s
- Rewards: 60s

**Response times:**
- Participant details with warm keys: ~100-500ms first time, <50ms cached

## Technical Details

**Pagination Pattern:**
- Uses offset-based pagination (`pagination.offset`)
- 100 grants per page
- Loops until no more grants returned

**Permission Validation:**
- Grantee must have ALL 24 required permissions
- Checks msg field in authorization object
- Format: `/inference.inference.MsgStartInference` or `/inference.bls.MsgRequestThresholdSignature`

**Sorting:**
- By `granted_at` timestamp (expiration field from grants)
- Descending order (newest first)
- Stored in database with same order

**Caching Strategy:**
- Same as rewards: background polling + lazy inline fetch on first view
- Per-epoch storage to track historical changes
- Replaces all warm keys for participant on each update

## API Structure

**Authz Grants Response:**
```json
{
  "grants": [
    {
      "granter": "gonka1...",
      "grantee": "gonka1...",
      "authorization": {
        "@type": "/cosmos.authz.v1beta1.GenericAuthorization",
        "msg": "/inference.inference.MsgStartInference"
      },
      "expiration": "2026-10-13T11:37:45.826503545Z"
    }
  ],
  "pagination": {
    "next_key": null,
    "total": "0"
  }
}
```

## Files Modified

**Backend:**
- `backend/src/backend/database.py` - table and methods
- `backend/src/backend/models.py` - WarmKeyInfo model
- `backend/src/backend/client.py` - get_authz_grants method
- `backend/src/backend/service.py` - warm key fetching and polling
- `backend/src/backend/app.py` - background polling task
- `backend/src/tests/test_warm_keys.py` - new test file
- `backend/src/tests/test_models.py` - updated tests
- `backend/scripts/test_authz_grants.py` - exploration script
- `backend/test_data/authz_grants_*.json` - sample data

**Frontend:**
- `frontend/src/types/inference.ts` - WarmKeyInfo interface
- `frontend/src/components/ParticipantModal.tsx` - warm keys display

## Notes

- Warm keys are stored per epoch to track historical changes
- A participant can have multiple valid warm keys
- All 24 permissions must be present for a grantee to be considered a warm key
- Grant expiration timestamp is used for sorting (not grant creation time)
- Empty warm key list returns None from database (consistent with other methods)

