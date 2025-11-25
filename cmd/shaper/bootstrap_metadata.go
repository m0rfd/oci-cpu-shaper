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

	updatedCfg, sizingResult, sizingErr := applyPoolSizingFromShape(
		ctx,
		cfg,
		boot.opts.mode,
		imdsClient,
	)
	if sizingErr != nil {
		boot.logger.Error("failed to size worker pool from shape", zap.Error(sizingErr))

		var empty metadataBootstrap

		return empty, exitCodeRuntimeError, false
	}

	logPoolSizing(boot.logger, sizingResult)

	cfg = updatedCfg

	logMetadataResolution(boot.logger, boot.opts.mode, metadata, cfg.OCI.Offline)

	return metadataBootstrap{
		cfg:        updatedCfg,
		metadata:   metadata,
		imdsClient: imdsClient,
	}, exitCodeSuccess, true
}
