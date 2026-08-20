package handler

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"okf/api/internal/middleware"
	"okf/api/internal/repository"
	"okf/api/internal/storage"
	"okf/pkg/domain"
)

type bundleFile struct {
	Path string `json:"path"`
	URL  string `json:"url"` // ruta relativa a la API: /api/v1/jobs/{id}/bundle/{path}
}

type bundleResponse struct {
	Path string `json:"path"`
	// Validation clasifica el resultado: "valid" | "valid_with_warnings".
	Validation domain.ValidationLevel `json:"validation"`
	Warnings   []string               `json:"warnings"`
	Files      []bundleFile           `json:"files"`
}

// bundleFilesFor lista los archivos del bundle de un job. Las URLs se sirven a
// través de la API (MinIO es privado y las URLs firmadas no sobreviven al
// cambio de host, por lo que no se exponen al navegador).
func bundleFilesFor(ctx context.Context, repo *repository.Postgres, store *storage.Storage, job domain.Job) (*bundleResponse, error) {
	bundle, err := repo.GetBundleByJob(ctx, job.ID)
	if err != nil {
		return nil, err
	}

	keys, err := store.ListObjects(ctx, store.BundlesBucket(), bundle.Path+"/")
	if err != nil {
		return nil, err
	}

	warnings := bundle.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	resp := &bundleResponse{
		Path:       bundle.Path,
		Validation: bundle.Validation,
		Warnings:   warnings,
		Files:      make([]bundleFile, 0, len(keys)),
	}
	for _, k := range keys {
		rel := strings.TrimPrefix(k, bundle.Path+"/")
		resp.Files = append(resp.Files, bundleFile{
			Path: k,
			URL:  fmt.Sprintf("/api/v1/jobs/%s/bundle/%s", job.ID, rel),
		})
	}
	return resp, nil
}

type Bundles struct {
	repo    *repository.Postgres
	storage *storage.Storage
	log     *slog.Logger
}

func NewBundles(repo *repository.Postgres, store *storage.Storage, log *slog.Logger) *Bundles {
	return &Bundles{repo: repo, storage: store, log: log}
}

// resolveCompletedJob valida autenticación y estado del job del path param "id".
func (b *Bundles) resolveCompletedJob(w http.ResponseWriter, r *http.Request) (domain.Job, bool) {
	job, err := b.repo.GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "trabajo no encontrado")
		return domain.Job{}, false
	}
	if job.UserID != middleware.UserID(r) {
		writeError(w, http.StatusForbidden, "este trabajo no te pertenece")
		return domain.Job{}, false
	}
	if job.Status != domain.JobStatusCompleted {
		writeError(w, http.StatusConflict, "el trabajo aún no ha terminado")
		return domain.Job{}, false
	}
	return job, true
}

// DownloadFile transmite un archivo individual del bundle (streaming desde MinIO).
func (b *Bundles) DownloadFile(w http.ResponseWriter, r *http.Request) {
	job, ok := b.resolveCompletedJob(w, r)
	if !ok {
		return
	}

	bundle, err := b.repo.GetBundleByJob(r.Context(), job.ID)
	if err != nil {
		internalError(w, b.log, err)
		return
	}

	rel := r.PathValue("path")
	if rel == "" || rel != filepath.ToSlash(filepath.Clean(rel)) || strings.Contains(rel, "..") {
		writeError(w, http.StatusBadRequest, "ruta de archivo del bundle inválida")
		return
	}

	objectKey := bundle.Path + "/" + rel
	obj, err := b.storage.GetObject(r.Context(), b.storage.BundlesBucket(), objectKey)
	if err != nil {
		writeError(w, http.StatusNotFound, "archivo del bundle no encontrado")
		return
	}
	defer obj.Close()

	if ct := mime.TypeByExtension(filepath.Ext(rel)); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filepath.Base(rel)))
	if _, err := io.Copy(w, obj); err != nil {
		b.log.Error("bundles: stream file", "key", objectKey, "error", err)
	}
}

// DownloadZip comprime el bundle completo en un .zip y lo transmite al cliente.
func (b *Bundles) DownloadZip(w http.ResponseWriter, r *http.Request) {
	job, ok := b.resolveCompletedJob(w, r)
	if !ok {
		return
	}

	bundle, err := b.repo.GetBundleByJob(r.Context(), job.ID)
	if err != nil {
		internalError(w, b.log, err)
		return
	}

	keys, err := b.storage.ListObjects(r.Context(), b.storage.BundlesBucket(), bundle.Path+"/")
	if err != nil {
		internalError(w, b.log, err)
		return
	}
	if len(keys) == 0 {
		writeError(w, http.StatusNotFound, "el bundle está vacío")
		return
	}

	name := strings.TrimSuffix(job.OriginalName, filepath.Ext(job.OriginalName))
	zipName := fmt.Sprintf("%s-okf.zip", name)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", zipName))

	zw := zip.NewWriter(w)
	defer zw.Close()
	for _, k := range keys {
		rel := strings.TrimPrefix(k, bundle.Path+"/")
		fw, err := zw.Create(rel)
		if err != nil {
			b.log.Error("bundles: create zip entry", "path", rel, "error", err)
			return
		}
		obj, err := b.storage.GetObject(r.Context(), b.storage.BundlesBucket(), k)
		if err != nil {
			b.log.Error("bundles: open object", "key", k, "error", err)
			return
		}
		if _, err := io.Copy(fw, obj); err != nil {
			obj.Close()
			b.log.Error("bundles: stream object", "key", k, "error", err)
			return
		}
		obj.Close()
	}
}