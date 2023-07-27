package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	jsoniter "github.com/json-iterator/go"
	"github.com/tidwall/gjson"
)

func ParseClientMsgJSON(json []byte) (ClientMsgJSON, error) {
	if !utf8.Valid(json) {
		return nil, fmt.Errorf("non-utf8 bytes: %v", json)
	}

	jsonStr := string(json)

	if !gjson.Valid(jsonStr) {
		return nil, fmt.Errorf("not a json: %q", json)
	}

	arr := gjson.Parse(jsonStr).Array()
	if len(arr) < 2 {
		return nil, fmt.Errorf("too short json array: %q", json)
	}

	if arr[0].Type != gjson.String {
		return nil, fmt.Errorf("client msg arr[0] type is not string: %q", json)
	}

	parsed := make([]interface{}, len(arr)-1)

	for idx, elem := range arr[1:] {
		switch arr[0].Str {
		case "EVENT":
			if idx > 0 {
				return nil, fmt.Errorf("invalid event msg: %q", json)
			}
			ev, err := ParseEventJSON([]byte(elem.Raw))
			if err != nil {
				return nil, fmt.Errorf("invalid event json: %w", err)
			}
			parsed[idx] = ev

		case "REQ":
			if idx == 0 {
				if elem.Type != gjson.String {
					return nil, fmt.Errorf("invalid req msg: %q", json)
				}
				parsed[idx] = elem.Str
			} else {
				fil, err := ParseFilterJSON(elem.Raw)
				if err != nil {
					return nil, fmt.Errorf("invalid filter json: %w", err)
				}
				parsed[idx] = fil
			}

		case "CLOSE":
			if idx > 0 {
				return nil, fmt.Errorf("invalid close msg: %q", json)
			}
			if elem.Type != gjson.String {
				return nil, fmt.Errorf("invalid close msg: %q", json)
			}
			parsed[idx] = elem.Str

		default:
			return nil, fmt.Errorf("unknown msg type: %q", json)
		}
	}

	switch arr[0].Str {
	case "EVENT":
		return NewClientEventMsgJSON(json, parsed[0].(*EventJSON)), nil

	case "REQ":
		if len(parsed) < 2 {
			return nil, fmt.Errorf("invalid req msg: %q", json)
		}
		filters := make([]*FilterJSON, len(parsed)-1)
		for idx, elem := range parsed[1:] {
			filters[idx] = elem.(*FilterJSON)
		}
		return NewClientReqMsgJSON(json, parsed[0].(string), filters), nil

	case "CLOSE":
		return NewClientCloseMsgJSON(json, parsed[0].(string)), nil

	default:
		panic("unreachable")
	}
}

type ClientMsgJSON interface {
	clientMsgJSON()
	Raw() []byte
}

func NewClientEventMsgJSON(raw []byte, json *EventJSON) *ClientEventMsgJSON {
	return &ClientEventMsgJSON{
		raw:       raw,
		EventJSON: json,
	}
}

type ClientEventMsgJSON struct {
	raw       []byte
	EventJSON *EventJSON
}

func (*ClientEventMsgJSON) clientMsgJSON() {}

func (m *ClientEventMsgJSON) Raw() []byte {
	return m.raw
}

func ParseEventJSON(json []byte) (*EventJSON, error) {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	var ev EventJSON
	if err := ji.Unmarshal(json, &ev); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event json: %q", err)
	}

	ev.Raw = json
	return &ev, nil
}

type EventJSON struct {
	ID        string     `json:"id"`
	Pubkey    string     `json:"pubkey"`
	CreatedAt int        `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`

	Raw []byte `json:"-"`
}

func (e *EventJSON) Verify() (bool, error) {
	if e == nil {
		return false, errors.New("empty event cannot be verified")
	}

	ser, err := e.Serialize()
	if err != nil {
		return false, fmt.Errorf("failed to serialize event: %w", err)
	}

	// ID
	hash := sha256.Sum256(ser)

	idBin, err := hex.DecodeString(e.ID)
	if err != nil {
		return false, fmt.Errorf("invalid event id: %w", err)
	}

	if !bytes.Equal(hash[:], idBin) {
		return false, nil
	}

	// Sig
	pKeyBin, err := hex.DecodeString(e.Pubkey)
	if err != nil {
		return false, fmt.Errorf("failed to decode public key: %w", err)
	}

	pKey, err := schnorr.ParsePubKey(pKeyBin)
	if err != nil {
		return false, fmt.Errorf("failed to parse public key: %w", err)
	}

	sigBin, err := hex.DecodeString(e.Sig)
	if err != nil {
		return false, fmt.Errorf("invalid event sig: %w", err)
	}

	sig, err := schnorr.ParseSignature(sigBin)
	if err != nil {
		return false, fmt.Errorf("failed to parse event sig: %w", err)
	}

	return sig.Verify(hash[:], pKey), nil
}

