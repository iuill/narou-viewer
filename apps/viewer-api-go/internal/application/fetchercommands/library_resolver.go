package fetchercommands

import (
	"strconv"

	"narou-viewer/apps/viewer-api-go/internal/library"
)

type LibraryWorkIDResolver struct {
	library *library.Service
}

func NewLibraryWorkIDResolver(libraryService *library.Service) LibraryWorkIDResolver {
	return LibraryWorkIDResolver{library: libraryService}
}

func (r LibraryWorkIDResolver) FetcherWorkID(novelID string) (string, bool, error) {
	if r.library == nil {
		return "", false, ErrWorkIDResolverUnavailable
	}
	work, ok, err := r.library.FindWork(novelID)
	if err != nil || !ok {
		return "", ok, err
	}
	return strconv.Itoa(work.ID), true, nil
}
