# New supported models

The proposal introduces 3 new models:

- `Qwen/Qwen3-235B-A22B-Instruct-2507-FP8` - first big model on Gonka chain.
- `Qwen/Qwen3-32B-FP8` - fresh medium size model, replacement for `Qwen/QwQ-32B`
- `RedHatAI/Qwen2.5-7B-Instruct-quantized.w8a16` - replacement for `Qwen/Qwen2.5-7B-Instruct` (minor improvement to get rid of fully dynamic quantization)

## New values estimations

The proccess of computing thresholds are described in [thresholds_sep2025.ipynb](./thresholds_sep2025.ipynb). The version of notebook to repeate experiments can be found in [mlnode/packages/benchmarks/notebooks/thresholds_sep2025.ipynb](mlnode/packages/benchmarks/notebooks/thresholds_sep2025.ipynb).

The data to reproduce experiment is available at [link](https://drive.google.com/drive/folders/1ehpcVC0pGw0XwrchXZUxTTRy1KdhBxrz?usp=drive_link) (but it's better to recompute for sure!).



- The mlnode 3.0.9 (current version in main branch) was used during experiment.
- `Qwen/Qwen3-235B-A22B-Instruct-2507-FP8` is proposed with `--max-model-len 240000`. That allows to deploy it on 320GB VRAM (=> 8xH100 would have 2 instances of deployement).


## Release process

If this proposal is approved, node operators will be able to modify their MLNodes config to switch to new models. Transition can be done asyncronously. 

Detailed instructions for a seamless transition will be published on the official project website and announced through all relevant community channels.
