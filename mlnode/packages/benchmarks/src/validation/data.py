from pydantic import (
    BaseModel,
    Field,
)
from typing import (
    List,
    Dict,
    Union
)
import pandas as pd


class PositionResult(BaseModel):
    token: str
    logprobs: Dict[str, float]


class Result(BaseModel):
    text: str
    results: List[PositionResult]


class ModelInfo(BaseModel):
    name: str
    url: str
    deploy_params: Dict[str, str] = Field(default_factory=dict)


class RequestParams(BaseModel):
    max_tokens: int
    temperature: float
    seed: int
    additional_params: Dict[str, Union[str, int, float]] = Field(default_factory=dict)
    top_logprobs: int = 3


class ValidationItem(BaseModel):
    prompt: str
    inference_result: Result
    validation_result: Result
    inference_model: ModelInfo
    validation_model: ModelInfo
    request_params: RequestParams

    def to_dict(self):
        return self.model_dump()


class ExperimentRequest(BaseModel):
    prompt: str
    inference_model: ModelInfo
    validation_model: ModelInfo
    request_params: RequestParams

    def to_result(self, inference_result: Result, validation_result: Result) -> ValidationItem:
        return ValidationItem(
            prompt=self.prompt,
            inference_result=inference_result,
            validation_result=validation_result,
            inference_model=self.inference_model,
            validation_model=self.validation_model,
            request_params=self.request_params
        )


def items_to_df(validation_results: List[ValidationItem]) -> pd.DataFrame:
    return pd.DataFrame([item.model_dump() for item in validation_results])


def df_to_items(df: pd.DataFrame) -> List[ValidationItem]:
    return [ValidationItem.model_validate(row) for row in df.to_dict(orient='records')]

def save_to_jsonl(
    validation_results: List[ValidationItem],
    path: str,
    append: bool = False
):
    mode = 'a' if append else 'w'
    with open(path, mode) as f:
        for result in validation_results:
            f.write(result.model_dump_json() + '\n')


def load_from_jsonl(
    path: str,
    n: int = None
) -> List[ValidationItem]:
    k = n if n is not None else float('inf')
    results = []
    with open(path, 'r') as f:
        for i, line in enumerate(f):
            if i >= k:
                break
            results.append(ValidationItem.model_validate_json(line))
    return results


class ModelPreset(BaseModel):
    model: str
    precision: str
    dtype: str
    additional_args: List[str] = Field(default_factory=list)

    def to_deploy_payload(self) -> Dict[str, Union[str, List[str]]]:
        return {
            "model": self.model,
            "dtype": self.dtype,
            "additional_args": self.additional_args,
        }


class ServerConfig(BaseModel):
    ip: str
    node_port: str
    inference_port: str
    gpu: str

    def up_url(self) -> str:
        return f"http://{self.ip}:{self.node_port}/api/v1/inference/up"

    def inference_url(self) -> str:
        return f"http://{self.ip}:{self.inference_port}/"


class RunParams(BaseModel):
    exp_name: str
    output_path: str
    batch_size: int
    n_prompts: int
    timeout: int
    tokenizer_model_name: str = "unsloth/llama-3-8b-Instruct"
    request: RequestParams


class InferenceValidationRun(BaseModel):
    model_inference: ModelPreset
    model_validation: ModelPreset
    server_inference: ServerConfig
    server_validation: ServerConfig
    run_inference: RunParams
    run_validation: RunParams

    def setting_filename(self) -> str:
        inf_model_name = f"{self.model_inference.model.split('/')[-1]}-{self.model_inference.precision}-{self.server_inference.gpu}"
        val_model_name = f"{self.model_validation.model.split('/')[-1]}-{self.model_validation.precision}-{self.server_validation.gpu}"
        return f"{inf_model_name}___{val_model_name}__{self.run_inference.exp_name}.jsonl"

