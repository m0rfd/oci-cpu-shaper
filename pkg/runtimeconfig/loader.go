package runtimeconfig

import (
	"fmt"
	"strings"

	"oci-cpu-shaper/pkg/adapt"
)

// Load returns the merged runtime configuration from defaults, YAML, and env.
func Load(path string) (Config, error) {
	cfg := Default()

	trimmed := strings.TrimSpace(path)
	if trimmed != "" {
		err := mergeRuntimeConfigFile(&cfg, trimmed)
		if err != nil {
			return Config{}, err
		}
	}

	applyEnvOverrides(&cfg)

	err := validateRuntimeConfig(cfg)
	if err != nil {
		return Config{}, fmt.Errorf("validate runtime config: %w", err)
	}

	err = adapt.ValidateConfig(cfg.ToAdaptConfig())
	if err != nil {
		return Config{}, fmt.Errorf("validate controller config: %w", err)
	}

	return cfg, nil
}
