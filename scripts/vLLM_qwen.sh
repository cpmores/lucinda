#!/bin/bash

MODEL_PATH=~/models/Qwen2.5-7B-Instruct-GPTQ-Int4
HOST=0.0.0.0
PORT=8000
MODEL_NAME=qwen-2.5-gptq
MAX_LEN=2048
MEM_UTIL=0.75
QUANT=gptq

vllm serve "$MODEL_PATH" \
    --host "$HOST" \
    --port "$PORT" \
    --served-model-name "$MODEL_NAME" \
    --max-model-len "$MAX_LEN" \
    --gpu-memory-utilization "$MEM_UTIL" \
    --quantization "$QUANT" \
    --enforce-eager \
    --max-num-seqs 8
