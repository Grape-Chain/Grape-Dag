package ws

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/VG-Grape/luna/services/rest/mapper"
	"github.com/VG-Grape/luna/tx/pb"
	"github.com/VG-Grape/luna/vm"
	"github.com/gorilla/websocket"
	golog "github.com/ipfs/go-log/v2"
	uuid "github.com/satori/go.uuid"
)

var logger golog.EventLogger = golog.Logger("ws-events")
var activeSubscriptions map[string]*ActiveSubscription = map[string]*ActiveSubscription{}
var subLock sync.RWMutex

func init() {
	vm.RegisterLogCallback(TriggerSubscriptions)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func EventsEndpoint(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Unable to upgrade incoming HTTP connection to WebSocket", err)
	}
	id := uuid.NewV4()
	logger.Infof("Client Connected, %s", id.String())
	reader(id, ws)
	ws.Close()
}

func reader(id uuid.UUID, conn *websocket.Conn) {
	messageType, p, err := conn.ReadMessage()
	if err != nil {
		logger.Errorf("Error reading message from WebSoGcket connection: %s, closing", err.Error())
		return
	}
	req := EventSubscriptionCreateRequest{}
	err = json.Unmarshal(p, &req)
	if err != nil {
		logger.Errorf("Invalid JSON supplied, expected SubscriptionRequest, got: %s", string(p))
		writeToWs(messageType, p, conn)
		return
	}
	logger.Infof("Event subscription received: %s, id %s", string(p), id.String())
	registrationResponse, _ := json.Marshal(SubscriptionRegisteredResponse{Id: id.String()})
	if writeToWs(websocket.TextMessage, registrationResponse, conn) != nil {
		return
	}
	subLock.Lock()
	req.Address = strings.TrimPrefix(req.Address, "0x")
	req.Topics = trimPrefix(req.Topics)
	activeSubscriptions[id.String()] = &ActiveSubscription{id: id, conn: conn, sub: req}
	subLock.Unlock()
	logger.Infof("Event subscription successfully registered %s", id.String())
	changeSubscription(id)
}

func changeSubscription(id uuid.UUID) {
	for {
		subLock.RLock()
		subscription, exists := activeSubscriptions[id.String()]
		if !exists {
			logger.Errorf("Subscription %s doesn't exist, nothing to listen for a change")
			subLock.RUnlock()
			return
		}
		subLock.RUnlock()
		_, message, err := subscription.conn.ReadMessage()
		if err != nil {
			logger.Errorf("error reading message from WebSocket connection: %s, closing", err.Error())
			return
		}
		processSubscriptionChange(id, message, subscription)
	}
}

func processSubscriptionChange(id uuid.UUID, message []byte, subscription *ActiveSubscription) {
	subLock.Lock()
	defer subLock.Unlock()
	subscription.lock.Lock()
	defer subscription.lock.Unlock()

	req := EventSubscriptionChangeRequest{}
	err := json.Unmarshal(message, &req)
	if err != nil {
		errMessage := fmt.Sprintf("Invalid JSON supplied, expected SubscriptionChangeRequest, got: %s", string(message))
		writeError(errMessage, subscription.conn)
		panic(errors.New(errMessage))
	}
	if req.Id != id.String() {
		errMessage := fmt.Sprintf("Change of the subscription with id=%s isn't possible for now, only current subscription id=%s is supported", req.Id, id.String())
		writeError(errMessage, subscription.conn)
		return
	}
	subscription.sub.Address = strings.TrimPrefix(req.Address, "0x")
	subscription.sub.Topics = trimPrefix(req.Topics)
	logger.Infof("Changed %s subscription topics to %v, address to %s", id.String(), req.Topics, req.Address)
}

func triggerSubscriptions(logs []vm.Log) []string {
	subLock.RLock()
	defer subLock.RUnlock()
	removeSubIds := []string{}
	logger.Infof("Search %d active subscriptions to route %d events", len(activeSubscriptions), len(logs))
	for _, l := range logs {
		address := hex.EncodeToString(l.ContractAddress)
		topics := stringTopics(l.Topics)
	OUTER:
		for _, sub := range activeSubscriptions {
			if sub.sub.Address == address {
				for i, requiredTopic := range sub.sub.Topics {
					if requiredTopic != topics[i] {
						continue OUTER
					}
				}
				logger.Infof("Found a match for log=%v and subscription=%v,id=%s", l, sub.sub, sub.id)
				if err := writeEvent(l, sub); err != nil {
					removeSubIds = append(removeSubIds, sub.id.String())
				}
			}
		}
	}
	return removeSubIds
}

func TriggerSubscriptions(logs []vm.Log) {
	removeSubIds := triggerSubscriptions(logs)
	deleteStaleSubscriptions(removeSubIds)
}

func deleteStaleSubscriptions(removeSubIds []string) {
	subLock.Lock()
	defer subLock.Unlock()
	for _, id := range removeSubIds {
		sub, exists := activeSubscriptions[id]
		if !exists {
			logger.Warnf("Subscription is arelady deleted by id=%s", id)
			continue
		}
		logger.Infof("Closing WS connection for subscription id=%s...", id)
		sub.conn.Close()
		delete(activeSubscriptions, id)
		logger.Infof("WS connection for subscription id=%s has been closed", id)
	}
}

func writeEvent(e vm.Log, s *ActiveSubscription) error {
	log := mapper.LogToDto(e)
	s.lock.Lock()
	defer s.lock.Unlock()
	eventJson, err := json.Marshal(log)
	if err != nil {
		panic(fmt.Errorf("Event cannot be marshaled into JSON: %v", e))
	}
	err = writeToWs(websocket.TextMessage, eventJson, s.conn)
	if err != nil {
		return err
	}
	logger.Infof("Event %v published for subscription %s", e, s.id.String())
	return nil
}

func addressFromPb(a *pb.Address) string {
	return hex.EncodeToString(a.AddBytes)
}
func stringTopics(topics [][]byte) []string {
	stringTopics := []string{}
	for _, topic := range topics {
		stringTopics = append(stringTopics, hex.EncodeToString(topic))
	}
	return stringTopics
}
func writeToWs(msgType int, msg []byte, conn *websocket.Conn) error {
	if err := conn.WriteMessage(msgType, msg); err != nil {
		logger.Errorf("WebSocket failed to write message: %s, closing", err.Error())
		return err
	}
	return nil
}

func writeError(errMessage string, conn *websocket.Conn) {
	logger.Error(errMessage)
	errorBytes, _ := json.Marshal(WsError{ErrorMessage: errMessage})
	writeToWs(websocket.TextMessage, errorBytes, conn)
}

func trimPrefix(strs []string) []string {
	result := []string{}
	for _, s := range strs {
		result = append(result, strings.TrimPrefix(s, "0x"))
	}
	return result
}

type EventSubscriptionCreateRequest struct {
	Address string   `json:"address,omitempty"`
	Topics  []string `json:"topics,omitempty"`
}

type EventSubscriptionChangeRequest struct {
	Id string `json:"subscriptionId,omitempty"`
	EventSubscriptionCreateRequest
}

type SubscriptionRegisteredResponse struct {
	Id string `json:"subscriptionId,omitempty"`
}

type ActiveSubscription struct {
	id   uuid.UUID
	conn *websocket.Conn
	sub  EventSubscriptionCreateRequest
	lock sync.Mutex
}

type WsError struct {
	ErrorMessage string `json:"errorMessage,omitempty"`
}