func (e *EventJSON) Serialize() ([]byte, error) {
	if e == nil {
		return nil, errors.New("empty event json cannot be serialized")
	}

	arr := []interface{}{0, e.Pubkey, e.CreatedAt, e.Kind, e.Tags, e.Content}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	res, err := ji.Marshal(arr)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize event: %w", err)
	}

	return res, nil
}

func (e *EventJSON) CreatedAtToTime() time.Time {
	return time.Unix(int64(e.CreatedAt), 0)
}

func NewClientReqMsgJSON(raw []byte, subID string, filters []*FilterJSON) *ClientReqMsgJSON {
	return &ClientReqMsgJSON{
		SubscriptionID: subID,
		FilterJSONs:    filters,
		raw:            raw,
	}
}

type ClientReqMsgJSON struct {
	SubscriptionID string
	FilterJSONs    []*FilterJSON
	raw            []byte
}

func (*ClientReqMsgJSON) clientMsgJSON() {}

func (m *ClientReqMsgJSON) Raw() []byte {
	return m.raw
}

func ParseFilterJSON(json string) (*FilterJSON, error) {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	var fil FilterJSON
	if err := ji.UnmarshalFromString(json, &fil); err != nil {
		return nil, fmt.Errorf("failed to unmarshal filter json: %q", err)
	}

	return &fil, nil
}

type FilterJSON struct {
	IDs     *[]string `json:"ids"`
	Authors *[]string `json:"authors"`
	Kinds   *[]int    `json:"kinds"`
	Etags   *[]string `json:"#e"`
	Ptags   *[]string `json:"#p"`
	Since   *int      `json:"since"`
	Until   *int      `json:"until"`
	Limit   *int      `json:"limit"`
}

func NewClientCloseMsgJSON(raw []byte, subID string) *ClientCloseMsgJSON {
	return &ClientCloseMsgJSON{
		SubscriptionID: subID,
		raw:            raw,
	}
}

type ClientCloseMsgJSON struct {
	SubscriptionID string
	raw            []byte
}

func (*ClientCloseMsgJSON) clientMsgJSON() {}

func (m *ClientCloseMsgJSON) Raw() []byte {
	return m.raw
}

type ServerMsg interface {
	serverMsg()
	json.Marshaler
}

func NewServerEventMsg(subID string, event *Event) *ServerEventMsg {
	return &ServerEventMsg{
		SubscriptionID: subID,
		Event:          event,
	}
}

type ServerEventMsg struct {
	SubscriptionID string
	*Event
}

func (ServerEventMsg) serverMsg() {}

func (msg *ServerEventMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, errors.New("cannot marshal nil server event msg")
	}

	payload := []interface{}{"EVENT", msg.SubscriptionID, msg.Event}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	res, err := ji.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal server event msg: %v", msg)
	}

	return res, nil
}

func NewServerEOSEMsg(subID string) *ServerEOSEMsg {
	return &ServerEOSEMsg{SubscriptionID: subID}
}

type ServerEOSEMsg struct {
	SubscriptionID string
}

func (ServerEOSEMsg) serverMsg() {}

func (msg *ServerEOSEMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, errors.New("cannot marshal nil server eose msg")
	}

	payload := []string{"EOSE", msg.SubscriptionID}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	res, err := ji.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal eose msg: %v", msg)
	}

	return res, nil
}

type ServerNoticeMsg struct {
	Message string
}

func (ServerNoticeMsg) serverMsg() {}

func (msg *ServerNoticeMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, errors.New("cannot marshal nil server notice msg")
	}

	payload := []string{"NOTICE", msg.Message}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	res, err := ji.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notice msg: %v", msg)
	}

	return res, nil
}

const (
	ServerOKMsgPrefixDuplicate   = "duplicate: "
	ServerOKMsgPrefixBlocked     = "blocked: "
	ServerOKMsgPrefixInvalid     = "invalid: "
	ServerOKMsgPrefixRateLimited = "rate-limited: "
	ServerOKMsgPrefixError       = "error: "
)

type ServerOKMsg struct {
	EventID       string
	Succeeded     bool
	MessagePrefix string
	Message       string
}

func NewServerOKMsg(eventID string, succeeded bool, msgPrefix, msg string) *ServerOKMsg {
	return &ServerOKMsg{
		EventID:       eventID,
		Succeeded:     succeeded,
		MessagePrefix: msgPrefix,
		Message:       msg,
	}
}

func (ServerOKMsg) serverMsg() {}

func (msg *ServerOKMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, errors.New("cannot marshal nil server ok msg")
	}

	payload := []interface{}{
		"OK",
		msg.EventID,
		msg.Succeeded,
		msg.MessagePrefix + msg.Message,
	}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	res, err := ji.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ok msg: %v", msg)
	}

	return res, nil
}

