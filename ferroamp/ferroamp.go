package ferroamp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
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

type pendingRequest struct {
	TransId string
	Payload string
	SentAt  time.Time
	DoneCh  chan struct{}
}

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
	pendingMutex      sync.RWMutex
	lastEsoFaultCode  uint16
	lastSsoFaultCode  uint16
	lastMessageTime   ConcurrentTimer
	stopPurgeCh       chan struct{}
	stopMonitorCh     chan struct{}
	OnEhubMessage     OnEhubMessage
	OnSsoMessage      OnSsoMessage
	OnEsoMessage      OnEsoMessage
	OnEsmMessage      OnEsmMessage
	OnControlResponse OnControlResponse
	OnControlEvent    OnControlEvent
}

func New(broker string, port int16, username string, password string) *Ferroamp {
	logger := slog.Default().With("module", "ferroamp")
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	opts.SetClientID("solarplant")
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetAutoReconnect(true)
	opts.OnConnect = func(client mqtt.Client) {
		logger.Info("ferroamp MQTT connected")
	}
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		logger.Warn("ferroamp MQTT connection lost", slog.Any("error", err))
	}

	mqttLogger := slog.Default().With("module", "mqtt")
	mqtt.CRITICAL = newMqttLogger(mqttLogger, slog.LevelError)
	mqtt.ERROR = newMqttLogger(mqttLogger, slog.LevelError)
	mqtt.WARN = newMqttLogger(mqttLogger, slog.LevelWarn)

	return &Ferroamp{
		mtqqClient:       mqtt.NewClient(opts),
		logger:           logger,
		pending:          make(map[string]pendingRequest),
		lastEsoFaultCode: 0,
		lastSsoFaultCode: 0,
		lastMessageTime:  ConcurrentTimer{},
	}
}

func (fa *Ferroamp) Connect() error {
	fa.logger.Debug("connecting ferroamp MQTT client")

	if token := fa.mtqqClient.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	fa.inactivityWatchdog()

	token := fa.mtqqClient.SubscribeMultiple(topics, func(client mqtt.Client, msg mqtt.Message) {
		fa.lastMessageTime.Reset()

		switch msg.Topic() {
		case "extapi/data/ehub":
			var ehub EhubMessage
			if err := json.Unmarshal(msg.Payload(), &ehub); err != nil {
				fa.logger.Error("error when reading EHUB message", slog.Any("error", err))
			} else if fa.OnEhubMessage != nil {
				fa.OnEhubMessage(&ehub)
			}

		case "extapi/data/sso":
			var sso SsoMessage
			if err := json.Unmarshal(msg.Payload(), &sso); err != nil {
				fa.logger.Error("error when reading SSO message", slog.Any("error", err))
			} else if fa.OnSsoMessage != nil {
				fa.OnSsoMessage(&sso)
			}

			faultCode := uint16(sso.FaultCode.Value)
			if faultCode > 0 && faultCode != fa.lastSsoFaultCode {
				fa.logger.Warn("fault code from SSO, please contact ferroamp support",
					slog.Any("faultCode", faultCode),
					slog.Any("lastFaultCode", fa.lastSsoFaultCode))
			}
			fa.lastSsoFaultCode = faultCode

		case "extapi/data/eso":
			var eso EsoMessage
			if err := json.Unmarshal(msg.Payload(), &eso); err != nil {
				fa.logger.Error("error when reading ESO message", slog.Any("error", err))
			} else if fa.OnEsoMessage != nil {
				fa.OnEsoMessage(&eso)
			}

			fa.handleEsoFaultCode(uint16(eso.FaultCode.Value))

		case "extapi/data/esm":
			var esm EsmMessage
			if err := json.Unmarshal(msg.Payload(), &esm); err != nil {
				fa.logger.Error("error when reading ESM message", slog.Any("error", err))
			} else if fa.OnEsmMessage != nil {
				fa.OnEsmMessage(&esm)
			}

		case "extapi/control/response":
			var crm ControlResponseMessage
			if err := json.Unmarshal(msg.Payload(), &crm); err != nil {
				fa.logger.Error("error when reading control response", slog.Any("error", err))
			} else {
				func() {
					fa.pendingMutex.RLock()
					defer fa.pendingMutex.RUnlock()
					if e, exists := fa.pending[crm.TransId]; exists {
						duration := time.Since(e.SentAt)
						fa.logger.Debug("received response for known transaction", slog.String("transId", crm.TransId), slog.Duration("duration", duration))
						e.DoneCh <- struct{}{}
					} else if strings.HasPrefix(crm.TransId, "solarplant-") {
						fa.logger.Warn("received response for unknown transaction", slog.String("transId", crm.TransId))
					} else {
						fa.logger.Info("received response for another client", slog.String("transId", crm.TransId), slog.Any("message", crm.Message))
					}

					if fa.OnControlResponse != nil {
						fa.OnControlResponse(&crm)
					}
				}()
			}

		case "extapi/control/event":
			var cem ControlEventMessage
			if err := json.Unmarshal(msg.Payload(), &cem); err != nil {
				fa.logger.Error("error when reading event", slog.Any("error", err))
			} else {
				fa.logger.Info("received control event", "event", cem)
				if fa.OnControlEvent != nil {
					fa.OnControlEvent(&cem)
				}
			}

		default:
			fa.logger.Warn("unknown topic", "topic", msg.Topic())
		}
	})

	if token.Wait() && token.Error() != nil {
		return token.Error()
	}

	fa.startPurgeRoutine()

	return nil
}

