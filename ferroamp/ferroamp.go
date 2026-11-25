package ferroamp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type OnEhubMessage func(msg *EhubMessage)
type OnSsoMessage func(msg *SsoMessage)
type OnEsoMessage func(msg *EsoMessage)
type OnEsmMessage func(msg *EsmMessage)
type OnControlResponse func(msg *ControlResponseMessage)
type OnControlEvent func(msg *ControlEventMessage)
type OnInactivity func()

// OnConnected is called on the initial connect and on every automatic
// reconnect. Battery instructions sent before a reconnect may have been lost.
type OnConnected func()

type pendingRequest struct {
	TransId    string
	Payload    string
	SentAt     time.Time
	ResponseCh chan *ControlResponseMessage // Buffered, receives the response at most once
}

const (
	publishTimeout         = 5 * time.Second
	controlResponseTimeout = 30 * time.Second
	subscribeTimeout       = 10 * time.Second
	// Time for the connect handler to report the first subscription; longer
	// than subscribeTimeout so that a slow subscription reports its own error.
	firstSubscribeTimeout = 2 * subscribeTimeout

	// SUBACK return code for a subscription that the broker refused
	subscriptionRejected = 0x80

	// Status of a control response that accepted the request
	statusAck = "ack"
)

var (
	errNotConnected         = errors.New("cannot send battery control request to ferroamp: mqtt not connected")
	errPublishTimeout       = errors.New("timeout when sending battery control request to ferroamp")
	errNoResponse           = errors.New("no response from ferroamp on battery control request")
	errRejected             = errors.New("ferroamp rejected battery control request")
	errSubscribeTimeout     = errors.New("timeout when subscribing to ferroamp topics")
	errSubscriptionRejected = errors.New("ferroamp mqtt broker rejected subscription")
)

var topics = map[string]byte{
	"extapi/data/ehub":        0,
	"extapi/data/sso":         0,
	"extapi/data/eso":         0,
	"extapi/data/esm":         0,
	"extapi/control/response": 0,
	"extapi/control/event":    0,
}

type Ferroamp struct {
	mtqqClient        mqtt.Client
	logger            *slog.Logger
	pending           map[string]pendingRequest
	pendingMutex      sync.Mutex
	subscribed        chan error // Receives the result of the first subscription
	lastEsoFaultCode  uint16
	lastSsoFaultCode  uint16
	lastMessageTime   ConcurrentTimer
	stopMonitorCh     chan struct{}
	OnEhubMessage     OnEhubMessage
	OnSsoMessage      OnSsoMessage
	OnEsoMessage      OnEsoMessage
	OnEsmMessage      OnEsmMessage
	OnControlResponse OnControlResponse
	OnControlEvent    OnControlEvent
	OnInactivity      OnInactivity
	OnConnected       OnConnected
}

func New(broker string, port int16, username string, password string) *Ferroamp {
	fa := &Ferroamp{
		logger:     slog.Default().With("module", "ferroamp"),
		pending:    make(map[string]pendingRequest),
		subscribed: make(chan error, 1),
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	opts.SetClientID("solarplant")
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetAutoReconnect(true)
	opts.OnConnect = fa.onConnect
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		fa.logger.Warn("ferroamp mqtt connection lost", slog.Any("error", err))
	}
	// Paho logs several debug records per incoming message, which would flood
	// the log, so its logger is limited to info and above.
	mqttHandler := newMinLevelHandler(slog.Default().Handler(), slog.LevelInfo)
	opts.SetLogger(slog.New(mqttHandler).With("module", "mqtt"))

	fa.mtqqClient = mqtt.NewClient(opts)

	return fa
}

