package ferroamp

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

type testMessage struct {
	topic   string
	payload string
}

func (m testMessage) Topic() string     { return m.topic }
func (m testMessage) Payload() []byte   { return []byte(m.payload) }
func (m testMessage) Duplicate() bool   { return false }
func (m testMessage) Qos() byte         { return 0 }
func (m testMessage) Retained() bool    { return false }
func (m testMessage) MessageID() uint16 { return 0 }
func (m testMessage) Ack()              {}

func TestHandleMessage(t *testing.T) {
	tests := []struct {
		topic   string
		payload string
		invalid string // Valid JSON that may partially populate a message before failing.
		want    any
	}{
		{"extapi/data/ehub", `{"soc":{"val":"42"}}`, `{"soc":{"val":"bad"}}`, &EhubMessage{Soc: FltObj{Value: 42}}},
		{"extapi/data/sso", `{"faultcode":{"val":"2"}}`, `{"faultcode":{"val":"2"},"id":{"val":1}}`, &SsoMessage{FaultCode: IntObj{Value: 2}}},
		{"extapi/data/eso", `{"faultcode":{"val":"2"}}`, `{"faultcode":{"val":"2"},"id":{"val":1}}`, &EsoMessage{FaultCode: IntObj{Value: 2}}},
		{"extapi/data/esm", `{"soc":{"val":"42"}}`, `{"soc":{"val":"bad"}}`, &EsmMessage{Soc: FltObj{Value: 42}}},
		{"extapi/control/response", `{"transId":"solarplant-test","status":"ack"}`, `{"transId":"solarplant-test","status":1}`, &ControlResponseMessage{TransId: "solarplant-test", Status: "ack"}},
		{"extapi/control/event", `{"event":"test"}`, `{"event":1}`, &ControlEventMessage{Event: "test"}},
	}
	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			for _, mode := range []string{"valid", "syntax error", "type error", "no callback"} {
				t.Run(mode, func(t *testing.T) {
					var logs bytes.Buffer
					done := make(chan struct{}, 1)
					fa := &Ferroamp{
						logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
						pending: map[string]pendingRequest{
							"solarplant-test": {SentAt: time.Now(), DoneCh: done},
						},
						lastSsoFaultCode: 1,
						lastEsoFaultCode: 1,
					}
					var got any
					calls := 0
					record := func(msg any) { got = msg; calls++ }
					if mode != "no callback" {
						fa.OnEhubMessage = func(msg *EhubMessage) { record(msg) }
						fa.OnSsoMessage = func(msg *SsoMessage) { record(msg) }
						fa.OnEsoMessage = func(msg *EsoMessage) { record(msg) }
						fa.OnEsmMessage = func(msg *EsmMessage) { record(msg) }
						fa.OnControlResponse = func(msg *ControlResponseMessage) { record(msg) }
						fa.OnControlEvent = func(msg *ControlEventMessage) { record(msg) }
					}
					payload := tt.payload
					invalid := mode == "syntax error" || mode == "type error"
					if mode == "syntax error" {
						payload = `{"broken":`
					} else if mode == "type error" {
						payload = tt.invalid
					}
					fa.handleMessage(testMessage{topic: tt.topic, payload: payload})
					if mode == "valid" {
						if calls != 1 || !reflect.DeepEqual(got, tt.want) {
							t.Fatalf("callback: calls=%d, got=%#v, want=%#v", calls, got, tt.want)
						}
					} else if calls != 0 {
						t.Fatalf("unexpected callback: %#v", got)
					}
					wantSso, wantEso := uint16(1), uint16(1)
					if !invalid {
						if tt.topic == "extapi/data/sso" {
							wantSso = 2
						}
						if tt.topic == "extapi/data/eso" {
							wantEso = 2
						}
					}
					if fa.lastSsoFaultCode != wantSso || fa.lastEsoFaultCode != wantEso {
						t.Errorf("fault state: SSO=%d, ESO=%d; want SSO=%d, ESO=%d", fa.lastSsoFaultCode, fa.lastEsoFaultCode, wantSso, wantEso)
					}
					wantAcks := 0
					if tt.topic == "extapi/control/response" && !invalid {
						wantAcks = 1
					}
					if len(done) != wantAcks {
						t.Errorf("transaction acknowledgements=%d, want %d", len(done), wantAcks)
					}
					if invalid {
						if !strings.Contains(logs.String(), "level=ERROR") || !strings.Contains(logs.String(), "topic="+tt.topic) {
							t.Errorf("missing decode error with topic: %s", logs.String())
						}
						if strings.Contains(logs.String(), "fault code") || strings.Contains(logs.String(), "received control") {
							t.Errorf("processed invalid message: %s", logs.String())
						}
					}
				})
			}
		})
	}
}
