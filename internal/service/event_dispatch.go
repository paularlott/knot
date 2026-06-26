package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/health"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/object"
)

const (
	SinkQueueSize      = 100
	WebhookTimeout     = 10 * time.Second
	RetryAttempts      = 3
	RetryDelay1        = 5 * time.Second
	RetryDelay2        = 15 * time.Second
	TombstoneRetention = 5 * time.Minute
)

type EventEnvelope struct {
	EventId   string
	EventType string
	SpaceId   string
	UserId    string
	Payload   map[string]interface{}
	Ts        hlc.Timestamp
	Actor     EventActor
}

type EventActor struct {
	Id       string
	Username string
	Kind     string
}

type InFlightEntry struct {
	EventId       string
	EventType     string
	SinkId        string
	UserId        string
	SpaceId       string
	Payload       map[string]interface{}
	ActorId       string
	ActorName     string
	ActorKind     string
	Status        string
	Attempts      uint32
	NextAttemptAt time.Time
	LastError     string
	Version       hlc.Timestamp
	TombstonedAt  time.Time
}

type sinkQueue struct {
	mu    sync.Mutex
	queue []*deliveryTask
}

type deliveryTask struct {
	envelope *EventEnvelope
	sink     *model.EventSink
}

type JSONRPCSubscription struct {
	SpaceId    string
	UserId     string
	MethodName string
	LocalName  string
	Events     []string
	EventSinks []string
}

type EventDispatcher struct {
	mu                sync.RWMutex
	inFlight          map[string]*InFlightEntry
	processed         map[string]time.Time
	jsonrpcDelivered  map[string]time.Time
	queues            map[string]*sinkQueue
	sinkCache         []*model.EventSink
	subscriptions     map[string][]*JSONRPCSubscription
	pendingDeliveries map[string]int
	httpClient        *http.Client
	insecureClient    *http.Client
}

var (
	dispatcher     *EventDispatcher
	dispatcherOnce sync.Once
)

func GetEventDispatcher() *EventDispatcher {
	dispatcherOnce.Do(func() {
		dispatcher = &EventDispatcher{
			inFlight:          make(map[string]*InFlightEntry),
			processed:         make(map[string]time.Time),
			jsonrpcDelivered:  make(map[string]time.Time),
			queues:            make(map[string]*sinkQueue),
			subscriptions:     make(map[string][]*JSONRPCSubscription),
			pendingDeliveries: make(map[string]int),
			httpClient: &http.Client{
				Timeout: WebhookTimeout,
			},
			insecureClient: &http.Client{
				Timeout: WebhookTimeout,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			},
		}

		go func() {
			ticker := time.NewTicker(TombstoneRetention)
			defer ticker.Stop()
			for range ticker.C {
				dispatcher.mu.Lock()
				dispatcher.cleanupTombstoned()
				dispatcher.mu.Unlock()
			}
		}()
	})
	return dispatcher
}

func (d *EventDispatcher) Dispatch(envelope *EventEnvelope) {
	transport := GetTransport()
	if transport != nil && !transport.IsLeader() {
		d.recordInFlight(envelope, "")
		return
	}

	d.processEvent(envelope)
	d.deliverToSubscriptions(envelope)
}

