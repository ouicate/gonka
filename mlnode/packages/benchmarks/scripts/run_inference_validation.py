import sys
sys.path.append('../src')
sys.path.append('../../common/src')
from validation.runner import run_validation
from validation.prompts import get_alpaca_data_questions
from validation.data import (
    ModelInfo,
    RequestParams,
    save_to_jsonl
)
import requests
from time import sleep 
from transformers import AutoTokenizer
import os
import yaml

from concurrent.futures import ThreadPoolExecutor

# Before running the script each server has an mlnode running.
# Below is an example for qwen3-235B inference-validation cycle using two 8xH100 servers.

# Load configuration from YAML
config_path = os.path.join(os.path.dirname(__file__), '..', 'resources', 'inference_validation.yml')
with open(config_path, 'r') as f:
    cfg = yaml.safe_load(f)

servers = cfg['servers']
models_cfg = cfg['models']

MAX_TOKENS = cfg['run']['request']['max_tokens']
TEMPERATURE = cfg['run']['request']['temperature']
SEED = cfg['run']['request']['seed']
LOGPROBS = cfg['run']['request']['top_logprobs']

# Urls needed to deploy and use the models
inference_up_url = lambda server:  f"http://{server['ip']}:{server['node_port']}/api/v1/inference/up"
inference_url = lambda server: f"http://{server['ip']}:{server['inference_port']}/"

# Inference-validation cycle is done on the first n_prompts prompts of the dataset.
prompts = get_alpaca_data_questions()
tokenizer_model_name = cfg['run'].get('tokenizer_model_name', "unsloth/llama-3-8b-Instruct")
tokenizer = AutoTokenizer.from_pretrained(tokenizer_model_name)

# Models are defined in the YAML config (cfg['models']).
# Settings for the inference-validation cycle: (inference_model, validation_model), (server_1, server_2)
# Inference is run on server_1 and validation is run on server_2.
settings = []
for s in cfg['settings']:
    inf_cfg = models_cfg[s['inference_model']]
    val_cfg = models_cfg[s['validation_model']]
    s1 = servers[s['server_1']]
    s2 = servers[s['server_2']]
    settings.append(((inf_cfg, val_cfg), (s1, s2)))

os.makedirs(cfg['run']['output_path'], exist_ok=True)

def post_up(server, payload):
    response = requests.post(inference_up_url(server), json=payload, timeout=timeout)
    response.raise_for_status()
    return server, response


exp_name = cfg['run']['exp_name']
batch_size = cfg['run']['batch_size']
n_prompts = cfg['run']['n_prompts']
timeout = cfg['run']['timeout']


# Helper to determine whether two server configs point to the same server
def servers_equal(a, b):
    return (
        a.get('ip') == b.get('ip') and
        a.get('node_port') == b.get('node_port') and
        a.get('inference_port') == b.get('inference_port')
    )


# Run the inference-validation cycle for different settings
# each iteration consists of the fllowing steps:
# 1. Deploy the models on the servers
# 2. Run the inference-validation cycle using run_validation function
# 3. Save the results

for (inference_config, validation_config), (s1, s2) in settings:
    
    inference_payload = {"model": inference_config['model'], 
                         "dtype": inference_config['dtype'], 
                         "additional_args": inference_config["additional_args"]}
    validation_payload = {"model": validation_config['model'], 
                          "dtype": validation_config['dtype'],
                          "additional_args": validation_config["additional_args"]}
    # Deploy models, avoiding duplicate deployment when targeting the same server with identical payloads
    up_requests = []
    if servers_equal(s1, s2) and inference_payload == validation_payload:
        up_requests = [(s1, inference_payload)]
    else:
        up_requests = [(s1, inference_payload), (s2, validation_payload)]

    with ThreadPoolExecutor(max_workers=len(up_requests)) as executor:
        futures = [executor.submit(post_up, srv, payload) for srv, payload in up_requests]
        for future in futures:
            server, response = future.result()
            print(response.status_code)
            print(response.text)
    
    sleep(20)
    
    inference_model_info = ModelInfo(
        url=inference_url(s1),
        name=inference_config['model'],
        deploy_params={
            "GPU": s1['gpu'],
            "precision": inference_config['precision'],
        }
    )

    validation_model_info = ModelInfo(
        url=inference_url(s2),
        name=validation_config['model'],
        deploy_params={
            "GPU": s2['gpu'],
            "precision": validation_config['precision'],
        }
    )
    
    request_params = RequestParams(
        max_tokens=MAX_TOKENS,
        temperature=TEMPERATURE,
        seed=SEED,
        top_logprobs=LOGPROBS
    )

    inf_model_name = f"{inference_config['model'].split('/')[-1]}-{inference_config['precision']}-{s1['gpu']}"
    val_model_name = f"{validation_config['model'].split('/')[-1]}-{validation_config['precision']}-{s2['gpu']}"
    setting_name = f"{inf_model_name}___{val_model_name}__{exp_name}.jsonl"
    DATA_PATH = f'{cfg["run"]["output_path"]}/{setting_name}'

    for start_idx in range(0, len(prompts[:n_prompts]), batch_size):
        prompt_batch = prompts[start_idx:start_idx + batch_size]
        results_batch = run_validation(
            prompt_batch,
            inference_model=inference_model_info,
            validation_model=validation_model_info,
            request_params=request_params,
            max_workers=50
        )
        save_to_jsonl(results_batch, DATA_PATH, append=False)
        print(f"Processed {start_idx + batch_size} from {len(prompts)}")