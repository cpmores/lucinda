package main

import (

	// ── Provider Import ──────────────────────────────────────────────────────────

	_ "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider/drivers/ollama"
	_ "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider/drivers/vllm"
)