// ReplayPending is called when this node becomes the zone leader. It scans
// the in-flight map for entries that were not completed (status pending,
// attempting, or retry) and re-processes them. Since all servers receive
// every event, non-leaders have recorded these entries but never dispatched
// them. On leadership change, the new leader picks up where the old one
// left off. Consumers may see duplicate deliveries (at-least-once) — they
// dedup via the event UUID.
func (d *EventDispatcher) ReplayPending() {
	d.mu.RLock()
	seen := make(map[string]bool)
	var pending []*InFlightEntry
	for _, entry := range d.inFlight {
		if entry.Status == "pending" || entry.Status == "attempting" || entry.Status == "retry" {
			if !seen[entry.EventId] {
				seen[entry.EventId] = true
				pending = append(pending, entry)
			}
		}
	}
	d.mu.RUnlock()

	if len(pending) == 0 {
		return
	}

	log.Info("replaying pending event deliveries", "count", len(pending))

	for _, entry := range pending {
		d.mu.Lock()
		delete(d.processed, entry.EventId)
		delete(d.jsonrpcDelivered, entry.EventId)
		d.mu.Unlock()

		envelope := &EventEnvelope{
			EventId:   entry.EventId,
			EventType: entry.EventType,
			SpaceId:   entry.SpaceId,
			UserId:    entry.UserId,
			Payload:   entry.Payload,
			Ts:        entry.Version,
			Actor: EventActor{
				Id:       entry.ActorId,
				Username: entry.ActorName,
				Kind:     entry.ActorKind,
			},
		}
		d.processEvent(envelope)
	}
}

func (d *EventDispatcher) processEvent(envelope *EventEnvelope) {
	d.mu.Lock()
	if _, seen := d.processed[envelope.EventId]; seen {
		d.mu.Unlock()
		return
	}
	d.processed[envelope.EventId] = time.Time{}
	d.mu.Unlock()

	d.mu.RLock()
	sinks := d.sinkCache
	d.mu.RUnlock()

	d.mu.Lock()
	d.processed[envelope.EventId] = time.Now()
	d.mu.Unlock()

	var matched []*model.EventSink
	for _, sink := range sinks {
		if sink.IsDeleted || !sink.Active {
			continue
		}
		if sink.SinkType == "json-rpc" {
			continue
		}
		if !sink.MatchEventType(envelope.EventType) {
			continue
		}
		if !d.isVisible(sink, envelope) {
			continue
		}
		matched = append(matched, sink)
	}

	// Reserve the pending count for every matched sink before starting any
	// delivery. Otherwise a fast delivery could decrement the counter to zero
	// and fire a premature "done" notification before the remaining sinks are
	// counted.
	d.mu.Lock()
	d.pendingDeliveries[envelope.EventId] += len(matched)
	d.mu.Unlock()

	for _, sink := range matched {
		if !d.enqueue(sink, envelope) {
			// Queue was full and the task dropped; release its reservation.
			d.decrementPending(envelope.EventId)
		}
	}

	if len(matched) == 0 {
		d.mu.Lock()
		d.checkEventComplete(envelope.EventId)
		d.mu.Unlock()
	}
}

func (d *EventDispatcher) isVisible(sink *model.EventSink, envelope *EventEnvelope) bool {
	if sink.IsGlobalSink() {
		return true
	}
	return sink.UserId == envelope.UserId
}

func (d *EventDispatcher) getQueue(sinkId string) *sinkQueue {
	d.mu.Lock()
	defer d.mu.Unlock()
	q, ok := d.queues[sinkId]
	if !ok {
		q = &sinkQueue{}
		d.queues[sinkId] = q
	}
	return q
}

// enqueue appends a delivery task and starts a worker. It returns false if the
// sink queue was full and the task was dropped, so the caller can release the
// pending-delivery reservation it made for this sink.
func (d *EventDispatcher) enqueue(sink *model.EventSink, envelope *EventEnvelope) bool {
	q := d.getQueue(sink.Id)
	q.mu.Lock()
	if len(q.queue) >= SinkQueueSize {
		q.mu.Unlock()
		log.Warn("event sink queue full, dropping event", "sink_id", sink.Id, "event_id", envelope.EventId)
		logAudit(model.AuditEventEventSinkDropped,
			fmt.Sprintf("Event sink %s queue full, dropped event %s", sink.Name, envelope.EventId),
			map[string]interface{}{"sink_id": sink.Id, "event_id": envelope.EventId})
		return false
	}
	q.queue = append(q.queue, &deliveryTask{envelope: envelope, sink: sink})
	q.mu.Unlock()

	go d.deliver(sink.Id)
	return true
}

