package engine

import "github.com/GrayCodeAI/eyrie/operationsgraph"

// OperationsGraphInput is the host-facing input for Eyrie's portable
// operations graph projection.
type OperationsGraphInput = operationsgraph.Input

// OperationsGraphExport is Eyrie's portable operations graph projection.
type OperationsGraphExport = operationsgraph.Export

// BuildOperationsGraph projects route and normalized usage telemetry without
// exposing provider, model, request, or generated content values.
func BuildOperationsGraph(input OperationsGraphInput) (*OperationsGraphExport, error) {
	return operationsgraph.Build(input)
}
