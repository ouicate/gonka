# Qwen3-30B Model Proposal

This is a proposal to add the [Qwen3-30B-A3B-Instruct-2507-FP8](https://huggingface.co/Qwen/Qwen3-30B-A3B-Instruct-2507-FP8) model to the Gonka inference network. It is a 30B MoE LLM with 3B active parameters quantized to FP8.

Key points:
- Requiring $\geq$ 48GB of VRAM, it is deployable on 2 x RT3090 / 1 x H100, equivalent or higher grade GPUs.
- All experiments were conducted using MLNode v3.0.8.
- Validation threshold computation overview:
    - Inferences validation is tested against INT-4 quantized version [Qwen3-30B-A3B-Instruct-2507-AWQ-4bit](https://huggingface.co/cpatonn/Qwen3-30B-A3B-Instruct-2507-AWQ-4bit).
    - Fraud detection accuracy: **54%**
    - Validation threshold bounds: **`(0.026, 0.027)`**. 
    - Detailed report on the validation threshold can be found in [qwen30B_thresholds.ipynb](./qwen30B_thresholds.ipynb). 
        - The validation threshold was computed using the standard procedure described in [models/README.md](../README.md).
    - The data used for our experiments is available for verification: [Qwen30B Validation Data](https://drive.google.com/drive/folders/1JqZ4wFsOr-RRZStk5bhnWppiV5g_-SpY?usp=sharing).
- It supports tool calling with `hermes` tool call parser, so the model is suggested to be deployed with with the following parameters:
    ```python
    additional_args=[
        '--enable-auto-tool-choice',  # Optional: enables automatic tool choice
        '--tool-call-parser', 'hermes',  # Optional: specifies the Hermes tool call parser
    ]
    ```