func (d *EventDispatcher) deliver(sinkId string) {
	q := d.getQueue(sinkId)
	q.mu.Lock()
	if len(q.queue) == 0 {
		q.mu.Unlock()
		return
	}
	task := q.queue[0]
	q.queue = q.queue[1:]
	q.mu.Unlock()

	d.recordInFlight(task.envelope, sinkId)

	for attempt := uint32(1); attempt <= RetryAttempts; attempt++ {
		err := d.deliverOnce(task)
		if err == nil {
			d.markDone(task.envelope.EventId, sinkId)
			return
		}

		log.Warn("event delivery attempt failed",
			"sink_id", sinkId,
			"event_id", task.envelope.EventId,
			"attempt", attempt,
			"error", err)

		if attempt < RetryAttempts {
			d.markRetry(task.envelope.EventId, sinkId, attempt, err.Error())
			delay := RetryDelay1
			if attempt == 2 {
				delay = RetryDelay2
			}
			time.Sleep(delay)
		}
	}

	d.markGivenUp(task.envelope.EventId, sinkId, task)
}

func (d *EventDispatcher) deliverOnce(task *deliveryTask) error {
	switch task.sink.SinkType {
	case "webhook":
		return d.deliverWebhook(task)
	case "script":
		return d.deliverScript(task)
	default:
		return fmt.Errorf("unknown sink type: %s", task.sink.SinkType)
	}
}

