package main

import (
	"context"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

type metadataBootstrap struct {
	cfg        runtimeconfig.Config
	metadata   ociMetadata
	imdsClient imds.Client
}

func stageMetadata(
	ctx context.Context,
	boot bootstrapResult,
) (metadataBootstrap, int, bool) {
	imdsClient := boot.deps.newIMDS()

	cfg, metadata, metadataErr := prepareRunMetadata(ctx, boot.cfg, imdsClient, boot.opts.mode)
	if metadataErr != nil {
		boot.logger.Error("failed to resolve oci metadata", zap.Error(metadataErr))

		var empty metadataBootstrap

		return empty, exitCodeRuntimeError, false
	}

	logMetadataResolution(boot.logger, boot.opts.mode, metadata, cfg.OCI.Offline)

	return metadataBootstrap{
		cfg:        cfg,
		metadata:   metadata,
		imdsClient: imdsClient,
	}, exitCodeSuccess, true
}
