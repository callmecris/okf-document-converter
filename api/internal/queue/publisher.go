// Package queue publica mensajes de trabajo en RabbitMQ.
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"okf/pkg/domain"
)

// Publisher publica mensajes de trabajo en una cola duráble de RabbitMQ.
// El canal se reconecta automáticamente si el broker lo cierra (p. ej. por
// conflictos de declaración de cola entre api y worker).
type Publisher struct {
	conn  *amqp.Connection
	ch    *amqp.Channel
	queue string
	log   *slog.Logger
}

// NewPublisher conecta (con reintentos) y asegura la cola.
func NewPublisher(ctx context.Context, url, queueName string, log *slog.Logger) (*Publisher, error) {
	conn, err := dial(ctx, url)
	if err != nil {
		return nil, err
	}
	p := &Publisher{conn: conn, queue: queueName, log: log}
	if err := p.ensureChannel(); err != nil {
		conn.Close()
		return nil, err
	}
	log.Info("rabbitmq connected (publisher)", "queue", queueName)
	return p, nil
}

// Publish envía un JobMessage con entrega persistente.
func (p *Publisher) Publish(ctx context.Context, msg domain.JobMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal job message: %w", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		if err := p.ensureChannel(); err != nil {
			return err
		}
		err = p.ch.PublishWithContext(ctx, "", p.queue, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		})
		if err == nil {
			p.log.Info("job message published", "job_id", msg.JobID, "format", msg.Format)
			return nil
		}
		// Canal posiblemente muerto: se fuerza reconexión para el siguiente intento.
		p.ch = nil
	}
	return fmt.Errorf("publish message: %w", err)
}

func (p *Publisher) Close() {
	_ = p.ch.Close()
	_ = p.conn.Close()
}

// ensureChannel crea el canal y asegura la topología de colas.
func (p *Publisher) ensureChannel() error {
	if p.ch != nil && !p.ch.IsClosed() {
		return nil
	}
	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	if err := declareTopology(ch, p.queue); err != nil {
		_ = ch.Close()
		return err
	}
	p.ch = ch
	return nil
}

// declareTopology declara la MISMA topología que worker/internal/consumer:
// cola principal con dead-letter exchange + cola de rechazos.
// Como ambos servicios declaran argumentos idénticos, nunca se produce un
// 406 PRECONDITION_FAILED por argumentos distintos.
func declareTopology(ch *amqp.Channel, queueName string) error {
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
		return fmt.Errorf("declare queue %s: %w", queueName, err)
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