func (d *EventDispatcher) deliverWebhook(task *deliveryTask) error {
	if task.sink.Webhook == nil {
		return fmt.Errorf("webhook config is nil")
	}

	body, err := model.RenderEventTemplate(task.sink.Webhook.BodyTemplate, d.envelopeToRenderData(task.envelope))
	if err != nil {
		return fmt.Errorf("template render error: %w", err)
	}

	req, err := http.NewRequest("POST", task.sink.Webhook.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Knot-Event-Id", task.envelope.EventId)
	req.Header.Set("X-Knot-Event-Type", task.envelope.EventType)
	req.Header.Set("X-Knot-Event-Ts", task.envelope.Ts.Time().Format(time.RFC3339Nano))

	mac := hmac.New(sha256.New, []byte(task.sink.Webhook.Secret))
	mac.Write(body)
	req.Header.Set("X-Knot-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))

	for k, v := range task.sink.Webhook.Headers {
		req.Header.Set(k, v)
	}

	client := d.httpClient
	if task.sink.Webhook.SkipTLSVerify {
		client = d.insecureClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}

	return nil
}

func (d *EventDispatcher) deliverScript(task *deliveryTask) error {
	db := database.GetInstance()
	script, err := db.GetScript(task.sink.ScriptId)
	if err != nil {
		return fmt.Errorf("failed to load sink script: %w", err)
	}
	if script.IsDeleted || !script.Active {
		return fmt.Errorf("sink script is not active")
	}

	userId := task.sink.UserId
	if userId == "" {
		userId = task.envelope.UserId
	}
	user, err := db.GetUser(userId)
	if err != nil || user == nil {
		return fmt.Errorf("failed to load user for script execution: %w", err)
	}

	params := make(map[string]object.Object)
	if task.envelope.Payload != nil {
		for k, v := range task.envelope.Payload {
			params[k] = conversion.FromGo(v)
		}
	}

	_, err = ExecuteEventScript(script, params, user, task.envelope)
	if err != nil {
		logAudit(model.AuditEventEventSinkScriptFailed,
			fmt.Sprintf("Event sink %s script failed for event %s: %s", task.sink.Name, task.envelope.EventId, err.Error()),
			map[string]interface{}{"sink_id": task.sink.Id, "event_id": task.envelope.EventId, "error": err.Error()})
		return err
	}

	return nil
}

func inFlightKey(eventId, sinkId string) string {
	return eventId + ":" + sinkId
}

func (d *EventDispatcher) recordInFlight(envelope *EventEnvelope, sinkId string) {
	key := inFlightKey(envelope.EventId, sinkId)
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.inFlight[key]; !ok {
		d.inFlight[key] = &InFlightEntry{
			EventId:   envelope.EventId,
			EventType: envelope.EventType,
			UserId:    envelope.UserId,
			SpaceId:   envelope.SpaceId,
			SinkId:    sinkId,
			Payload:   envelope.Payload,
			ActorId:   envelope.Actor.Id,
			ActorName: envelope.Actor.Username,
			ActorKind: envelope.Actor.Kind,
			Status:    "pending",
			Version:   hlc.Now(),
		}
	}
}

func (d *EventDispatcher) markDone(eventId, sinkId string) {
	key := inFlightKey(eventId, sinkId)
	d.mu.Lock()
	if entry, ok := d.inFlight[key]; ok {
		entry.Status = "done"
		entry.TombstonedAt = time.Now().UTC()
		entry.Version = hlc.Now()
	}
	d.mu.Unlock()
	d.decrementPending(eventId)
}

func (d *EventDispatcher) markRetry(eventId, sinkId string, attempt uint32, lastError string) {
	key := inFlightKey(eventId, sinkId)
	d.mu.Lock()
	defer d.mu.Unlock()
	if entry, ok := d.inFlight[key]; ok {
		entry.Status = "retry"
		entry.Attempts = attempt
		entry.LastError = lastError
		entry.Version = hlc.Now()
	}
}

func (d *EventDispatcher) markGivenUp(eventId, sinkId string, task *deliveryTask) {
	key := inFlightKey(eventId, sinkId)
	d.mu.Lock()
	if entry, ok := d.inFlight[key]; ok {
		entry.Status = "given_up"
		entry.TombstonedAt = time.Now().UTC()
		entry.Version = hlc.Now()
	}
	d.mu.Unlock()

	eventName := model.AuditEventEventSinkDeliveryFailed
	if task.sink.SinkType == "script" {
		eventName = model.AuditEventEventSinkScriptFailed
	}
	logAudit(eventName,
		fmt.Sprintf("Event sink %s gave up delivering event %s after %d attempts", task.sink.Name, eventId, RetryAttempts),
		map[string]interface{}{"sink_id": sinkId, "event_id": eventId})

	d.decrementPending(eventId)
}

func (d *EventDispatcher) cleanupTombstoned() {
	now := time.Now().UTC()
	for id, entry := range d.inFlight {
		if !entry.TombstonedAt.IsZero() && now.Sub(entry.TombstonedAt) > TombstoneRetention {
			delete(d.inFlight, id)
		}
	}
	for id, ts := range d.processed {
		if ts.IsZero() {
			continue
		}
		if now.Sub(ts) > TombstoneRetention {
			delete(d.processed, id)
		}
	}
	for id, ts := range d.jsonrpcDelivered {
		if now.Sub(ts) > TombstoneRetention {
			delete(d.jsonrpcDelivered, id)
		}
	}
	for _, entry := range d.inFlight {
		if entry.TombstonedAt.IsZero() && entry.Status == "pending" {
			eventTime := entry.Version.Time()
			if !eventTime.IsZero() && now.Sub(eventTime) > MaxPendingAge {
				entry.TombstonedAt = now
				entry.Status = "expired"
				entry.Version = hlc.Now()
			}
		}
	}
}

const MaxPendingAge = 10 * time.Minute

// decrementPending decrements the pending delivery counter for an event.
// When the counter reaches zero, notifies zone members that the event is done.
func (d *EventDispatcher) decrementPending(eventId string) {
	var notify bool
	d.mu.Lock()
	d.pendingDeliveries[eventId]--
	if d.pendingDeliveries[eventId] <= 0 {
		delete(d.pendingDeliveries, eventId)
		notify = true
	}
	d.mu.Unlock()

	if notify {
		if transport := GetTransport(); transport != nil {
			transport.NotifyEventDone(eventId)
		}
	}
}

// checkEventComplete is called after initial dispatch to handle events with
// zero matching deliveries (immediately complete).
func (d *EventDispatcher) checkEventComplete(eventId string) {
	if d.pendingDeliveries[eventId] <= 0 {
		delete(d.pendingDeliveries, eventId)
		go func() {
			if transport := GetTransport(); transport != nil {
				transport.NotifyEventDone(eventId)
			}
		}()
	}
}

// MarkEventDone tombstones all in-flight entries for an event. Called by
// non-leaders on receipt of an EventDoneMsg from the leader.
func (d *EventDispatcher) MarkEventDone(eventId string) {
	now := time.Now().UTC()
	d.mu.Lock()
	prefix := eventId + ":"
	for key, entry := range d.inFlight {
		if strings.HasPrefix(key, prefix) {
			if entry.TombstonedAt.IsZero() {
				entry.TombstonedAt = now
				entry.Status = "done"
				entry.Version = hlc.Now()
			}
		}
	}
	d.mu.Unlock()
}

// MergeInFlight merges incoming in-flight records from gossip. Entries with
// a newer HLC version overwrite local copies; entries we don't have are added.
func (d *EventDispatcher) MergeInFlight(entries []*InFlightEntry) {
	d.mu.Lock()
	for _, incoming := range entries {
		key := inFlightKey(incoming.EventId, incoming.SinkId)
		if local, ok := d.inFlight[key]; ok {
			if incoming.Version.After(local.Version) {
				d.inFlight[key] = incoming
			}
		} else {
			d.inFlight[key] = incoming
		}
	}
	d.mu.Unlock()
}

// GetEntriesForGossip returns all in-flight entries, including tombstoned
// ones. Used for periodic gossip to zone peers. Tombstones must be gossiped
// so peers that missed the direct done notification (e.g. they were down)
// learn the entry is complete and remove it after the retention window;
// otherwise the record would live on those peers until MaxPendingAge.
func (d *EventDispatcher) GetEntriesForGossip() []*InFlightEntry {
	var entries []*InFlightEntry

	d.mu.RLock()
	for _, entry := range d.inFlight {
		entries = append(entries, entry)
	}
	d.mu.RUnlock()

	return entries
}

// recordInFlightLocked is recordInFlight without locking (caller holds lock).
func (d *EventDispatcher) recordInFlightLocked(envelope *EventEnvelope, sinkId string) {
	key := inFlightKey(envelope.EventId, sinkId)
	if _, exists := d.inFlight[key]; exists {
		return
	}
	d.inFlight[key] = &InFlightEntry{
		EventId:   envelope.EventId,
		EventType: envelope.EventType,
		SinkId:    sinkId,
		UserId:    envelope.UserId,
		SpaceId:   envelope.SpaceId,
		Payload:   envelope.Payload,
		ActorId:   envelope.Actor.Id,
		ActorName: envelope.Actor.Username,
		ActorKind: envelope.Actor.Kind,
		Status:    "pending",
		Version:   hlc.Now(),
	}
}

// ReloadSinks refreshes the in-memory sink cache from the database.
func (d *EventDispatcher) ReloadSinks() {
	sinks, err := database.GetInstance().GetEventSinks()
	if err != nil {
		log.Error("failed to load event sinks into cache", "error", err)
		return
	}
	d.mu.Lock()
	d.sinkCache = sinks
	d.mu.Unlock()
}

// RegisterSubscriptions extracts event subscriptions from method definitions
// and registers them for json-rpc event delivery. Called when methods are
// registered by a running session.
func (d *EventDispatcher) RegisterSubscriptions(spaceId, userId string, methods []methods.MethodDefinition) {
	var subs []*JSONRPCSubscription
	for _, m := range methods {
		if len(m.Events) == 0 {
			continue
		}
		subs = append(subs, &JSONRPCSubscription{
			SpaceId:    spaceId,
			UserId:     userId,
			MethodName: m.Name,
			LocalName:  m.LocalName,
			Events:     m.Events,
			EventSinks: m.EventSinks,
		})
	}
	d.mu.Lock()
	d.subscriptions[spaceId] = subs
	d.mu.Unlock()

	log.Debug("registered json-rpc event subscriptions", "space_id", spaceId, "count", len(subs))
}

// UnregisterSubscriptions removes all subscriptions for a space. Called when
// a session disconnects or methods are unregistered.
func (d *EventDispatcher) UnregisterSubscriptions(spaceId string) {
	d.mu.Lock()
	delete(d.subscriptions, spaceId)
	d.mu.Unlock()
}

// JSONRPCCaller is the callback used to invoke a method on a running session.
// Set by agent_server.ListenAndServe to avoid a circular dependency.
type JSONRPCCaller func(spaceId, localMethod string, params json.RawMessage) error

var jsonrpcCaller JSONRPCCaller

func SetJSONRPCCaller(fn JSONRPCCaller) {
	jsonrpcCaller = fn
}

func matchEventType(patterns []string, eventType string) bool {
	for _, pattern := range patterns {
		if pattern == "*" {
			return true
		}
		if len(pattern) > 2 && pattern[len(pattern)-2:] == ".*" {
			prefix := pattern[:len(pattern)-1]
			if len(eventType) >= len(prefix) && eventType[:len(prefix)] == prefix {
				return true
			}
			continue
		}
		if pattern == eventType {
			return true
		}
	}
	return false
}

// deliverToSubscriptions delivers events to running json-rpc methods that have
// subscribed via `events` annotations. Runs on the zone leader (same model
// as webhook/script delivery). `event_sinks` on the subscription provides
// optional formatting only — the referenced sinks' body templates transform
// the payload before delivery.
func (d *EventDispatcher) deliverToSubscriptions(envelope *EventEnvelope) {
	d.mu.Lock()
	if _, seen := d.jsonrpcDelivered[envelope.EventId]; seen {
		d.mu.Unlock()
		return
	}
	d.jsonrpcDelivered[envelope.EventId] = time.Now()
	d.mu.Unlock()

	if jsonrpcCaller == nil {
		return
	}

	d.mu.RLock()
	var matched []struct {
		sub       *JSONRPCSubscription
		template  string
		hasCustom bool
	}
	for _, subs := range d.subscriptions {
		for _, sub := range subs {
			if sub.UserId != envelope.UserId {
				continue
			}

			if !matchEventType(sub.Events, envelope.EventType) {
				continue
			}

			var customTemplate string
			hasCustom := false

			for _, sinkName := range sub.EventSinks {
				for _, sink := range d.sinkCache {
					if sink.Name != sinkName || sink.SinkType != "json-rpc" || !sink.Active || sink.IsDeleted {
						continue
					}
					if sink.UserId != "" && sink.UserId != sub.UserId {
						continue
					}
					if !sink.MatchEventType(envelope.EventType) {
						continue
					}
					if sink.Webhook != nil {
						customTemplate = sink.Webhook.BodyTemplate
					}
					hasCustom = true
					break
				}
				if hasCustom {
					break
				}
			}

			matched = append(matched, struct {
				sub       *JSONRPCSubscription
				template  string
				hasCustom bool
			}{sub, customTemplate, hasCustom})
		}
	}
	d.mu.RUnlock()

	d.mu.Lock()
	d.pendingDeliveries[envelope.EventId] += len(matched)
	d.mu.Unlock()

	for _, m := range matched {
		go d.deliverJSONRPCWithRetry(envelope, m.sub, m.template, m.hasCustom)
	}
}

func (d *EventDispatcher) deliverJSONRPCWithRetry(envelope *EventEnvelope, sub *JSONRPCSubscription, template string, hasCustom bool) {
	defer d.decrementPending(envelope.EventId)

	renderData := d.envelopeToRenderData(envelope)

	var bodyTemplate string
	if hasCustom {
		bodyTemplate = template
	}
	params, err := model.RenderEventTemplate(bodyTemplate, renderData)
	if err != nil {
		log.Error("json-rpc event template render failed",
			"event_id", envelope.EventId,
			"method", sub.MethodName,
			"space_id", sub.SpaceId,
			"error", err)
		return
	}

	for attempt := uint32(1); attempt <= RetryAttempts; attempt++ {
		err := jsonrpcCaller(sub.SpaceId, sub.LocalName, params)
		if err == nil {
			return
		}

		log.Warn("json-rpc event delivery attempt failed",
			"event_id", envelope.EventId,
			"method", sub.MethodName,
			"space_id", sub.SpaceId,
			"attempt", attempt,
			"error", err)

		if attempt < RetryAttempts {
			delay := RetryDelay1
			if attempt == 2 {
				delay = RetryDelay2
			}
			time.Sleep(delay)
		}
	}

	logAudit(model.AuditEventEventSinkDeliveryFailed,
		fmt.Sprintf("JSON-RPC method %s gave up delivering event %s after %d attempts", sub.MethodName, envelope.EventId, RetryAttempts),
		map[string]interface{}{"method": sub.MethodName, "space_id": sub.SpaceId, "event_id": envelope.EventId})
}

func RaiseSystemEvent(eventType, spaceId, userId string, payload map[string]interface{}) {
	id, err := uuid.NewV7()
	if err != nil {
		log.Error("failed to generate event id", "error", err)
		return
	}

	envelope := &EventEnvelope{
		EventId:   id.String(),
		EventType: eventType,
		SpaceId:   spaceId,
		UserId:    userId,
		Payload:   payload,
		Ts:        hlc.Now(),
		Actor: EventActor{
			Id:   userId,
			Kind: model.AuditActorTypeSystem,
		},
	}

	log.Debug("raise system event", "event_type", eventType, "event_id", id.String(), "space_id", spaceId)

	transport := GetTransport()
	if transport != nil {
		transport.BroadcastEvent(envelope)
	}

	GetEventDispatcher().Dispatch(envelope)
}

func RaiseCustomEvent(eventId, eventType, spaceId, userId string, payload map[string]interface{}) {
	if !strings.HasPrefix(eventType, "custom.") {
		eventType = "custom." + eventType
	}

	if eventId == "" {
		id, err := uuid.NewV7()
		if err != nil {
			log.Error("failed to generate event id", "error", err)
			return
		}
		eventId = id.String()
	}

	username := ""
	if userId != "" {
		if user, err := database.GetInstance().GetUser(userId); err == nil && user != nil {
			username = user.Username
		}
	}

	envelope := &EventEnvelope{
		EventId:   eventId,
		EventType: eventType,
		SpaceId:   spaceId,
		UserId:    userId,
		Payload:   payload,
		Ts:        hlc.Now(),
		Actor: EventActor{
			Id:       userId,
			Username: username,
			Kind:     model.AuditActorTypeUser,
		},
	}

	transport := GetTransport()
	if transport != nil {
		transport.BroadcastEvent(envelope)
	}

	GetEventDispatcher().Dispatch(envelope)
}

func MaskWebhookSecret(secret string) string {
	if len(secret) <= 4 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:2] + strings.Repeat("*", len(secret)-4) + secret[len(secret)-2:]
}

func SetSpaceHealth(spaceId string, healthy bool, failures uint32) {
	prev := health.Get(spaceId)
	health.Set(spaceId, healthy, failures)

	transitioned := false
	if prev == nil {
		transitioned = true
	} else if prev.Healthy != healthy {
		transitioned = true
	}

	if !transitioned {
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil {
		return
	}

	if healthy {
		RaiseSystemEvent("space.healthy", space.Id, space.UserId, map[string]interface{}{
			"space_name": space.Name,
			"space_id":   space.Id,
			"previous":   "unhealthy",
			"current":    "healthy",
			"checked_at": time.Now().UTC().Format(time.RFC3339Nano),
		})
	} else {
		RaiseSystemEvent("space.unhealthy", space.Id, space.UserId, map[string]interface{}{
			"space_name":           space.Name,
			"space_id":             space.Id,
			"previous":             "healthy",
			"current":              "unhealthy",
			"consecutive_failures": failures,
			"checked_at":           time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
}

func (d *EventDispatcher) envelopeToRenderData(env *EventEnvelope) *model.EventRenderData {
	data := &model.EventRenderData{
		EventId:   env.EventId,
		EventType: env.EventType,
		SpaceId:   env.SpaceId,
		UserId:    env.UserId,
		Payload:   env.Payload,
		Ts:        env.Ts,
		ActorId:   env.Actor.Id,
		ActorName: env.Actor.Username,
		ActorKind: env.Actor.Kind,
		PortURLs:  map[string]string{},
	}

	if env.SpaceId != "" {
		db := database.GetInstance()
		space, err := db.GetSpace(env.SpaceId)
		if err == nil && space != nil {
			data.SpaceName = space.Name

			user, err := db.GetUser(space.UserId)
			if err == nil && user != nil {
				data.Username = user.Username
			}

			if space.PoolId != "" {
				pool, err := db.GetPoolDefinition(space.PoolId)
				if err == nil && pool != nil && !pool.IsDeleted {
					data.PoolName = pool.Name
				}
			}

			data.PortURLs = buildPortURLs(space, data.Username, data.PoolName)

			if space.CustomFields != nil {
				data.CustomFields = make(map[string]string, len(space.CustomFields))
				for _, field := range space.CustomFields {
					data.CustomFields[field.Name] = field.Value
				}
			}
		}
	}

	return data
}

func resolveSpaceURLs(space *model.Space) map[string]string {
	db := database.GetInstance()

	username := ""
	if user, err := db.GetUser(space.UserId); err == nil && user != nil {
		username = user.Username
	}

	poolName := ""
	if space.PoolId != "" {
		if pool, err := db.GetPoolDefinition(space.PoolId); err == nil && pool != nil && !pool.IsDeleted {
			poolName = pool.Name
		}
	}

	return buildPortURLs(space, username, poolName)
}

func buildPortURLs(space *model.Space, username, poolName string) map[string]string {
	urls := map[string]string{}

	if username == "" {
		log.Debug("buildPortURLs: empty username, skipping")
		return urls
	}

	routingName := space.Name
	if poolName != "" {
		routingName = poolName
	}

	cfg := config.GetServerConfig()
	wildcardDomain := cfg.WildcardDomain
	if wildcardDomain == "" {
		log.Debug("buildPortURLs: empty wildcard domain, skipping")
		return urls
	}
	if wildcardDomain[0] == '*' {
		wildcardDomain = wildcardDomain[1:]
	}
	if wildcardDomain != "" && wildcardDomain[0] != '.' {
		wildcardDomain = "." + wildcardDomain
	}

	if space.TemplateId == "" {
		log.Debug("buildPortURLs: space has no template_id", "space_id", space.Id)
		return urls
	}

	db := database.GetInstance()
	tmpl, err := db.GetTemplate(space.TemplateId)
	if err != nil || tmpl == nil {
		log.Warn("buildPortURLs: failed to load template", "template_id", space.TemplateId, "error", err)
		return urls
	}

	log.Trace("buildPortURLs", "template", tmpl.Name, "port_count", len(tmpl.Ports), "username", username, "routing_name", routingName, "wildcard_domain", wildcardDomain)

	for _, port := range tmpl.Ports {
		url := "https://" + username + "--" + routingName + "--" + fmt.Sprintf("%d", port.Port) + wildcardDomain
		urls[port.Name] = url
		log.Trace("buildPortURLs: added", "port_name", port.Name, "port", port.Port, "url", url)
	}

	return urls
}

func logAudit(event, details string, properties map[string]interface{}) {
	entry := model.NewAuditLogEntry("system", model.AuditActorTypeSystem, event, details, &properties)
	transport := GetTransport()
	if transport != nil {
		transport.GossipAuditLog(entry)
	}
	if database.GetInstance().HasAuditLog() {
		if err := database.GetInstance().SaveAuditLog(entry); err == nil {
			sse.PublishAuditLogsChanged()
		}
	}
}
