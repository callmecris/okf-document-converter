package handler

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"okf/api/internal/middleware"
	"okf/api/internal/queue"
	"okf/api/internal/repository"
	"okf/api/internal/storage"
	"okf/pkg/domain"
)

const maxUploadSize = 100 << 20 // 100 MB

type Jobs struct {
	repo      *repository.Postgres
	storage   *storage.Storage
	publisher *queue.Publisher
	log       *slog.Logger
}

func NewJobs(repo *repository.Postgres, store *storage.Storage, pub *queue.Publisher, log *slog.Logger) *Jobs {
	return &Jobs{repo: repo, storage: store, publisher: pub, log: log}
}

type jobDetail struct {
	domain.Job
	Bundle *bundleResponse `json:"bundle,omitempty"`
}

// Create recibe un multipart "file", sube el original a MinIO y encola el trabajo.
func (j *Jobs) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeError(w, http.StatusRequestEntityTooLarge, "el archivo supera el tamaño máximo permitido")
			return
		}
		writeError(w, http.StatusBadRequest, "formulario multipart inválido")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "falta el campo 'file'")
		return
	}
	defer file.Close()

	name := filepath.Base(header.Filename)
	format, ok := domain.ParseFormat(filepath.Ext(name))
	if !ok {
		writeError(w, http.StatusBadRequest, "formato no soportado; usa pdf, docx o epub")
		return
	}

	userID := middleware.UserID(r)
	jobID := uuid.NewString()
	objectKey := filepath.Join("originals", userID, jobID+filepath.Ext(name))

	tmp, err := os.CreateTemp("", "okf-upload-*"+filepath.Ext(name))
	if err != nil {
		internalError(w, j.log, err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		internalError(w, j.log, err)
		return
	}
	if err := tmp.Close(); err != nil {
		internalError(w, j.log, err)
		return
	}

	if err := j.storage.PutFile(r.Context(), j.storage.OriginalsBucket(), objectKey, tmp.Name(), contentTypeFor(format)); err != nil {
		internalError(w, j.log, err)
		return
	}

	job, err := j.repo.CreateJob(r.Context(), repository.CreateJobParams{
		ID:           jobID,
		UserID:       userID,
		OriginalName: name,
		Format:       format,
		ObjectKey:    objectKey,
	})
	if err != nil {
		j.log.Error("job: create job", "error", err)
		writeError(w, http.StatusInternalServerError, "no se pudo crear el trabajo")
		return
	}

	if err := j.publisher.Publish(r.Context(), domain.JobMessage{
		JobID:     job.ID,
		UserID:    userID,
		Format:    format,
		ObjectKey: objectKey,
		Attempt:   1,
	}); err != nil {
		j.log.Error("job: publish message", "error", err)
		_ = j.repo.UpdateJobStatus(r.Context(), job.ID, domain.JobStatusFailed)
		writeError(w, http.StatusInternalServerError, "no se pudo encolar el trabajo")
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

// List devuelve los trabajos del usuario autenticado.
func (j *Jobs) List(w http.ResponseWriter, r *http.Request) {
	jobs, err := j.repo.ListJobs(r.Context(), middleware.UserID(r))
	if err != nil {
		internalError(w, j.log, err)
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

// Get devuelve un trabajo con su bundle (archivos firmados) si ya terminó.
func (j *Jobs) Get(w http.ResponseWriter, r *http.Request) {
	job, err := j.repo.GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "trabajo no encontrado")
		return
	}
	if job.UserID != middleware.UserID(r) {
		writeError(w, http.StatusForbidden, "este trabajo no te pertenece")
		return
	}

	resp := jobDetail{Job: job}
	if job.Status == domain.JobStatusCompleted {
		bundle, err := bundleFilesFor(r.Context(), j.repo, j.storage, job)
		if err != nil {
			internalError(w, j.log, err)
			return
		}
		resp.Bundle = bundle
	}
	writeJSON(w, http.StatusOK, resp)
}

func contentTypeFor(format domain.DocFormat) string {
	switch format {
	case domain.FormatPDF:
		return "application/pdf"
	case domain.FormatDOCX:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case domain.FormatEPUB:
		return "application/epub+zip"
	default:
		return "application/octet-stream"
	}
}