func NewEvent(json *EventJSON, receivedAt time.Time) *Event {
	return &Event{
		EventJSON:  json,
		ReceivedAt: receivedAt,
	}
}

type Event struct {
	*EventJSON
	ReceivedAt time.Time
}

func (e *Event) ValidCreatedAt() bool {
	sub := time.Until(e.CreatedAtToTime())
	// TODO(high-moctane) no magic number
	return -10*time.Minute <= sub && sub <= 5*time.Minute
}

func (e *Event) MarshalJSON() ([]byte, error) {
	return []byte(e.Raw), nil
}

func NewFilter(json *FilterJSON) (*Filter, error) {
	if json == nil {
		return nil, errors.New("nil filter")
	}

	if json.IDs != nil {
		for _, id := range *json.IDs {
			if len(id) < Cfg.MinPrefix {
				return nil, errors.New("too short id prefix")
			}
		}
	}

	if json.Authors != nil {
		for _, id := range *json.Authors {
			if len(id) < Cfg.MinPrefix {
				return nil, errors.New("too short author id prefix")
			}
		}
	}

	if json.Ptags != nil {
		for _, id := range *json.Ptags {
			if len(id) < Cfg.MinPrefix {
				return nil, errors.New("too short ptag id prefix")
			}
		}
	}

	if json.Etags != nil {
		for _, id := range *json.Etags {
			if len(id) < Cfg.MinPrefix {
				return nil, errors.New("too short etag id prefix")
			}
		}
	}

	if (json.IDs == nil || len(*json.IDs) == 0) &&
		(json.Authors == nil || len(*json.Authors) == 0) &&
		(json.Ptags == nil || len(*json.Ptags) == 0) &&
		(json.Etags == nil || len(*json.Etags) == 0) {
		return nil, errors.New("empty ids, authors, #p, #e")
	}

	return &Filter{FilterJSON: json}, nil
}

type Filter struct {
	*FilterJSON
}

func (fil *Filter) Match(event *Event) bool {
	return fil.MatchIDs(event) &&
		fil.MatchAuthors(event) &&
		fil.MatchKinds(event) &&
		fil.MatchEtags(event) &&
		fil.MatchPtags(event) &&
		fil.MatchSince(event) &&
		fil.MatchUntil(event)
}

func (fil *Filter) MatchIDs(event *Event) bool {
	if fil == nil || fil.IDs == nil {
		return true
	}

	for _, prefix := range *fil.IDs {
		if strings.HasPrefix(event.ID, prefix) {
			return true
		}
	}
	return false
}

func (fil *Filter) MatchAuthors(event *Event) bool {
	if fil == nil || fil.Authors == nil {
		return true
	}

	for _, prefix := range *fil.Authors {
		if strings.HasPrefix(event.Pubkey, prefix) {
			return true
		}
	}
	return false
}

func (fil *Filter) MatchKinds(event *Event) bool {
	if fil == nil || fil.Kinds == nil {
		return true
	}

	for _, k := range *fil.Kinds {
		if event.Kind == k {
			return true
		}
	}
	return false
}

func (fil *Filter) MatchEtags(event *Event) bool {
	if fil == nil || fil.Etags == nil {
		return true
	}

	for _, id := range *fil.Etags {
		for _, tag := range event.Tags {
			if len(tag) < 2 {
				continue
			}
			if tag[0] == "e" && strings.HasPrefix(tag[1], id) {
				return true
			}
		}
	}
	return false
}

func (fil *Filter) MatchPtags(event *Event) bool {
	if fil == nil || fil.Ptags == nil {
		return true
	}

	for _, id := range *fil.Ptags {
		for _, tag := range event.Tags {
			if len(tag) < 2 {
				continue
			}
			if tag[0] == "p" && strings.HasPrefix(tag[1], id) {
				return true
			}
		}
	}
	return false
}

func (fil *Filter) MatchSince(event *Event) bool {
	return fil == nil || fil.Since == nil || event.CreatedAt > *fil.Since
}

func (fil *Filter) MatchUntil(event *Event) bool {
	return fil == nil || fil.Until == nil || event.CreatedAt < *fil.Until
}

func NewFiltersFromFilterJSONs(jsons []*FilterJSON) (Filters, error) {
	if len(jsons) > Cfg.MaxFilters+2 {
		return nil, fmt.Errorf("filter is too long: %v", jsons)
	}

	res := make(Filters, len(jsons))

	var err error
	for i, json := range jsons {
		res[i], err = NewFilter(json)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

type Filters []*Filter

func (fils Filters) Match(event *Event) bool {
	for _, fil := range fils {
		if fil.Match(event) {
			return true
		}
	}
	return false
}
