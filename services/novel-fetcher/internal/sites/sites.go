package sites

import (
	"context"
	"errors"
	"fmt"

	"narou-viewer/services/novel-fetcher/internal/fetcher"
	"narou-viewer/services/novel-fetcher/internal/model"
)

var ErrUnsupportedSite = errors.New("unsupported site")

type Progress struct {
	Phase       string
	CurrentStep int
	TotalSteps  int
	Message     string
}

type ProgressReporter func(Progress)

type WorkFetcher interface {
	FetchToc(ctx context.Context, target string, report ProgressReporter) (model.Work, error)
	FetchEpisode(ctx context.Context, work model.Work, episode model.Episode, report ProgressReporter) (model.Episode, error)
}

type textFetcher interface {
	FetchText(ctx context.Context, rawURL string, policy fetcher.FetchPolicy) (string, error)
}

type MultiFetcher struct {
	fetchers []WorkFetcher
}

func NewMultiFetcher(fetchers ...WorkFetcher) *MultiFetcher {
	return &MultiFetcher{fetchers: fetchers}
}

func (f *MultiFetcher) FetchToc(ctx context.Context, target string, report ProgressReporter) (model.Work, error) {
	var lastUnsupported error
	for _, fetcher := range f.fetchers {
		work, err := fetcher.FetchToc(ctx, target, report)
		if err == nil {
			return work, nil
		}
		if errors.Is(err, ErrUnsupportedSite) {
			lastUnsupported = err
			continue
		}
		return model.Work{}, err
	}
	if lastUnsupported != nil {
		return model.Work{}, lastUnsupported
	}
	return model.Work{}, fmt.Errorf("%w: no site fetchers are registered", ErrUnsupportedSite)
}

func (f *MultiFetcher) FetchEpisode(ctx context.Context, work model.Work, episode model.Episode, report ProgressReporter) (model.Episode, error) {
	var lastUnsupported error
	for _, fetcher := range f.fetchers {
		fetched, err := fetcher.FetchEpisode(ctx, work, episode, report)
		if err == nil {
			return fetched, nil
		}
		if errors.Is(err, ErrUnsupportedSite) {
			lastUnsupported = err
			continue
		}
		return model.Episode{}, err
	}
	if lastUnsupported != nil {
		return model.Episode{}, lastUnsupported
	}
	return model.Episode{}, fmt.Errorf("%w: no site fetchers are registered", ErrUnsupportedSite)
}
