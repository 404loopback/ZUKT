package search

import "context"

type Embedder interface {
	Embed(ctx context.Context, input string) ([]float32, error)
	EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error)
	Dimension() int
	Model() string
}
