package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/google/uuid"

	"okf/pkg/domain"
	"okf/pkg/logger"
	"okf/worker/internal/config"
	"okf/worker/internal/consumer"
	"okf/worker/internal/converter"
	"okf/worker/internal/okf"
	"okf/worker/internal/repository"
	"okf/worker/internal/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker stopped with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}

	db, err := repository.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return err
	}

	store, err := storage.New(
		cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey,
		cfg.MinIOUseSSL, cfg.BucketOriginals, cfg.BucketBundles,
	)
	if err != nil {
		return err
	}
	if err := store.EnsureBuckets(ctx); err != nil {
		return err
	}

	pipeline := converter.NewPipeline(cfg.PDFChunkPages)

	handler := func(ctx context.Context, msg domain.JobMessage) (err error) {
		// Un panic dentro del pipeline no debe dejar el trabajo "processing"
		// para siempre: se captura, se marca como fallido y se propaga como
		// error para que el consumer envíe el mensaje al DLQ.
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic procesando job: %v", r)
				log.Error("panic en processJob", "job_id", msg.JobID, "panic", r, "stack", string(debug.Stack()))
				if dbErr := db.MarkFailed(context.Background(), msg.JobID, err.Error()); dbErr != nil {
					log.Error("no se pudo marcar job como fallido tras panic", "job_id", msg.JobID, "error", dbErr)
				}
			}
		}()
		return processJob(ctx, cfg, db, store, pipeline, msg, log)
	}

	c, err := consumer.NewConsumer(ctx, cfg.RabbitMQURL, cfg.RabbitMQQueue, 3, handler, log)
	if err != nil {
		return err
	}
	defer c.Close()

	return c.Start(ctx)
}

// processJob ejecuta el pipeline completo de un trabajo:
// claim atómico → descarga → conversión → bundle OKF → validación → subida.
func processJob(
	ctx context.Context,
	cfg *config.Config,
	db *repository.Postgres,
	store *storage.Storage,
	pipeline *converter.Pipeline,
	msg domain.JobMessage,
	log *slog.Logger,
) error {
	// Idempotencia: solo un worker puede claim; los duplicados se ACK y descartan.
	claimed, err := db.ClaimJob(ctx, msg.JobID)
	if err != nil {
		return err
	}
	if !claimed {
		log.Warn("job ya procesado o en progreso; mensaje descartado", "job_id", msg.JobID)
		return nil
	}

	workDir, err := os.MkdirTemp(cfg.WorkDir, "okf-job-*")
	if err != nil {
		return failJob(db, msg.JobID, err)
	}
	defer os.RemoveAll(workDir)

	srcPath := filepath.Join(workDir, "original."+string(msg.Format))
	if err := store.Download(ctx, store.OriginalsBucket(), msg.ObjectKey, srcPath); err != nil {
		return failJob(db, msg.JobID, err)
	}

	job, err := db.GetJobName(ctx, msg.JobID)
	if err != nil {
		return failJob(db, msg.JobID, err)
	}

	segments, err := pipeline.Convert(ctx, converter.Options{
		Format:       msg.Format,
		SourcePath:   srcPath,
		WorkDir:      workDir,
		PDFChunkSize: cfg.PDFChunkPages,
	})
	if err != nil {
		return failJob(db, msg.JobID, err)
	}

	result, err := okf.Build(ctx, workDir, segments, okf.Meta{
		JobID:        msg.JobID,
		UserID:       msg.UserID,
		OriginalName: job.OriginalName,
		Format:       string(msg.Format),
		SourcePath:   srcPath,
		ConvertedAt:  time.Now(),
	})
	if err != nil {
		return failJob(db, msg.JobID, err)
	}

	if err := okf.Validate(result.Dir); err != nil {
		return failJob(db, msg.JobID, err)
	}

	if err := store.UploadDir(ctx, store.BundlesBucket(), result.Dir, result.BundlePath); err != nil {
		return failJob(db, msg.JobID, err)
	}

	if err := db.MarkCompleted(ctx, msg.JobID); err != nil {
		return err
	}
	if err := db.CreateBundle(ctx, uuid.NewString(), msg.JobID, result.BundlePath); err != nil {
		log.Error("job completado pero falló el registro del bundle", "job_id", msg.JobID, "error", err)
	}

	log.Info("job completado",
		"job_id", msg.JobID,
		"format", msg.Format,
		"conceptos", len(result.Segments),
		"bundle", result.BundlePath,
	)
	return nil
}

// failJob registra el error y devuelve un error para que el consumer
// envíe el mensaje a la cola dead-letter (sin reprocesar).
func failJob(db *repository.Postgres, jobID string, err error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if dbErr := db.MarkFailed(ctx, jobID, err.Error()); dbErr != nil {
		return fmt.Errorf("mark failed: %w (causa: %v)", dbErr, err)
	}
	return err
}