# Model Proposals Directory

## Overview

This directory contains proposals for new models to be added to the [Gonka.ai](https://gonka.ai/) blockchain AI network. Each model proposal includes comprehensive validation threshold computations and technical specifications that enable network participants to make informed voting decisions.

## Directory Structure

Each model proposal is organized in its own subdirectory containing:
- **Analysis notebook** (`<model_name>_thresholds.ipynb`): Jupyter notebook with threshold computation and data analysis
- **README.md**: Summary of validation parameters and results

## Validation Threshold Computation Process

### What is a Validation Threshold?

In the Gonka.ai network, validators verify that inference providers are running the correct model by comparing probability distributions of generated tokens. The validation threshold determines the acceptable distance between the inference and validation distributions. This threshold must be carefully calibrated to:
- **Accept honest inferences**: Allow for minor variations due to different hardware, drivers, or numerical precision
- **Reject fraudulent inferences**: Detect when a provider uses a cheaper/different model than claimed

### Methodology Overview

The validation threshold is computed through an inference-validation cycle that compares honest and fraudulent model behaviors:

1. **Honest Scenario**: Same model (e.g., FP8 precision) runs on both inference and validation servers
2. **Fraud Scenario**: Different model (e.g., INT4 quantized) runs on inference, while the correct model validates

By analyzing the distribution distances in both scenarios, we identify threshold bounds that maximize fraud detection while minimizing false positives.

### Process Workflow

#### 1. Define Model Presets

Model configurations are defined in `mlnode/packages/benchmarks/src/validation/model_presets.py`. Each preset includes:
- **Model identifier**: HuggingFace model name
- **Precision**: Quantization level (fp8, int4, int8, etc.)
- **Data type**: float16, auto, etc.
- **Additional arguments**: Deployment parameters (GPU utilization, tool parsers, max sequence length, etc.)

**Example:**
```python
QWEN3_30B_FP8 = ModelPreset(
    model='Qwen/Qwen3-30B-A3B-Instruct-2507-FP8',
    precision='fp8',
    dtype='float16',
    additional_args=[
        '--gpu-memory-utilization', '0.95',
        '--enable-auto-tool-choice',
        '--tool-call-parser', 'hermes',
        '--max_model_len', '10000',
    ],
)

QWEN3_30B_INT4 = ModelPreset(
    model='cpatonn/Qwen3-30B-A3B-Instruct-2507-AWQ-4bit',
    precision='int4',
    dtype='float16',
    additional_args=[
        '--gpu-memory-utilization', '0.95',
        '--enable-auto-tool-choice',
        '--tool-call-parser', 'hermes',
        '--max_model_len', '10000',
    ],
)
```

#### 2. Run Inference-Validation Cycles

The `mlnode/packages/benchmarks/scripts/inference.py` script orchestrates the inference-validation cycles. To customize for your model:

**Key Variables to Modify:**

```python
# Import your model presets
from validation.model_presets import YOUR_MODEL_FP8, YOUR_MODEL_INT4

# Configure run parameters
N_PROMPTS = 1000  # Number of prompts to test
run_params = RunParams(
    exp_name='your_model_name',  # Experiment identifier
    output_path='../data/inference_results',
    n_prompts=N_PROMPTS,
    timeout=1800,
    tokenizer_model_name='unsloth/llama-3-8b-Instruct',
    request=RequestParams(
        max_tokens=3000,
        temperature=0.99,  # High temperature for honest runs
        seed=42,
        top_logprobs=4,  # Number of top log probabilities to record
    ),
)

# Define server configurations
server_1 = ServerConfig(
    ip='your.server.ip',
    inference_port='port',
    node_port='port',
    gpu='1xH100',  # GPU configuration descriptor
)

# Define inference-validation runs
runs = [
    # Honest run: Same model on both sides
    InferenceValidationRun(
        model_inference=YOUR_MODEL_FP8,
        model_validation=YOUR_MODEL_FP8,
        server_inference=server_1,
        server_validation=server_2,
        run_inference=get_run_params(0.99, N_PROMPTS),
        run_validation=get_run_params(0.99, N_PROMPTS),
        max_workers=10,
    ),
    # Fraud run: Different model on inference side
    InferenceValidationRun(
        model_inference=YOUR_MODEL_INT4,  # Fraudulent model
        model_validation=YOUR_MODEL_FP8,  # Correct validator
        server_inference=server_1,
        server_validation=server_2,
        run_inference=get_run_params(0.7, N_PROMPTS//5),
        run_validation=get_run_params(0.7, N_PROMPTS//5),
        max_workers=10,
    ),
]
```

**What Happens During Execution:**

1. **Model Deployment**: Both inference and validation models are deployed to their respective servers
2. **Prompt Processing**: The script processes prompts from multilingual datasets (English, Spanish, Chinese, Hindi, Arabic)
3. **Token Generation**: For each prompt, the inference model generates tokens with top-k log probabilities
4. **Validation**: The validation model independently generates the same sequence and records its probabilities
5. **Distance Calculation**: Token distribution distances are computed at each position
6. **Results Storage**: Output files are saved to `mlnode/packages/benchmarks/data/inference_results/`:
   - `<config_name>.jsonl`: Full inference-validation results
   - `<config_name>_config.json`: Run configuration for reproducibility

#### 3. Analyze Results and Compute Thresholds

The `mlnode/packages/benchmarks/notebooks/analysis.ipynb` notebook processes the inference results to determine optimal thresholds.

**Key Variables to Modify:**

```python
# Set your model identifier
model_name = 'your_model_name'

# Specify data file paths
honest_data_paths = [
    '../data/inference_results/YourModel-fp8-1xH100___YourModel-fp8-1xH100.jsonl',
    # Add more honest comparison runs (different hardware combinations)
]

fraud_data_paths = [
    '../data/inference_results/YourModel-int4-1xH100___YourModel-fp8-1xH100.jsonl',
    # Add more fraud detection runs
]
```

**Analysis Steps:**

1. **Load Data**: Import JSONL files containing inference-validation distances
2. **Process Distributions**: Extract distance metrics and classify by scenario (honest/fraud)
3. **Optimize Bounds**: Use parallel search to find threshold bounds that maximize F1-score:
   - **Lower bound**: Minimum distance considered suspicious
   - **Upper bound**: Maximum distance before flagging as fraud
4. **Visualize Results**: Generate three key plots:
   - **Honest Classification**: Distribution of distances for legitimate inferences
   - **Fraud Classification**: Distribution of distances for fraudulent inferences
   - **Length vs Distance**: Relationship between response length and validation distance

**Output Metrics:**
- Optimal lower and upper threshold bounds
- Fraud detection rate (percentage of fraud cases correctly identified)
- Mean and standard deviation by hardware configuration


## Inference-Validation Distance Metric

The validation system compares probability distributions using a token distance function that:
1. Computes the L2 distance between log-probability vectors at each token position
2. Averages distances across all tokens in the generated sequence
3. Handles missing tokens and top-k probability selections

This metric is sensitive enough to detect model substitution while robust to acceptable hardware variations.

## Hardware Considerations

Validation thresholds account for legitimate distance variations arising from:
- **GPU Architecture**: Different NVIDIA GPUs (H100, H200, RTX 3090) may produce slightly different floating-point results
- **Tensor Parallelism**: Distributed inference across multiple GPUs can affect numerical precision
- **Driver Versions**: CUDA and inference framework versions may impact computation
- **Quantization**: Same quantization method (e.g., FP8) can have minor implementation differences

The threshold computation process tests multiple hardware combinations to ensure robustness.

## Best Practices

1. **Sufficient Sample Size**: Use at least 1000 honest samples and 200 fraud samples per configuration
2. **Temperature Diversity**: Test with high temperature (0.99) for honest runs to maximize distribution variance
3. **Hardware Diversity**: Validate across multiple GPU types to ensure threshold generalization
4. **Multilingual Testing**: Include prompts from multiple languages to ensure model performance consistency
5. **Conservative Thresholds**: Bias toward accepting honest behavior to avoid false fraud accusations
6. **Documentation**: Maintain detailed records of all configurations, data sources, and computed thresholds