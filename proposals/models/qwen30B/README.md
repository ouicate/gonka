# Qwen30B Model Validation

## Thresholds

- **Lower bound**: 0.0262
- **Upper bound**: 0.0272
- **Fraud detection rate**: 54%

## Model Preset

**Honest (FP8)**: `Qwen/Qwen3-30B-A3B-Instruct-2507-FP8`
- Precision: `fp8`, dtype: `float16`
- Args: `--gpu-memory-utilization 0.95 --enable-auto-tool-choice --tool-call-parser hermes --max_model_len 10000`

**Fraud (INT4)**: `cpatonn/Qwen3-30B-A3B-Instruct-2507-AWQ-4bit`
- Precision: `int4`, dtype: `float16`
- Args: `--gpu-memory-utilization 0.95 --enable-auto-tool-choice --tool-call-parser hermes --max_model_len 10000`

## Comparisons

Honest inference-validation cycles
- **H100 vs H100**: Main use case, 1000 honest samples
- **2x3090 vs H100**: Distribution comparison, 200 samples
- **2x3090 vs 2x3090**: Distribution comparison, 200 samples

Fraud inference-validation cycles
- **H100 vs H100**: 200 fraud samples evaluated for cross-device consistency
- **2x3090 vs H100**: 200 fraud samples tested to assess robustness across hardware


## Results

See analysis in `qwen30B_analysis.ipynb`:
- `qwen_30B_length_vs_distance.png` - Length vs distance scatter plot
- `qwen_30B_fraud_classification.png` - Fraud detection illustration
- `qwen_30B_honest_classification.png` - Honest classification illustration

## Data

All the relevant data, including deployment configs could be found in [drive](https://drive.google.com/drive/folders/1JqZ4wFsOr-RRZStk5bhnWppiV5g_-SpY?usp=sharing) 