func (fa *Ferroamp) Connect() error {
	fa.logger.Debug("connecting ferroamp MQTT client")

	if token := fa.mtqqClient.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	// The subscription is made in onConnect, which the client runs once connected.
	timeout := time.NewTimer(firstSubscribeTimeout)
	defer timeout.Stop()
	select {
	case err := <-fa.subscribed:
		if err != nil {
			return err
		}
	case <-timeout.C:
		return fmt.Errorf("%w: connect handler did not report a subscription within %s", errSubscribeTimeout, firstSubscribeTimeout)
	}

	fa.inactivityWatchdog()

	return nil
}

// onConnect runs on the initial connect and on every automatic reconnect. The
// broker forgets the subscriptions when the connection is lost (clean session),
// so they must be made again each time.
func (fa *Ferroamp) onConnect(client mqtt.Client) {
	fa.logger.Info("ferroamp mqtt connected")

	err := fa.subscribe(client)
	if err != nil {
		// Without subscriptions no data arrives, so the inactivity watchdog will take over.
		fa.logger.Error("failed to subscribe to ferroamp topics", slog.Any("error", err))
	}

	// Only Connect waits for the first result, later results are logged above.
	select {
	case fa.subscribed <- err:
	default:
	}

	if fa.OnConnected != nil {
		fa.OnConnected()
	}
}

func (fa *Ferroamp) subscribe(client mqtt.Client) error {
	token := client.SubscribeMultiple(topics, func(_ mqtt.Client, msg mqtt.Message) {
		fa.handleMessage(msg)
	})
	if !token.WaitTimeout(subscribeTimeout) {
		return errSubscribeTimeout
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("error when subscribing to ferroamp topics: %w", err)
	}
	// A refused topic is not a token error, it is only visible in the result.
	if st, ok := token.(*mqtt.SubscribeToken); ok {
		if rejected := rejectedTopics(st.Result()); len(rejected) > 0 {
			return fmt.Errorf("%w: %s", errSubscriptionRejected, strings.Join(rejected, ", "))
		}
	}
	fa.logger.Debug("subscribed to ferroamp topics")
	return nil
}

// rejectedTopics returns the topics that the broker refused, in sorted order.
func rejectedTopics(result map[string]byte) []string {
	var rejected []string
	for topic, code := range result {
		if code == subscriptionRejected {
			rejected = append(rejected, topic)
		}
	}
	slices.Sort(rejected)
	return rejected
}

func (fa *Ferroamp) decodeAndHandle[T any](msg mqtt.Message, handle func(*T)) {
	var value T
	if err := json.Unmarshal(msg.Payload(), &value); err != nil {
		fa.logger.Error("error when reading MQTT message", slog.String("topic", msg.Topic()), slog.Any("error", err))
		return
	}
	if handle != nil {
		handle(&value)
	}
}

func (fa *Ferroamp) handleMessage(msg mqtt.Message) {
	fa.lastMessageTime.Reset()

	switch msg.Topic() {
	case "extapi/data/ehub":
		fa.decodeAndHandle(msg, fa.OnEhubMessage)

	case "extapi/data/sso":
		fa.decodeAndHandle(msg, func(sso *SsoMessage) {
			if fa.OnSsoMessage != nil {
				fa.OnSsoMessage(sso)
			}

			faultCode := uint16(sso.FaultCode.Value)
			if faultCode > 0 && faultCode != fa.lastSsoFaultCode {
				fa.logger.Warn("fault code from SSO, please contact ferroamp support",
					slog.Any("faultCode", faultCode),
					slog.Any("lastFaultCode", fa.lastSsoFaultCode))
			}
			fa.lastSsoFaultCode = faultCode
		})

	case "extapi/data/eso":
		fa.decodeAndHandle(msg, func(eso *EsoMessage) {
			if fa.OnEsoMessage != nil {
				fa.OnEsoMessage(eso)
			}
			fa.handleEsoFaultCode(uint16(eso.FaultCode.Value))
		})

	case "extapi/data/esm":
		fa.decodeAndHandle(msg, fa.OnEsmMessage)

	case "extapi/control/response":
		fa.decodeAndHandle(msg, func(crm *ControlResponseMessage) {
			if e, exists := fa.takePending(crm.TransId); exists {
				duration := time.Since(e.SentAt)
				fa.logger.Debug("received response for known transaction", slog.String("transId", crm.TransId), slog.Duration("duration", duration))
				e.ResponseCh <- crm
			} else if strings.HasPrefix(crm.TransId, "solarplant-") {
				// The request has already been answered or has timed out
				fa.logger.Warn("received response for unknown transaction", slog.String("transId", crm.TransId))
			} else {
				fa.logger.Info("received response for another client", slog.String("transId", crm.TransId), slog.Any("message", crm.Message))
			}

			if fa.OnControlResponse != nil {
				fa.OnControlResponse(crm)
			}
		})

	case "extapi/control/event":
		fa.decodeAndHandle(msg, func(cem *ControlEventMessage) {
			fa.logger.Info("received control event", "event", *cem)
			if fa.OnControlEvent != nil {
				fa.OnControlEvent(cem)
			}
		})

	default:
		fa.logger.Warn("unknown topic", "topic", msg.Topic())
	}
}

