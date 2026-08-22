package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"okf/api/internal/middleware"
	"okf/api/internal/repository"
)

// Metrics expone la actividad de conversión. El endpoint JSON (/api/v1/metrics)
// va tras JWTAuth y devuelve SOLO la actividad del usuario autenticado; el
// endpoint Prometheus (/metrics) es operativo y agrega toda la plataforma.
type Metrics struct {
	repo *repository.Postgres
	log  *slog.Logger
}

func NewMetrics(repo *repository.Postgres, log *slog.Logger) *Metrics {
	return &Metrics{repo: repo, log: log}
}

// JSON devuelve las métricas del usuario autenticado en formato JSON.
func (m *Metrics) JSON(w http.ResponseWriter, r *http.Request) {
	metrics, err := m.repo.GetMetrics(r.Context(), middleware.UserID(r))
	if err != nil {
		internalError(w, m.log, err)
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// Prometheus expone las métricas globales del sistema en formato de
// exposición Prometheus, para que puedan recolectarse con herramientas
// estándar. No se expone a través del frontend.
func (m *Metrics) Prometheus(w http.ResponseWriter, r *http.Request) {
	metrics, err := m.repo.GetMetrics(r.Context(), "")
	if err != nil {
		internalError(w, m.log, err)
		return
	}

	var b strings.Builder
	b.WriteString("# HELP okf_jobs_total Trabajos de conversión por estado.\n")
	b.WriteString("# TYPE okf_jobs_total gauge\n")
	for _, k := range sortedKeys(metrics.JobsByStatus) {
		fmt.Fprintf(&b, "okf_jobs_total{status=%q} %d\n", k, metrics.JobsByStatus[k])
	}

	b.WriteString("# HELP okf_jobs_by_format_total Trabajos por formato de entrada.\n")
	b.WriteString("# TYPE okf_jobs_by_format_total gauge\n")
	for _, k := range sortedKeys(metrics.JobsByFormat) {
		fmt.Fprintf(&b, "okf_jobs_by_format_total{format=%q} %d\n", k, metrics.JobsByFormat[k])
	}

	b.WriteString("# HELP okf_bundles_total Bundles publicados por clasificación de validación.\n")
	b.WriteString("# TYPE okf_bundles_total gauge\n")
	for _, k := range sortedKeys(metrics.BundlesByValidation) {
		fmt.Fprintf(&b, "okf_bundles_total{validation=%q} %d\n", k, metrics.BundlesByValidation[k])
	}

	b.WriteString("# HELP okf_job_duration_seconds Duración media de las conversiones terminadas.\n")
	b.WriteString("# TYPE okf_job_duration_seconds gauge\n")
	fmt.Fprintf(&b, "okf_job_duration_seconds %.3f\n", metrics.AvgDurationSeconds)

	b.WriteString("# HELP okf_retries_total Trabajos creados como reintento de otro.\n")
	b.WriteString("# TYPE okf_retries_total counter\n")
	fmt.Fprintf(&b, "okf_retries_total %d\n", metrics.Retries)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// sortedKeys devuelve las claves ordenadas para una salida estable.
func sortedKeys[K ~string, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
