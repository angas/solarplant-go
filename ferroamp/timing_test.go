package ferroamp

import (
	"bytes"
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

func TestPendingRequestExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer
		expired := make(chan struct{})
		fa := &Ferroamp{
			logger:  slog.New(slog.NewTextHandler(&logs, nil)),
			pending: map[string]pendingRequest{"old": {SentAt: time.Now(), DoneCh: expired}},
		}
		fa.startPurgeRoutine()
		defer func() { close(fa.stopPurgeCh); synctest.Wait() }()
		synctest.Wait()
		time.Sleep(time.Minute)
		synctest.Wait()
		if len(fa.pending) != 1 {
			t.Fatal("request expired at exactly one minute")
		}
		select {
		case <-expired:
			t.Fatal("waiter released too early")
		default:
		}
		fresh := make(chan struct{})
		fa.pendingMutex.Lock()
		fa.pending["fresh"] = pendingRequest{SentAt: time.Now(), DoneCh: fresh}
		fa.pendingMutex.Unlock()
		time.Sleep(time.Minute)
		synctest.Wait()
		if _, exists := fa.pending["old"]; exists {
			t.Fatal("expired request retained")
		}
		select {
		case <-expired:
		default:
			t.Fatal("expired request waiter not released")
		}
		if _, exists := fa.pending["fresh"]; !exists {
			t.Fatal("fresh request removed")
		}
		select {
		case <-fresh:
			t.Fatal("fresh request waiter released")
		default:
		}
	})
}

// Only Publish is expected in these tests; other MQTT operations fail if called.
type publishClient struct {
	mqtt.Client
	publish func(string, byte, bool, any) mqtt.Token
}

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
func (t publishToken) Error() error { return t.err }

func TestControlRequestTiming(t *testing.T) {
	publishErr := errors.New("publish failed")
	for _, mode := range []string{"ack", "no response", "publish timeout", "publish error"} {
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
					mtqqClient: publishClient{publish: func(topic string, qos byte, retained bool, payload any) mqtt.Token {
						publishes++
						if topic != "extapi/control/request" || qos != 0 || retained || payload != "payload" {
							t.Errorf("unexpected publish: %s %d %v %v", topic, qos, retained, payload)
						}
						return token
					}},
				}
				done := make(chan error, 1)
				start := time.Now()
				go func() { done <- fa.sendControlRequest("solarplant-test", "payload") }()
				synctest.Wait()
				wantDuration := time.Duration(0)
				switch mode {
				case "ack", "no response":
					time.Sleep(29 * time.Second)
					synctest.Wait()
					select {
					case err := <-done:
						t.Fatalf("request completed early: %v", err)
					default:
					}
					if mode == "ack" {
						fa.handleMessage(testMessage{topic: "extapi/control/response", payload: `{"transId":"solarplant-test","status":"ack"}`})
						wantDuration = 29 * time.Second
					} else {
						wantDuration = 30 * time.Second
					}
				case "publish timeout":
					wantDuration = 5 * time.Second
				}
				err := <-done
				if elapsed := time.Since(start); elapsed != wantDuration {
					t.Fatalf("elapsed=%v, want %v", elapsed, wantDuration)
				}
				if publishes != 1 {
					t.Fatalf("publishes=%d, want 1", publishes)
				}
				switch mode {
				case "publish error":
					if !errors.Is(err, publishErr) {
						t.Fatalf("error=%v, want publish error", err)
					}
				case "publish timeout":
					if err == nil || !strings.Contains(err.Error(), "timeout") {
						t.Fatalf("error=%v, want timeout", err)
					}
				default:
					// Missing application acknowledgement currently logs a warning but returns nil.
					if err != nil {
						t.Fatal(err)
					}
				}
				if strings.Contains(logs.String(), "pending request timed out") != (mode == "no response") {
					t.Fatalf("unexpected timeout logging: %s", logs.String())
				}
				if (mode == "publish error" || mode == "publish timeout") && len(fa.pending) != 0 {
					t.Fatal("failed publish created a pending request")
				}
			})
		})
	}
}