func (fa *Ferroamp) Disconnect() {
	fa.logger.Info("disconnecting ferroamp mqtt client")
	if fa.stopPurgeCh != nil {
		close(fa.stopPurgeCh)
		fa.stopPurgeCh = nil
	}
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

func (fa *Ferroamp) sendControlRequest(transId string, payload string) error {
	token := fa.mtqqClient.Publish("extapi/control/request", 0, false, payload)
	ok := token.WaitTimeout(time.Second * 5)
	if !ok {
		return fmt.Errorf("timeout when sending battery control request to ferroamp")
	} else if token.Error() != nil {
		return fmt.Errorf("error when sending battery control request to ferroamp: %w", token.Error())
	} else {
		func() {
			fa.pendingMutex.Lock()
			defer fa.pendingMutex.Unlock()
			fa.pending[transId] = pendingRequest{
				TransId: transId,
				Payload: payload,
				SentAt:  time.Now(),
				DoneCh:  make(chan struct{}),
			}
			fa.logger.Debug("successfully sent battery control request to ferroamp, waiting for ack/nak...")
		}()

		select {
		case <-fa.pending[transId].DoneCh:
		case <-time.After(30 * time.Second):
			fa.logger.Warn("pending request timed out", slog.String("transId", transId))
		}

		return nil
	}
}

func (fa *Ferroamp) SetBatteryAuto() error {
	transId := fmt.Sprintf("solarplant-%d", time.Now().Unix())
	payload := fmt.Sprintf(`{"transId":"%s","cmd":{"name":"auto"}}`, transId)
	fa.logger.Info("setting ferroamp battery in auto mode", "payload", payload)
	return fa.sendControlRequest(transId, payload)
}

/** Positive values (kW) equals discharge, negative charge */
func (fa *Ferroamp) SetBatteryLoad(power float64) error {
	transId, payload := fa.formatPayload(power)
	fa.logger.Info("sending new battery load to ferroamp", "power", power, "payload", payload)
	return fa.sendControlRequest(transId, payload)
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

func (fa *Ferroamp) startPurgeRoutine() {
	fa.stopPurgeCh = make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				func() {
					fa.pendingMutex.Lock()
					defer fa.pendingMutex.Unlock()
					for transId, e := range fa.pending {
						duration := time.Since(e.SentAt)
						if duration > time.Minute {
							fa.logger.Debug("purging previous request", slog.String("transId", transId), slog.Duration("duration", duration))
							close(e.DoneCh)
							delete(fa.pending, transId)
						}
					}
				}()

			case <-fa.stopPurgeCh:
				fa.logger.Debug("stopping purge routine")
				return
			}
		}
	}()
}

func (fa *Ferroamp) inactivityWatchdog() {
	trafficOk := true
	maxElapsed := 10 * time.Second
	panicTimeout := 60 * time.Second
	fa.lastMessageTime.Reset()
	fa.stopMonitorCh = make(chan struct{})

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if fa.lastMessageTime.Elapsed() >= panicTimeout {
					fa.logger.Error(fmt.Sprintf("no incoming mqtt traffic for the last %.0f seconds, terminating...", panicTimeout.Seconds()))
					fa.Disconnect()
					time.Sleep(1 * time.Second)
					os.Exit(1)
				}
				if fa.lastMessageTime.Elapsed() >= maxElapsed {
					if trafficOk {
						// TODO: Maybe implement a reconnect if this turns out to be a problem.
						fa.logger.Warn(fmt.Sprintf("no incoming mqtt traffic for the last %.0f seconds", maxElapsed.Seconds()))
						trafficOk = false
					}
				} else {
					if !trafficOk {
						fa.logger.Info("mqtt traffic is restored")
						trafficOk = true
					}
				}

			case <-fa.stopMonitorCh:
				fa.logger.Debug("stopping ferroamp monitor routine")
				return
			}
		}
	}()
}
