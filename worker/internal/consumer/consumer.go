// Package consumer implementa el suscriptor de RabbitMQ del worker.
// Mensajes fallidos se envían a una cola dead-letter (sin requeue) para
// inspección posterior; la idempotencia real la garantiza el claim atómico en DB.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"okf/pkg/domain"
)

// HandlerFunc procesa un mensaje de trabajo. Un error provoca NACK sin requeue.
type HandlerFunc func(ctx context.Context, msg domain.JobMessage) error

type Consumer struct {
	conn    *amqp.Connection
	ch      *amqp.Channel
	queue   string
	handler HandlerFunc
	log     *slog.Logger
	// prefetch es también el número máximo de trabajos que este worker
	// procesa en paralelo: coincide con el QoS para no acumular mensajes
	// reservados que no se estén atendiendo.
	prefetch int
}

// NewConsumer conecta (con reintentos), configura prefetch y declara colas.
func NewConsumer(ctx context.Context, url, queueName string, prefetch int, handler HandlerFunc, log *slog.Logger) (*Consumer, error) {
	conn, err := dial(ctx, url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set prefetch: %w", err)
	}
	if err := declareTopology(ch, queueName, log); err != nil {
		conn.Close()
		return nil, err
	}
	log.Info("rabbitmq connected (consumer)", "queue", queueName, "prefetch", prefetch)
	return &Consumer{conn: conn, ch: ch, queue: queueName, handler: handler, log: log, prefetch: prefetch}, nil
}

// Start consume mensajes hasta que el contexto se cancele.
func (c *Consumer) Start(ctx context.Context) error {
	tag := fmt.Sprintf("worker-%s", uuid.NewString()[:8])
	deliveries, err := c.ch.Consume(c.queue, tag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("start consuming: %w", err)
	}
	c.log.Info("consumer started", "tag", tag, "concurrencia", c.prefetch)

	// Los trabajos se atienden en paralelo hasta el límite de prefetch: un
	// documento largo no bloquea a los demás. El semáforo acota la
	// concurrencia y wg permite drenar los trabajos en curso al apagar.
	sem := make(chan struct{}, c.prefetch)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait() // no cortar trabajos a medias
			return nil
		case d, ok := <-deliveries:
			if !ok {
				wg.Wait()
				return errors.New("delivery channel closed")
			}

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				// Apagando: se devuelve el mensaje a la cola sin procesar.
				_ = d.Nack(false, true)
				wg.Wait()
				return nil
			}

			wg.Add(1)
			go func(d amqp.Delivery) {
				defer wg.Done()
				defer func() { <-sem }()
				c.process(ctx, d)
			}(d)
		}
	}
}

func (c *Consumer) process(ctx context.Context, d amqp.Delivery) {
	defer func() {
		if r := recover(); r != nil {
			// Un panic no debe matar al worker ni dejar el job en "processing":
			// el handler en main.go ya marcó el job como fallido en DB (tiene
			// acceso al repositorio); aquí solo se NACK (-> DLQ) y se sigue.
			c.log.Error("consumer: panic en delivery", "error", r, "stack", string(debug.Stack()))
			_ = d.Nack(false, false)
		}
	}()

	var msg domain.JobMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		c.log.Error("consumer: unmarshal message", "error", err)
		_ = d.Nack(false, false) // mensaje corrupto -> dead letter
		return
	}

	procCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	if err := c.handler(procCtx, msg); err != nil {
		c.log.Error("consumer: job failed", "job_id", msg.JobID, "error", err)
		_ = d.Nack(false, false) // sin requeue -> cola dead-letter
		return
	}
	_ = d.Ack(false)
}

func (c *Consumer) Close() {
	_ = c.ch.Close()
	_ = c.conn.Close()
}

// declareTopology declara la cola principal con dead-letter + cola de rechazos.
// Nota: mantener sincronizado con api/internal/queue.
func declareTopology(ch *amqp.Channel, queueName string, log *slog.Logger) error {
	dlx := queueName + ".dlx"
	dlq := queueName + ".dlq"

	if err := ch.ExchangeDeclare(dlx, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter exchange: %w", err)
	}
	if _, err := ch.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter queue: %w", err)
	}
	if err := ch.QueueBind(dlq, "", dlx, false, nil); err != nil {
		return fmt.Errorf("bind dead-letter queue: %w", err)
	}

	if _, err := ch.QueueDeclare(queueName, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": dlx,
	}); err != nil {
		// La cola puede ya existir declarada sin dead-letter (creada por la API).
		log.Warn("queue declare (with dlx) failed; using existing queue", "queue", queueName)
	}
	return nil
}

func dial(ctx context.Context, url string) (*amqp.Connection, error) {
	var lastErr error
	for i := 1; i <= 30; i++ {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("connect to rabbitmq: %w", lastErr)
}