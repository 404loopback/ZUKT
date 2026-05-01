package search

import "context"

func (s *Service) PrepareSemanticIndex(ctx context.Context, repo string) (SemanticIndexStats, error) {
	repoRef, err := s.resolveRepo(ctx, repo)
	if err != nil {
		return SemanticIndexStats{}, err
	}
	if s.semanticBackend == nil {
		return SemanticIndexStats{
			Repo:    repoRef.Name,
			Backend: "disabled",
		}, nil
	}
	stats, err := s.semanticBackend.Prepare(ctx, repoRef)
	if err != nil {
		return SemanticIndexStats{}, err
	}
	return semanticStatsOrEmpty(stats), nil
}

func (s *Service) SemanticStatus(ctx context.Context) (*SemanticStatus, error) {
	if s.semanticBackend == nil {
		return &SemanticStatus{Backend: "disabled", Enabled: false, Ready: false, Message: "semantic backend disabled"}, nil
	}
	return s.semanticBackend.Status(ctx)
}
