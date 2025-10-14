# MLNode Client - GPU and Model Management

GPU monitoring and model management endpoints for the MLNode API client.

## Error Handling

Methods return `ErrAPINotImplemented` when ML node doesn't support the endpoint (older versions):

```go
if errors.Is(err, &mlnodeclient.ErrAPINotImplemented{}) {
    // Handle unsupported API
}
```

## GPU Operations

### GetGPUDevices
`GET /api/v1/gpu/devices`

Returns CUDA device information. Empty list if no GPUs or NVML unavailable.

```go
resp, err := client.GetGPUDevices(ctx)
// resp.Devices[i]: Index, Name, TotalMemoryMB, FreeMemoryMB, UsedMemoryMB,
//                  UtilizationPercent, TemperatureC, IsAvailable, ErrorMessage
```

### GetGPUDriver
`GET /api/v1/gpu/driver`

Returns driver version info: `DriverVersion`, `CudaDriverVersion`, `NvmlVersion`

## Model Management

### CheckModelStatus
`POST /api/v1/models/status`

Check model cache status. Returns: `DOWNLOADED`, `DOWNLOADING`, `NOT_FOUND`, or `PARTIAL`

```go
model := mlnodeclient.Model{
    HfRepo: "meta-llama/Llama-2-7b-hf",
    HfCommit: nil, // nil = latest
}
status, err := client.CheckModelStatus(ctx, model)
```

Response includes `Progress` field when `DOWNLOADING` (contains `StartTime`, `ElapsedSeconds`)

### DownloadModel
`POST /api/v1/models/download`

Start async download. Max 3 concurrent downloads.
- Returns 409 if already downloading
- Returns 429 if limit reached

```go
resp, err := client.DownloadModel(ctx, model)
// resp.TaskId, resp.Status, resp.Model
```

### DeleteModel
`DELETE /api/v1/models`

Delete model or cancel download:
- `HfCommit` set: deletes specific revision
- `HfCommit` nil: deletes all versions
- Returns "deleted" or "cancelled" status

### ListModels
`GET /api/v1/models/list`

List all cached models with status.

```go
resp, err := client.ListModels(ctx)
// resp.Models[i]: Model{HfRepo, HfCommit}, Status
```

### GetDiskSpace
`GET /api/v1/models/space`

Returns `CacheSizeGB`, `AvailableGB`, `CachePath`

## Implementation

Code organization:
- `types.go` - Response types
- `errors.go` - Error types
- `gpu.go` - GPU methods
- `models.go` - Model methods
- `interface.go` - MLNodeClient interface
- `mock.go` - Mock client with state tracking and error injection
- `client.go` - Base client
- `poc.go` - PoC methods

Mock client supports full state tracking, error injection, call counters, and parameter capture for testing.