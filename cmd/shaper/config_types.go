package main

import "time"

type runtimeConfig struct {
	Controller controllerConfig
	Estimator  estimatorConfig
	Pool       poolConfig
	HTTP       httpConfig
	OCI        ociConfig
}

type controllerConfig struct {
	TargetStart       float64
	TargetMin         float64
	TargetMax         float64
	StepUp            float64
	StepDown          float64
	FallbackTarget    float64
	GoalLow           float64
	GoalHigh          float64
	Interval          time.Duration
	RelaxedInterval   time.Duration
	RelaxedThreshold  float64
	SuppressThreshold float64
	SuppressResume    float64
}

type estimatorConfig struct {
	Interval time.Duration
}

type poolConfig struct {
	Workers         int
	Quantum         time.Duration
	PauseThreshold  float64
	ResumeThreshold float64
}

type httpConfig struct {
	Bind string
}

type ociConfig struct {
	CompartmentID string
	Region        string
	InstanceID    string
	Offline       bool
}

type fileConfig struct {
	Controller controllerFileConfig `yaml:"controller"`
	Estimator  estimatorFileConfig  `yaml:"estimator"`
	Pool       poolFileConfig       `yaml:"pool"`
	HTTP       httpFileConfig       `yaml:"http"`
	OCI        ociFileConfig        `yaml:"oci"`
}

type controllerFileConfig struct {
	TargetStart       *float64       `yaml:"targetStart"`
	TargetMin         *float64       `yaml:"targetMin"`
	TargetMax         *float64       `yaml:"targetMax"`
	StepUp            *float64       `yaml:"stepUp"`
	StepDown          *float64       `yaml:"stepDown"`
	FallbackTarget    *float64       `yaml:"fallbackTarget"`
	GoalLow           *float64       `yaml:"goalLow"`
	GoalHigh          *float64       `yaml:"goalHigh"`
	Interval          *time.Duration `yaml:"interval"`
	RelaxedInterval   *time.Duration `yaml:"relaxedInterval"`
	RelaxedThreshold  *float64       `yaml:"relaxedThreshold"`
	SuppressThreshold *float64       `yaml:"suppressThreshold"`
	SuppressResume    *float64       `yaml:"suppressResume"`
}

type estimatorFileConfig struct {
	Interval *time.Duration `yaml:"interval"`
}

type poolFileConfig struct {
	Workers         *int           `yaml:"workers"`
	Quantum         *time.Duration `yaml:"quantum"`
	PauseThreshold  *float64       `yaml:"pauseThreshold"`
	ResumeThreshold *float64       `yaml:"resumeThreshold"`
}

type httpFileConfig struct {
	Bind *string `yaml:"bind"`
}

type ociFileConfig struct {
	CompartmentID *string `yaml:"compartmentId"`
	Region        *string `yaml:"region"`
	InstanceID    *string `yaml:"instanceId"`
	Offline       *bool   `yaml:"offline"`
}
