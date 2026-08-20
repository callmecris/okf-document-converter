// Package domain contiene los modelos y enums compartidos entre la API y los
// workers. No depende de infraestructura (DB, cola, almacenamiento).
package domain

import (
	"strings"
	"time"
)

// DocFormat es un formato de documento de entrada soportado.
type DocFormat string

const (
	FormatPDF  DocFormat = "pdf"
	FormatDOCX DocFormat = "docx"
	FormatEPUB DocFormat = "epub"
	// Formatos de texto con estructura detectable por encabezados.
	FormatMD   DocFormat = "md"
	FormatTXT  DocFormat = "txt"
	FormatHTML DocFormat = "html"
)

// ParseFormat normaliza una extensión de archivo (".PDF", "pdf") al formato
// correspondiente. El segundo valor es false si el formato no está soportado.
func ParseFormat(ext string) (DocFormat, bool) {
	switch DocFormat(strings.ToLower(strings.TrimPrefix(ext, "."))) {
	case FormatPDF:
		return FormatPDF, true
	case FormatDOCX:
		return FormatDOCX, true
	case FormatEPUB:
		return FormatEPUB, true
	case FormatMD, "markdown":
		return FormatMD, true
	case FormatTXT, "text":
		return FormatTXT, true
	case FormatHTML, "htm":
		return FormatHTML, true
	default:
		return "", false
	}
}

// JobStatus es el estado del ciclo de vida de un trabajo de conversión.
// Los valores coinciden con el CHECK de scripts/init.sql.
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	// JobStatusCanceled: cancelado por el usuario. Si la conversión ya estaba
	// en curso, el worker la termina pero no publica el bundle.
	JobStatusCanceled JobStatus = "canceled"
)

// Cancelable indica si un trabajo en este estado admite cancelación.
// Solo tiene sentido cancelar lo que aún no terminó.
func (s JobStatus) Cancelable() bool {
	return s == JobStatusPending || s == JobStatusProcessing
}

// User es una cuenta de la plataforma. PasswordHash nunca se serializa a JSON.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Job es un trabajo de conversión de un documento a bundle OKF.
type Job struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	OriginalName string    `json:"original_name"`
	Format       DocFormat `json:"format"`
	ObjectKey    string    `json:"object_key"`
	Status       JobStatus `json:"status"`
	ErrorMessage string    `json:"error_message,omitempty"`
	// RetryOf enlaza este trabajo con el trabajo fallido que reintenta.
	RetryOf   *string   `json:"retry_of,omitempty"`
	Attempt   int       `json:"attempt"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ValidationLevel clasifica el resultado de un bundle publicado, de forma
// separada a la validez de plataforma.
type ValidationLevel string

const (
	ValidationValid         ValidationLevel = "valid"
	ValidationWithWarnings  ValidationLevel = "valid_with_warnings"
)

// Bundle es el resultado publicado de un trabajo completado.
type Bundle struct {
	ID         string          `json:"id"`
	JobID      string          `json:"job_id"`
	Path       string          `json:"path"`
	Validation ValidationLevel `json:"validation"`
	Warnings   []string        `json:"warnings"`
	CreatedAt  time.Time       `json:"created_at"`
}

// JobMessage es el contrato que viaja por la cola entre la API y los workers.
// Contiene todo lo que un worker necesita para procesar el trabajo, de modo
// que la API no conserve estado en memoria.
type JobMessage struct {
	JobID     string    `json:"job_id"`
	UserID    string    `json:"user_id"`
	Format    DocFormat `json:"format"`
	ObjectKey string    `json:"object_key"`
	Attempt   int       `json:"attempt"`
}
