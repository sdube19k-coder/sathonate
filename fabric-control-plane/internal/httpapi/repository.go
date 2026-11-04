package httpapi

import (
	"net/http"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/drafts"
)

type Repository interface {
	List() ([]drafts.Draft, error)
	Get(string) (drafts.Draft, error)
	Save(domain.NetworkSpec, string, int) (drafts.Draft, error)
	Review(string, int) (drafts.Draft, error)
	Archive(string, int) error
}
type RepositoryResolver func(*http.Request) Repository
type fileRepository struct{ *drafts.Store }

func (f fileRepository) List() ([]drafts.Draft, error) { return f.Store.List(), nil }
func FileResolver(store *drafts.Store) RepositoryResolver {
	return func(*http.Request) Repository { return fileRepository{store} }
}
