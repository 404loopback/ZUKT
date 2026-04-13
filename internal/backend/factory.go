package backend

import (
	"fmt"

	"github.com/404loopback/zukt/internal/config"
	"github.com/404loopback/zukt/internal/zoekt"
)

func NewSearcher(cfg config.Config) (zoekt.Searcher, error) {
	httpSearcher, err := zoekt.NewHTTPSearcher(cfg.ZoektHTTPURL, cfg.ZoektTimeout)
	if err != nil {
		return nil, fmt.Errorf("build zoekt http backend: %w", err)
	}
	return httpSearcher, nil
}
