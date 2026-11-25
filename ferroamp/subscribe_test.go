package ferroamp

import (
	"bytes"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// subscribeClient connects like the real client: a successful Connect runs the
// OnConnect handler in its own goroutine.
type subscribeClient struct {
	mqtt.Client
	onConnect     func(mqtt.Client)
	noCallback    bool // Connect succeeds without running the OnConnect handler
	connectErr    error
	subscribe     publishToken
	subscriptions []map[string]byte
	handler       mqtt.MessageHandler
}

func (c *subscribeClient) Connect() mqtt.Token {
	if c.connectErr == nil && !c.noCallback {
		go c.onConnect(c)
	}
	return publishToken{err: c.connectErr}
}

func (c *subscribeClient) SubscribeMultiple(filters map[string]byte, handler mqtt.MessageHandler) mqtt.Token {
	c.subscriptions = append(c.subscriptions, maps.Clone(filters))
	c.handler = handler
	return c.subscribe
}

func TestSubscribeOnEveryConnect(t *testing.T) {
	subscribeErr := errors.New("subscribe failed")
	connectErr := errors.New("connect failed")
	for _, mode := range []string{"ok", "connect error", "no connect callback", "subscribe error", "subscribe timeout"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var logs bytes.Buffer
				client := &subscribeClient{}
				switch mode {
				case "connect error":
					client.connectErr = connectErr
				case "no connect callback":
					client.noCallback = true
				case "subscribe error":
					client.subscribe.err = subscribeErr
				case "subscribe timeout":
					client.subscribe.timeout = true
				}
				start := time.Now()
				fa := &Ferroamp{
					logger:     slog.New(slog.NewTextHandler(&logs, nil)),
					mtqqClient: client,
					subscribed: make(chan error, 1),
				}
				client.onConnect = fa.onConnect
				var connected atomic.Int32
				fa.OnConnected = func() { connected.Add(1) }

				err := fa.Connect()
				if fa.stopMonitorCh != nil {
					defer func() { close(fa.stopMonitorCh); synctest.Wait() }()
				}

				switch mode {
				case "connect error":
					if !errors.Is(err, connectErr) {
						t.Fatalf("error=%v, want connect error", err)
					}
					if len(client.subscriptions) != 0 {
						t.Fatal("subscribed without a connection")
					}
					return
				case "no connect callback":
					// Connect must not wait forever for a subscription that is never made.
					if !errors.Is(err, errSubscribeTimeout) || time.Since(start) != firstSubscribeTimeout {
						t.Fatalf("error=%v after %v, want subscribe timeout after %v", err, time.Since(start), firstSubscribeTimeout)
					}
					if fa.stopMonitorCh != nil {
						t.Fatal("watchdog running after connect error")
					}
					return
				case "subscribe error":
					if !errors.Is(err, subscribeErr) {
						t.Fatalf("error=%v, want subscribe error", err)
					}
				case "subscribe timeout":
					if !errors.Is(err, errSubscribeTimeout) || time.Since(start) != subscribeTimeout {
						t.Fatalf("error=%v after %v, want subscribe timeout after %v", err, time.Since(start), subscribeTimeout)
					}
				default:
					if err != nil {
						t.Fatal(err)
					}
				}
				if (err != nil) != strings.Contains(logs.String(), "failed to subscribe to ferroamp topics") {
					t.Fatalf("unexpected subscribe logging for error %v: %s", err, logs.String())
				}
				if (err == nil) != (fa.stopMonitorCh != nil) {
					t.Fatalf("watchdog running=%v after connect error %v", fa.stopMonitorCh != nil, err)
				}

				// An automatic reconnect runs the handler again, and must subscribe again.
				fa.onConnect(client)
				if len(client.subscriptions) != 2 {
					t.Fatalf("subscriptions=%d, want 2", len(client.subscriptions))
				}
				// Every connect is reported, since instructions may have been lost meanwhile.
				synctest.Wait()
				if got := connected.Load(); got != 2 {
					t.Fatalf("connected callbacks=%d, want 2", got)
				}
				for _, got := range client.subscriptions {
					if !maps.Equal(got, topics) {
						t.Fatalf("subscribed to %v, want %v", got, topics)
					}
				}

				// Subscribed messages must reach the message handling.
				calls := 0
				fa.OnEhubMessage = func(*EhubMessage) { calls++ }
				client.handler(client, testMessage{topic: "extapi/data/ehub", payload: `{}`})
				if calls != 1 {
					t.Fatalf("message callbacks=%d, want 1", calls)
				}
			})
		})
	}
}

func TestRejectedTopics(t *testing.T) {
	tests := []struct {
		name   string
		result map[string]byte
		want   []string
	}{
		{"no result", nil, nil},
		{"all granted", map[string]byte{"a": 0, "b": 1, "c": 2}, nil},
		{"some rejected", map[string]byte{"b": subscriptionRejected, "c": 0, "a": subscriptionRejected}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rejectedTopics(tt.result); !slices.Equal(got, tt.want) {
				t.Fatalf("rejected=%v, want %v", got, tt.want)
			}
		})
	}
}