func (fa *Ferroamp) Disconnect() {
	fa.logger.Info("disconnecting ferroamp mqtt client")
	if fa.stopMonitorCh != nil {
		close(fa.stopMonitorCh) // Signal the monitor routine to stop
		fa.stopMonitorCh = nil
	}

	keys := make([]string, 0, len(topics))
	for k := range topics {
		keys = append(keys, k)
	}
	token := fa.mtqqClient.Unsubscribe(keys...)
	token.WaitTimeout(1 * time.Second)
	if token.Error() != nil {
		fa.logger.Error("error unsubscribing from topics", slog.Any("error", token.Error()))
	}

	fa.mtqqClient.Disconnect(250)
}

func (fa *Ferroamp) formatPayload(power float64) (transId string, payload string) {
	watts := int(math.Abs(power * 1e3))
	transId = fmt.Sprintf("solarplant-%d", time.Now().Unix())
	if power <= 0 {
		payload = fmt.Sprintf(`{"transId":"%s","cmd":{"name":"charge","arg":"%d"}}`, transId, watts)
	} else {
		payload = fmt.Sprintf(`{"transId":"%s","cmd":{"name":"discharge","arg":"%d"}}`, transId, watts)
	}

	return transId, payload
}

func (fa *Ferroamp) sendControlRequest(ctx context.Context, transId string, payload string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("battery control request to ferroamp not sent: %w", err)
	}

	// Paho silently drops QoS 0 publishes while reconnecting and reports success,
	// so the connection state must be checked before publishing.
	if !fa.mtqqClient.IsConnectionOpen() {
		return errNotConnected
	}

	// Registered before publishing, so that a fast response always finds its request.
	response := make(chan *ControlResponseMessage, 1)
	fa.pendingMutex.Lock()
	fa.pending[transId] = pendingRequest{
		TransId:    transId,
		Payload:    payload,
		SentAt:     time.Now(),
		ResponseCh: response,
	}
	fa.pendingMutex.Unlock()
	defer fa.takePending(transId)

	token := fa.mtqqClient.Publish("extapi/control/request", 0, false, payload)
	if !token.WaitTimeout(publishTimeout) {
		return errPublishTimeout
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("error when sending battery control request to ferroamp: %w", err)
	}
	fa.logger.Debug("successfully sent battery control request to ferroamp, waiting for ack/nak...")

	timeout := time.NewTimer(controlResponseTimeout)
	defer timeout.Stop()
	select {
	case crm := <-response:
		if crm.Status != statusAck {
			return fmt.Errorf("%w with status %q: %s (transId %s)", errRejected, crm.Status, crm.Message, transId)
		}
		return nil
	case <-timeout.C:
		return fmt.Errorf("%w within %s (transId %s)", errNoResponse, controlResponseTimeout, transId)
	case <-ctx.Done():
		return fmt.Errorf("stopped waiting for response from ferroamp (transId %s): %w", transId, ctx.Err())
	}
}

