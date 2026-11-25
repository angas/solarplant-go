package ferroamp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type timingLog struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (l *timingLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.Write(p)
}

func (l *timingLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.String()
}

func TestInactivityWatchdogTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs timingLog
		fa := &Ferroamp{logger: slog.New(slog.NewTextHandler(&logs, nil))}
		var calls atomic.Int32
		fa.OnInactivity = func() { calls.Add(1) }
		fa.inactivityWatchdog()
		defer func() { close(fa.stopMonitorCh); synctest.Wait() }()
		synctest.Wait()
		check := func(advance time.Duration, warnings, failures, recoveries int) {
			t.Helper()
			time.Sleep(advance)
			synctest.Wait()
			if strings.Count(logs.String(), "level=WARN") != warnings || strings.Count(logs.String(), "level=ERROR") != failures || int(calls.Load()) != failures || strings.Count(logs.String(), "mqtt traffic is restored") != recoveries {
				t.Fatalf("at %v: callbacks=%d, logs:\n%s", time.Now(), calls.Load(), logs.String())
			}
		}
		check(9*time.Second, 0, 0, 0)
		check(time.Second, 1, 0, 0)
		check(49*time.Second, 1, 0, 0)
		check(time.Second, 1, 1, 0)
		check(time.Minute, 1, 1, 0) // A continuing outage must not repeat notifications.
		fa.handleMessage(testMessage{topic: "extapi/data/ehub", payload: `{}`})
		check(time.Second, 1, 1, 1)
		check(9*time.Second, 2, 1, 1)
		check(50*time.Second, 2, 2, 1) // Recovery re-arms both thresholds.
	})
}

// Only IsConnectionOpen and Publish are expected in these tests; other MQTT operations fail if called.
type publishClient struct {
	mqtt.Client
	disconnected bool
	publish      func(string, byte, bool, any) mqtt.Token
}

func (c publishClient) IsConnectionOpen() bool { return !c.disconnected }

func (c publishClient) Publish(topic string, qos byte, retained bool, payload any) mqtt.Token {
	return c.publish(topic, qos, retained, payload)
}

type publishToken struct {
	mqtt.Token
	timeout bool
	err     error
}

func (t publishToken) WaitTimeout(timeout time.Duration) bool {
	if t.timeout {
		time.Sleep(timeout)
		return false
	}
	return true
}
func (t publishToken) Wait() bool   { return true }
func (t publishToken) Error() error { return t.err }

func TestControlRequestTiming(t *testing.T) {
	publishErr := errors.New("publish failed")
	for _, mode := range []string{"ack", "nak", "no response", "publish timeout", "publish error", "not connected", "cancelled before send", "cancelled while waiting"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var logs bytes.Buffer
				token := publishToken{timeout: mode == "publish timeout"}
				if mode == "publish error" {
					token.err = publishErr
				}
				publishes := 0
				fa := &Ferroamp{
					logger:  slog.New(slog.NewTextHandler(&logs, nil)),
					pending: make(map[string]pendingRequest),
					mtqqClient: publishClient{disconnected: mode == "not connected", publish: func(topic string, qos byte, retained bool, payload any) mqtt.Token {
						publishes++
						if topic != "extapi/control/request" || qos != 0 || retained || payload != "payload" {
							t.Errorf("unexpected publish: %s %d %v %v", topic, qos, retained, payload)
						}
						return token
					}},
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if mode == "cancelled before send" {
					cancel()
				}
				done := make(chan error, 1)
				start := time.Now()
				go func() { done <- fa.sendControlRequest(ctx, "solarplant-test", "payload") }()
				synctest.Wait()
				wantDuration := time.Duration(0)
				switch mode {
				case "ack", "nak", "no response", "cancelled while waiting":
					time.Sleep(29 * time.Second)
					synctest.Wait()
					select {
					case err := <-done:
						t.Fatalf("request completed early: %v", err)
					default:
					}
					switch mode {
					case "ack", "nak":
						fa.handleMessage(testMessage{topic: "extapi/control/response", payload: `{"transId":"solarplant-test","status":"` + mode + `","msg":"reason"}`})
						wantDuration = 29 * time.Second
					case "cancelled while waiting":
						cancel()
						wantDuration = 29 * time.Second
					default:
						wantDuration = 30 * time.Second
					}
				case "publish timeout":
					wantDuration = 5 * time.Second
				}
				err := <-done
				if elapsed := time.Since(start); elapsed != wantDuration {
					t.Fatalf("elapsed=%v, want %v", elapsed, wantDuration)
				}
				wantPublishes := 1
				if mode == "not connected" || mode == "cancelled before send" {
					wantPublishes = 0
				}
				if publishes != wantPublishes {
					t.Fatalf("publishes=%d, want %d", publishes, wantPublishes)
				}
				switch mode {
				case "not connected":
					if !errors.Is(err, errNotConnected) {
						t.Fatalf("error=%v, want not connected", err)
					}
				case "publish error":
					if !errors.Is(err, publishErr) {
						t.Fatalf("error=%v, want publish error", err)
					}
				case "publish timeout":
					if !errors.Is(err, errPublishTimeout) {
						t.Fatalf("error=%v, want publish timeout", err)
					}
				case "cancelled before send", "cancelled while waiting":
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("error=%v, want cancelled", err)
					}
				case "nak":
					if !errors.Is(err, errRejected) || !strings.Contains(err.Error(), "reason") {
						t.Fatalf("error=%v, want rejected with reason", err)
					}
				case "no response":
					if !errors.Is(err, errNoResponse) {
						t.Fatalf("error=%v, want no response", err)
					}
				default:
					if err != nil {
						t.Fatal(err)
					}
				}
				if len(fa.pending) != 0 {
					t.Fatalf("pending requests retained: %v", fa.pending)
				}

				// A late or duplicate response must neither block the message handler nor be mistaken for a new one.
				fa.handleMessage(testMessage{topic: "extapi/control/response", payload: `{"transId":"solarplant-test","status":"ack"}`})
				if !strings.Contains(logs.String(), "received response for unknown transaction") {
					t.Fatalf("late response not logged: %s", logs.String())
				}
			})
		})
	}
}
