package main

import (
	"fmt"
	"strings"

	"oci-cpu-shaper/pkg/adapt"
)

func loadConfig(path string) (runtimeConfig, error) {
	cfg := defaultRuntimeConfig()

	trimmed := strings.TrimSpace(path)
	if trimmed != "" {
		err := mergeRuntimeConfigFile(&cfg, trimmed)
		if err != nil {
			return runtimeConfig{}, err
		}
	}

	applyEnvOverrides(&cfg)

	err := validateRuntimeConfig(cfg)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("validate runtime config: %w", err)
	}

	err = adapt.ValidateConfig(runtimeToAdaptControllerConfig(cfg))
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("validate controller config: %w", err)
	}

	return cfg, nil
}
