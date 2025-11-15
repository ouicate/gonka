# Power Capping Algorithm Test Wrapper

This test wrapper allows you to easily test the `CalculateOptimalCap` function with custom weights and max percentages.

## Quick Start

### Option 1: Run Pre-defined Tests

Run all predefined test cases:
```bash
cd inference-chain/x/inference/keeper
go test -v -run TestCalculateOptimalCapWrapper
```

Run edge case tests:
```bash
go test -v -run TestEdgeCases
```

### Option 2: Test with Custom Values

The easiest way to test your own values is to modify `TestCustomCap` in `bitcoin_rewards_cap_test.go`:

1. Open `bitcoin_rewards_cap_test.go`
2. Find the `TestCustomCap` function (around line 127)
3. Modify these lines:
   ```go
   weights := []int64{100, 200, 300, 400}  // Your participant weights
   maxPercentage := 0.30                    // Max percentage (e.g., 0.30 = 30%)
   ```
4. Run:
   ```bash
   go test -v -run TestCustomCap
   ```

### Option 3: Use the Helper Function

You can also use the `RunCapTest` helper function in your own test:

```go
func TestMyScenario(t *testing.T) {
    weights := []int64{50, 150, 300, 500}
    maxPercentage := 0.25
    
    result := RunCapTest(weights, maxPercentage)
    t.Log(result)
}
```

## Understanding the Output

The test output shows:
- **Input Weights**: The original participant weights
- **Max Percentage**: The maximum allowed percentage for any single participant
- **Total Power (before)**: Sum of all original weights
- **Was Capped**: Whether any capping was applied
- **Results**: For each participant:
  - Original weight → Capped weight
  - Whether it was capped
  - Percentage of total after capping
- **Total Power (after)**: Sum of all weights after capping
- **Actual Max Percentage**: The highest percentage any participant has after capping

### Example Output

```
=== Power Capping Test ===
Input Weights: [50 50 900]
Max Percentage: 30.00%
Total Power (before): 1000
Was Capped: true

Results:
  Participant 0: 50 -> 50 (unchanged, 16.67% of total)
  Participant 1: 50 -> 50 (unchanged, 16.67% of total)
  Participant 2: 900 -> 200 (CAPPED, 66.67% of total)

Total Power (after): 300
Actual Max Percentage: 66.67%
```

## Pre-defined Test Scenarios

The test suite includes several scenarios:

### Basic Tests (`TestCalculateOptimalCapWrapper`)
- Basic 4 participants
- High concentration (one dominant participant)
- No capping needed (all equal)
- Small network (2 participants)
- Extreme concentration

### Edge Cases (`TestEdgeCases`)
- Single participant
- Two equal participants
- Very small weights
- Large weights
- Many participants with one dominant
- Different cap percentages (50%, 40%)

## Performance Testing

Run benchmarks to test performance:
```bash
go test -bench=BenchmarkCalculateOptimalCap -benchmem
```

## Function Signature

```go
func CalculateOptimalCap(
    participants []*types.ActiveParticipant, 
    totalPower int64, 
    maxPercentage *types.Decimal
) ([]*types.ActiveParticipant, int64, bool)
```

**Parameters:**
- `participants`: List of participants with their weights
- `totalPower`: Sum of all participant weights
- `maxPercentage`: Maximum allowed percentage (as types.Decimal)

**Returns:**
- Capped participants list
- New total power after capping
- Whether capping was applied (bool)

## Tips

- The algorithm ensures no single participant exceeds the max percentage of total power
- For small networks (< 4 participants), higher limits are automatically applied:
  - 1 participant: 100% (no cap)
  - 2 participants: 50%
  - 3 participants: 40%
  - 4+ participants: 30% (default)
- The algorithm maintains proportional relationships while enforcing the cap
- All integer division remainders are handled to ensure exact distribution




