package broker

import (
	"time"

	"github.com/nats-io/nats.go"
)

type NATS struct {
	conn *nats.Conn
}

type MessageHandler = func(subject string, payload []byte)

func Connect(url string) (*NATS, error) {
	conn, err := nats.Connect(url, nats.Timeout(5*time.Second))
	if err != nil {
		return nil, err
	}

	return &NATS{conn: conn}, nil
}

func (n *NATS) Publish(subject string, payload []byte) error {
	if err := n.conn.Publish(subject, payload); err != nil {
		return err
	}

	return n.conn.FlushTimeout(2 * time.Second)
}

func (n *NATS) Subscribe(subject string, handler MessageHandler) error {
	_, err := n.conn.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Subject, msg.Data)
	})
	if err != nil {
		return err
	}

	return n.conn.FlushTimeout(2 * time.Second)
}

func (n *NATS) Drain() error {
	return n.conn.Drain()
}

func (n *NATS) Close() {
	n.conn.Close()
}