// takePending removes and returns the pending request for a transaction. A
// request can only be taken once, which guarantees that its ResponseCh
// receives at most one response and that sending on it never blocks.
func (fa *Ferroamp) takePending(transId string) (pendingRequest, bool) {
	fa.pendingMutex.Lock()
	defer fa.pendingMutex.Unlock()
	e, exists := fa.pending[transId]
	delete(fa.pending, transId)
	return e, exists
}

// SetBatteryAuto hands the battery control back to Ferroamp. It blocks until
// Ferroamp has responded, the request times out or ctx is done.
func (fa *Ferroamp) SetBatteryAuto(ctx context.Context) error {
	transId := fmt.Sprintf("solarplant-%d", time.Now().Unix())
	payload := fmt.Sprintf(`{"transId":"%s","cmd":{"name":"auto"}}`, transId)
	fa.logger.Info("setting ferroamp battery in auto mode", "payload", payload)
	return fa.sendControlRequest(ctx, transId, payload)
}

// SetBatteryLoad sets the battery power in kW, where positive values equals
// discharge and negative charge. It blocks until Ferroamp has responded, the
// request times out or ctx is done.
func (fa *Ferroamp) SetBatteryLoad(ctx context.Context, power float64) error {
	transId, payload := fa.formatPayload(power)
	fa.logger.Info("sending new battery load to ferroamp", "power", power, "payload", payload)
	return fa.sendControlRequest(ctx, transId, payload)
}

func (fa *Ferroamp) handleEsoFaultCode(newFaultCode uint16) {
	if fa.lastEsoFaultCode == newFaultCode {
		return // Nothing changed
	}

	for bitValue, desc := range esoFaultsCodes {
		hexCode := fmt.Sprintf("0x%04x", bitValue)

		// Check if fault code is new
		if fa.lastEsoFaultCode&bitValue == 0 && newFaultCode&bitValue != 0 {
			fa.logger.Warn(fmt.Sprintf("new fault code (%s) from ESO: %s", hexCode, desc), slog.String("faultCode", hexCode))
		}
		// Check if fault code is cleared
		if fa.lastEsoFaultCode&bitValue != 0 && newFaultCode&bitValue == 0 {
			fa.logger.Debug(fmt.Sprintf("cleared fault code (%s) from ESO", hexCode), slog.String("faultCode", hexCode))
		}
	}

	fa.lastEsoFaultCode = newFaultCode
}

func (fa *Ferroamp) inactivityWatchdog() {
	inWarnState, inErrorState := false, false
	inactivityWarnTimeout := 10 * time.Second
	inactivityErrorTimeout := 60 * time.Second
	fa.lastMessageTime.Reset()
	fa.stopMonitorCh = make(chan struct{})

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Warn if no traffic
				if fa.lastMessageTime.Elapsed() >= inactivityWarnTimeout {
					if !inWarnState {
						inWarnState = true
						fa.logger.Warn(fmt.Sprintf("no incoming mqtt traffic for the last %.0f seconds", inactivityWarnTimeout.Seconds()))
					}
				} else {
					if inWarnState {
						inWarnState = false
						fa.logger.Info("mqtt traffic is restored")
					}
				}
				// Signal inactivity if no traffic for a longer time
				if fa.lastMessageTime.Elapsed() >= inactivityErrorTimeout {
					if !inErrorState {
						inErrorState = true
						fa.logger.Error(fmt.Sprintf("no incoming mqtt traffic for the last %.0f seconds", inactivityErrorTimeout.Seconds()))
						if fa.OnInactivity != nil {
							fa.OnInactivity()
						}
					}
				} else {
					if inErrorState {
						inErrorState = false
					}
				}

			case <-fa.stopMonitorCh:
				fa.logger.Debug("stopping ferroamp monitor routine")
				return
			}
		}
	}()
}
