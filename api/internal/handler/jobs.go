package handler

import (
	"log/slog"
	"net/http"
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
		writeError(w, http.StatusBadRequest, "formato no soportado; usa md, txt, html, pdf, docx o epub")
		return
	}

	userID := middleware.UserID(r)
	jobID := uuid.NewString()
	objectKey := filepath.Join("originals", userID, jobID+filepath.Ext(name))

	// El archivo va directo del request a MinIO: la API no escribe nada en el
	// disco del contenedor (requisito de API sin estado).
	if err := j.storage.PutStream(r.Context(), j.storage.OriginalsBucket(), objectKey,
		file, header.Size, contentTypeFor(format)); err != nil {
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

// Cancel cancela un trabajo en curso o pendiente. Si el worker ya lo estaba
// procesando, la conversión termina pero el bundle no se publica.
func (j *Jobs) Cancel(w http.ResponseWriter, r *http.Request) {
	job, err := j.repo.GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "trabajo no encontrado")
		return
	}
	if job.UserID != middleware.UserID(r) {
		writeError(w, http.StatusForbidden, "este trabajo no te pertenece")
		return
	}
	if !job.Status.Cancelable() {
		writeError(w, http.StatusConflict, "el trabajo ya terminó; no se puede cancelar")
		return
	}

	canceled, err := j.repo.CancelJob(r.Context(), job.ID)
	if err != nil {
		internalError(w, j.log, err)
		return
	}
	if !canceled {
		// Carrera con el worker: terminó entre el GET y el UPDATE.
		writeError(w, http.StatusConflict, "el trabajo ya terminó; no se puede cancelar")
		return
	}

	updated, err := j.repo.GetJob(r.Context(), job.ID)
	if err != nil {
		internalError(w, j.log, err)
		return
	}
	j.log.Info("job cancelado", "job_id", job.ID, "estado_previo", job.Status)
	writeJSON(w, http.StatusOK, updated)
}

// Retry reencola un trabajo fallido reutilizando el original ya almacenado en
// MinIO (no hay que volver a subir el archivo). El nuevo trabajo queda
// vinculado al anterior mediante retry_of.
//
// Es idempotente: si el trabajo ya tiene un reintento, se devuelve ese mismo
// en lugar de crear uno nuevo.
func (j *Jobs) Retry(w http.ResponseWriter, r *http.Request) {
	job, err := j.repo.GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "trabajo no encontrado")
		return
	}
	if job.UserID != middleware.UserID(r) {
		writeError(w, http.StatusForbidden, "este trabajo no te pertenece")
		return
	}
	if job.Status != domain.JobStatusFailed && job.Status != domain.JobStatusCanceled {
		writeError(w, http.StatusConflict, "solo se pueden reintentar trabajos fallidos o cancelados")
		return
	}

	// Idempotencia: no crear un segundo reintento del mismo trabajo.
	if existing, found, _ := j.repo.FindRetryOf(r.Context(), job.ID); found {
		writeJSON(w, http.StatusOK, existing)
		return
	}

	retryID := uuid.NewString()
	retry, err := j.repo.CreateJob(r.Context(), repository.CreateJobParams{
		ID:           retryID,
		UserID:       job.UserID,
		OriginalName: job.OriginalName,
		Format:       job.Format,
		ObjectKey:    job.ObjectKey, // se reutiliza el original ya subido
		RetryOf:      &job.ID,
		Attempt:      job.Attempt + 1,
	})
	if err != nil {
		internalError(w, j.log, err)
		return
	}

	if err := j.publisher.Publish(r.Context(), domain.JobMessage{
		JobID:     retry.ID,
		UserID:    job.UserID,
		Format:    job.Format,
		ObjectKey: job.ObjectKey,
		Attempt:   retry.Attempt,
	}); err != nil {
		j.log.Error("retry: publish message", "error", err)
		_ = j.repo.UpdateJobStatus(r.Context(), retry.ID, domain.JobStatusFailed)
		writeError(w, http.StatusInternalServerError, "no se pudo encolar el reintento")
		return
	}

	j.log.Info("job reintentado", "job_id", retry.ID, "retry_of", job.ID, "attempt", retry.Attempt)
	writeJSON(w, http.StatusCreated, retry)
}

func contentTypeFor(format domain.DocFormat) string {
	switch format {
	case domain.FormatPDF:
		return "application/pdf"
	case domain.FormatDOCX:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case domain.FormatEPUB:
		return "application/epub+zip"
	case domain.FormatMD:
		return "text/markdown; charset=utf-8"
	case domain.FormatTXT:
		return "text/plain; charset=utf-8"
	case domain.FormatHTML:
		return "text/html; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}