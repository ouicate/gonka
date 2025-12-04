import os
import subprocess
import sys
import time

# Configuration
FILE_PATH = "x/inference/keeper/permissions.go"
TEST_CMD = ["go", "test", "./x/inference/keeper"]
START_LINE = 28
END_LINE = 71

def run_tests():
    # Capture output to avoid clutter, but we need to know if it passed
    try:
        # timeout to prevent hanging
        result = subprocess.run(TEST_CMD, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=60)
        return result.returncode
    except subprocess.TimeoutExpired:
        print("Timeout!")
        return 1 # Treat as failure (or should it be success? No, if it hangs it's bad, but here we want to know if it FAILS. If it timeouts, it certainly didn't PASS cleanly)

def main():
    if not os.path.exists(FILE_PATH):
        print(f"Error: {FILE_PATH} not found. Run script from inference-chain directory.")
        sys.exit(1)

    print(f"Reading {FILE_PATH}...")
    with open(FILE_PATH, "r") as f:
        lines = f.readlines()

    uncovered_lines = []

    print(f"Checking lines {START_LINE} to {END_LINE}...")
    
    for i in range(START_LINE - 1, END_LINE):
        original_line = lines[i]
        
        stripped = original_line.strip()
        if not stripped or stripped.startswith("//"):
            continue
            
        if "reflect.TypeOf" not in original_line:
            continue

        print(f"Testing line {i+1}: {stripped[:50]}...")
        
        # Comment out the line
        lines[i] = "// " + original_line
        
        with open(FILE_PATH, "w") as f:
            f.writelines(lines)
            
        # Run tests
        start = time.time()
        exit_code = run_tests()
        duration = time.time() - start
        
        if exit_code == 0:
            print(f"   -> TESTS PASSED (Uncovered!) [{duration:.2f}s]")
            uncovered_lines.append(i + 1)
        else:
            print(f"   -> Tests failed (Covered) [{duration:.2f}s]")
            
        # Restore line
        lines[i] = original_line
        with open(FILE_PATH, "w") as f:
            f.writelines(lines)

    print("\nSummary of uncovered lines (tests passed when permissions removed):")
    if not uncovered_lines:
        print("None. All lines are covered!")
    for line_num in uncovered_lines:
        print(f"Line {line_num}")

if __name__ == "__main__":
    main()
