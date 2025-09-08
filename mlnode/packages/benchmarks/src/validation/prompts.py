from datasets import load_dataset
from typing import List


def get_squad_data_questions() -> List[str]:
    dataset = load_dataset('squad', keep_in_memory=True)
    prompts = []
    train_prompts = [f"Context: {context}\nQuestion: {question} " for question, context in zip(dataset['train']['question'], dataset['train']['context'])]
    prompts.extend(train_prompts)
    validation_prompts = [f"Context: {context}\nQuestion: {question} " for question, context in zip(dataset['validation']['question'], dataset['validation']['context'])]
    prompts.extend(validation_prompts)
    return prompts


def get_alpaca_data_questions() -> List[str]:
    dataset = load_dataset('tatsu-lab/alpaca', keep_in_memory=True)
    prompts = []
    train_prompts = [f"Instruction: {instruction} " for instruction in dataset['train']['instruction']]
    prompts.extend(train_prompts)
    return prompts
