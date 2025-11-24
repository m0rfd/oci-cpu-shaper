package main

import "errors"

var (
	errControllerIMDSRequired        = errors.New("controller factory: imds client is required")
	errControllerCompartmentRequired = errors.New(
		"controller factory: OCI compartment ID is required",
	)
	errControllerRegionRequired = errors.New("controller factory: OCI region is required")
)
