package mocrelay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

var ErrInvalidClientMsg = errors.New("invalid client message")

type ClientMsg interface {
	ClientMsg()
}

func IsNilClientMsg(msg ClientMsg) bool {
	return msg == nil || reflect.ValueOf(msg).IsNil()
}

var clientMsgRegexp = regexp.MustCompile(`^\[\s*"(\w*)"`)

func ParseClientMsg(b []byte) (msg ClientMsg, err error) {
	match := clientMsgRegexp.FindSubmatch(b)
	if len(match) == 0 {
		return nil, errors.New("not a client msg")
	}

	switch string(match[1]) {
	case "EVENT":
		var ret ClientEventMsg
		if err := json.Unmarshal(b, &ret); err != nil {
			return nil, fmt.Errorf("failed to parse client msg: %w", err)
		}
		return &ret, nil

	case "REQ":
		var ret ClientReqMsg
		if err := json.Unmarshal(b, &ret); err != nil {
			return nil, fmt.Errorf("failed to parse client msg: %w", err)
		}
		return &ret, nil

	case "CLOSE":
		var ret ClientCloseMsg
		if err := json.Unmarshal(b, &ret); err != nil {
			return nil, fmt.Errorf("failed to parse client msg: %w", err)
		}
		return &ret, nil

	case "AUTH":
		var ret ClientAuthMsg
		if err := json.Unmarshal(b, &ret); err != nil {
			return nil, fmt.Errorf("failed to parse client msg: %w", err)
		}
		return &ret, nil

	case "COUNT":
		var ret ClientCountMsg
		if err := json.Unmarshal(b, &ret); err != nil {
			return nil, fmt.Errorf("failed to parse client msg: %w", err)
		}
		return &ret, nil

	default:
		var ret ClientUnknownMsg
		if err := json.Unmarshal(b, &ret); err != nil {
			return nil, fmt.Errorf("failed to parse client msg: %w", err)
		}
		return &ret, nil
	}
}

func CheckClientMsg(msg ClientMsg) (bool, error) {
	if msg == nil {
		return false, nil
	}

	switch msg := msg.(type) {
	case *ClientEventMsg:
		if !msg.Valid() {
			return false, nil
		}
		ok, err := msg.Event.Verify()
		if err != nil {
			return false, fmt.Errorf("failed to verify event: %w", err)
		}
		return ok, nil

	case *ClientReqMsg:
		return msg.Valid(), nil

	case *ClientCloseMsg:
		return msg.Valid(), nil

	case *ClientAuthMsg:
		return msg.Valid(), nil

	case *ClientCountMsg:
		return msg.Valid(), nil

	default:
		return false, nil
	}
}

var _ ClientMsg = (*ClientUnknownMsg)(nil)

type ClientUnknownMsg struct {
	Label string
	Msg   []interface{}
}

func (*ClientUnknownMsg) ClientMsg() {}

func (msg *ClientUnknownMsg) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}

	var elems []any
	if err := json.Unmarshal(b, &elems); err != nil {
		return fmt.Errorf("not a json array: %w", err)
	}
	if len(elems) == 0 {
		return fmt.Errorf("empty json array")
	}

	label, ok := elems[0].(string)
	if !ok {
		return fmt.Errorf("label is not a string")
	}

	msg.Label = label
	msg.Msg = elems[1:]

	return nil
}

func (msg *ClientUnknownMsg) Valid() bool {
	return msg != nil && msg.Msg != nil
}

var _ ClientMsg = (*ClientEventMsg)(nil)

type ClientEventMsg struct {
	Event *Event
}

func (*ClientEventMsg) ClientMsg() {}

func (msg *ClientEventMsg) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}

	var elems []json.RawMessage
	if err := json.Unmarshal(b, &elems); err != nil {
		return fmt.Errorf("not a json array: %w", err)
	}
	if len(elems) != 2 {
		return fmt.Errorf("client event msg length must be 3 but got %d", len(elems))
	}

	var label string
	if err := json.Unmarshal(elems[0], &label); err != nil {
		return fmt.Errorf("label must be string: %w", err)
	}
	if label != "EVENT" {
		return fmt.Errorf(`client event msg label is must be "EVENT" but got %q`, label)
	}

	var event Event
	if err := json.Unmarshal(elems[1], &event); err != nil {
		return fmt.Errorf("failed to unmarshal event json: %w", err)
	}

	msg.Event = &event

	return nil
}

func (msg *ClientEventMsg) Valid() bool {
	return msg != nil && msg.Event.Valid()
}

var _ ClientMsg = (*ClientReqMsg)(nil)

type ClientReqMsg struct {
	SubscriptionID string
	ReqFilters     []*ReqFilter
}

func (*ClientReqMsg) ClientMsg() {}

func (msg *ClientReqMsg) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}

	var elems []json.RawMessage
	if err := json.Unmarshal(b, &elems); err != nil {
		return fmt.Errorf("not a json array: %w", err)
	}
	if len(elems) < 3 {
		return fmt.Errorf("client req msg length must be 3 or more but got %d", len(elems))
	}

	var label string
	if err := json.Unmarshal(elems[0], &label); err != nil {
		return fmt.Errorf("label is not a json string: %w", err)
	}
	if label != "REQ" {
		return fmt.Errorf(`client req msg labes must be "REQ" but got %q`, label)
	}

	var ret ClientReqMsg

	if err := json.Unmarshal(elems[1], &ret.SubscriptionID); err != nil {
		return fmt.Errorf("subscription id is not a json string: %w", err)
	}

	ret.ReqFilters = make([]*ReqFilter, len(elems)-2)
	for i := 0; i < len(elems)-2; i++ {
		if err := json.Unmarshal(elems[i+2], &ret.ReqFilters[i]); err != nil {
			return fmt.Errorf("failed to unmarshal filter: %w", err)
		}
	}

	*msg = ret

	return nil
}

func (msg *ClientReqMsg) Valid() (ok bool) {
	if msg == nil {
		return
	}

	if len(msg.ReqFilters) == 0 {
		return
	}

	if !sliceAllFunc(msg.ReqFilters, func(f *ReqFilter) bool { return f.Valid() }) {
		return
	}

	ok = true
	return
}

var _ ClientMsg = (*ClientCloseMsg)(nil)

type ClientCloseMsg struct {
	SubscriptionID string
}

func (*ClientCloseMsg) ClientMsg() {}

func (msg *ClientCloseMsg) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}

	var elems []string
	if err := json.Unmarshal(b, &elems); err != nil {
		return fmt.Errorf("not a json array: %w", err)
	}
	if len(elems) != 2 {
		return fmt.Errorf("client close msg length must be 2 but got %d", len(elems))
	}

	if elems[0] != "CLOSE" {
		return fmt.Errorf(`client close msg label must be "CLOSE" but got %q`, elems[0])
	}

	msg.SubscriptionID = elems[1]

	return nil
}

func (msg *ClientCloseMsg) Valid() bool { return msg != nil }

var _ ClientMsg = (*ClientAuthMsg)(nil)

type ClientAuthMsg struct {
	Challenge string
}

func (*ClientAuthMsg) ClientMsg() {}

func (msg *ClientAuthMsg) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}

	var elems []string
	if err := json.Unmarshal(b, &elems); err != nil {
		return fmt.Errorf("not a json array: %w", err)
	}
	if len(elems) != 2 {
		return fmt.Errorf("client auth msg length must be 2 but got %d", len(elems))
	}

	if elems[0] != "AUTH" {
		return fmt.Errorf(`client auth msg label must be "AUTH" but got %q`, elems[0])
	}

	msg.Challenge = elems[1]

	return nil
}

func (msg *ClientAuthMsg) Valid() bool { return msg != nil }

var _ ClientMsg = (*ClientCountMsg)(nil)

type ClientCountMsg struct {
	SubscriptionID string
	ReqFilters     []*ReqFilter
}

func (*ClientCountMsg) ClientMsg() {}

func (msg *ClientCountMsg) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}

	var elems []json.RawMessage
	if err := json.Unmarshal(b, &elems); err != nil {
		return fmt.Errorf("not a json array: %w", err)
	}
	if len(elems) < 3 {
		return fmt.Errorf("client count msg length must be 3 or more but got %d", len(elems))
	}

	var label string
	if err := json.Unmarshal(elems[0], &label); err != nil {
		return fmt.Errorf("label is not a json string: %w", err)
	}
	if label != "COUNT" {
		return fmt.Errorf(`client count msg labes must be "COUNT" but got %q`, label)
	}

	var ret ClientCountMsg

	if err := json.Unmarshal(elems[1], &ret.SubscriptionID); err != nil {
		return fmt.Errorf("subscription id is not a json string: %w", err)
	}

	ret.ReqFilters = make([]*ReqFilter, len(elems)-2)
	for i := 0; i < len(elems)-2; i++ {
		if err := json.Unmarshal(elems[i+2], &ret.ReqFilters[i]); err != nil {
			return fmt.Errorf("failed to unmarshal filter: %w", err)
		}
	}

	*msg = ret

	return nil
}

func (msg *ClientCountMsg) Valid() (ok bool) {
	if msg == nil {
		return
	}

	if len(msg.ReqFilters) == 0 {
		return
	}

	if !sliceAllFunc(msg.ReqFilters, func(f *ReqFilter) bool { return f.Valid() }) {
		return
	}

	ok = true
	return
}

type ReqFilter struct {
	IDs     []string
	Authors []string
	Kinds   []int64
	Tags    map[string][]string
	Since   *int64
	Until   *int64
	Limit   *int64
}

func (fil *ReqFilter) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}

	dec := json.NewDecoder(bytes.NewBuffer(b))
	dec.UseNumber()

	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return fmt.Errorf("not a json object: %w", err)
	}

	var ret ReqFilter

	for k, v := range obj {
		switch {
		case k == "ids":
			sli, ok := v.([]any)
			if !ok {
				return errors.New("ids is not a json array")
			}
			ret.IDs, ok = anySliceAs[string](sli)
			if !ok {
				return errors.New("ids is not a string json array")
			}

		case k == "authors":
			sli, ok := v.([]any)
			if !ok {
				return errors.New("authors is not a json array")
			}
			ret.Authors, ok = anySliceAs[string](sli)
			if !ok {
				return errors.New("authors is not a string json array")
			}

		case k == "kinds":
			sli, ok := v.([]any)
			if !ok {
				return errors.New("kinds is not a json array")
			}
			numKinds, ok := anySliceAs[json.Number](sli)
			if !ok {
				return errors.New("kinds is not a number array")
			}

			kinds := make([]int64, len(numKinds))
			for i, num := range numKinds {
				kind, err := num.Int64()
				if err != nil {
					return fmt.Errorf("kind is not integer: %w", err)
				}
				kinds[i] = kind
			}
			ret.Kinds = kinds

		case len(k) == 2 && k[0] == '#' && ('A' <= k[1] && k[1] <= 'Z' || 'a' <= k[1] && k[1] <= 'z'):
			// tags
			if ret.Tags == nil {
				ret.Tags = make(map[string][]string)
			}

			sli, ok := v.([]any)
			if !ok {
				return fmt.Errorf("%s is not a json array", k)
			}
			vs, ok := anySliceAs[string](sli)
			if !ok {
				return fmt.Errorf("%s is not a string json array", k)
			}
			ret.Tags[k] = vs

		case k == "since":
			numSince, ok := v.(json.Number)
			if !ok {
				return errors.New("since is not a json number")
			}
			since, err := numSince.Int64()
			if err != nil {
				return fmt.Errorf("since is not integer: %w", err)
			}
			ret.Since = toPtr(since)

		case k == "until":
			numUntil, ok := v.(json.Number)
			if !ok {
				return errors.New("until is not a json number")
			}
			until, err := numUntil.Int64()
			if err != nil {
				return fmt.Errorf("until is not integer: %w", err)
			}
			ret.Until = toPtr(until)

		case k == "limit":
			numLimit, ok := v.(json.Number)
			if !ok {
				return errors.New("limit is not a json number")
			}
			limit, err := numLimit.Int64()
			if err != nil {
				return fmt.Errorf("limit is not integer: %w", err)
			}
			ret.Limit = toPtr(limit)

		default:
			return fmt.Errorf("contains invalid member: (%s, %v)", k, v)
		}
	}

	*fil = ret

	return nil
}

func (fil *ReqFilter) Valid() (ok bool) {
	if fil == nil {
		return
	}

	if fil.IDs != nil {
		if !sliceAllFunc(fil.IDs, validID) {
			return
		}
	}

	if fil.Authors != nil {
		if !sliceAllFunc(fil.Authors, validPubkey) {
			return
		}
	}

	if fil.Kinds != nil {
		if !sliceAllFunc(fil.Kinds, validKind) {
			return
		}
	}

	if fil.Tags != nil {
		for tag, vals := range fil.Tags {
			if len(tag) != 2 || tag[0] != '#' ||
				!('A' <= tag[1] && tag[1] <= 'Z' || 'a' <= tag[1] && tag[1] <= 'z') {
				return
			}
			if vals == nil {
				return
			}

			switch tag {
			case "#e":
				if !sliceAllFunc(vals, validID) {
					return
				}

			case "#p":
				if !sliceAllFunc(vals, validPubkey) {
					return
				}

			case "#a":
				if !sliceAllFunc(vals, validNaddr) {
					return
				}
			}
		}
	}

	if fil.Since != nil {
		if *fil.Since < 0 {
			return
		}
	}

	if fil.Until != nil {
		if *fil.Until < 0 {
			return
		}
	}

	if fil.Since != nil && fil.Until != nil {
		if *fil.Since > *fil.Until {
			return
		}
	}

	if fil.Limit != nil {
		if *fil.Limit < 0 {
			return
		}
	}

	ok = true
	return
}

type ServerMsg interface {
	ServerMsg()
	MarshalJSON() ([]byte, error)
}

func IsNilServerMsg(msg ServerMsg) bool {
	return msg == nil || reflect.ValueOf(msg).IsNil()
}

type ServerEOSEMsg struct {
	SubscriptionID string
}

func NewServerEOSEMsg(subID string) *ServerEOSEMsg {
	return &ServerEOSEMsg{
		SubscriptionID: subID,
	}
}

func (*ServerEOSEMsg) ServerMsg() {}

var ErrMarshalServerEOSEMsg = errors.New("failed to marshal server eose msg")

func (msg *ServerEOSEMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerEOSEMsg
	}

	v := [2]string{"EOSE", msg.SubscriptionID}
	ret, err := json.Marshal(&v)
	if err != nil {
		return nil, errors.Join(err, ErrMarshalServerEOSEMsg)
	}

	return ret, nil
}

type ServerEventMsg struct {
	SubscriptionID string
	Event          *Event
}

func NewServerEventMsg(subID string, event *Event) *ServerEventMsg {
	ret := &ServerEventMsg{
		SubscriptionID: subID,
		Event:          event,
	}
	return ret
}

func (*ServerEventMsg) ServerMsg() {}

var ErrMarshalServerEventMsg = errors.New("failed to marshal server event msg")

func (msg *ServerEventMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerEventMsg
	}

	v := [3]interface{}{"EVENT", msg.SubscriptionID, msg.Event}
	ret, err := json.Marshal(&v)
	if err != nil {
		return nil, errors.Join(err, ErrMarshalServerEventMsg)
	}

	return ret, nil
}

type ServerNoticeMsg struct {
	Message string
}

func NewServerNoticeMsg(message string) *ServerNoticeMsg {
	return &ServerNoticeMsg{
		Message: message,
	}
}

func NewServerNoticeMsgf(format string, a ...any) *ServerNoticeMsg {
	return &ServerNoticeMsg{
		Message: fmt.Sprintf(format, a...),
	}
}

func (*ServerNoticeMsg) ServerMsg() {}

var ErrMarshalServerNoticeMsg = errors.New("failed to marshal server notice msg")

func (msg *ServerNoticeMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerNoticeMsg
	}

	v := [2]string{"NOTICE", msg.Message}
	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerNoticeMsg)
	}

	return ret, err
}

type ServerOKMsg struct {
	EventID   string
	Accepted  bool
	Msg       string
	MsgPrefix string
}

const (
	ServerOKMsgPrefixNoPrefix    = ""
	ServerOKMsgPrefixPoW         = "pow: "
	ServerOKMsgPrefixDuplicate   = "duplicate: "
	ServerOkMsgPrefixBlocked     = "blocked: "
	ServerOkMsgPrefixRateLimited = "rate-limited: "
	ServerOkMsgPrefixRateInvalid = "invalid: "
	ServerOkMsgPrefixError       = "error: "
)

func NewServerOKMsg(eventID string, accepted bool, prefix, msg string) *ServerOKMsg {
	return &ServerOKMsg{
		EventID:   eventID,
		Accepted:  accepted,
		MsgPrefix: prefix,
		Msg:       msg,
	}
}

func (*ServerOKMsg) ServerMsg() {}

func (msg *ServerOKMsg) Message() string {
	return msg.MsgPrefix + msg.Msg
}

var ErrMarshalServerOKMsg = errors.New("failed to marshal server ok msg")

func (msg *ServerOKMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerOKMsg
	}

	v := [4]interface{}{"OK", msg.EventID, msg.Accepted, msg.Message()}
	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerOKMsg)
	}

	return ret, err
}

type ServerAuthMsg struct {
	Event *Event
}

var ErrServerAuthMsgNilEvent = errors.New("server auth msg event must be non nil value")

func NewServerAuthMsg(event *Event) (*ServerAuthMsg, error) {
	if event == nil {
		return nil, ErrServerAuthMsgNilEvent
	}

	return &ServerAuthMsg{Event: event}, nil
}

func (*ServerAuthMsg) ServerMsg() {}

var ErrMarshalServerAuthMsg = errors.New("failed to marshal server auth msg")

func (msg *ServerAuthMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerAuthMsg
	}

	v := [2]interface{}{"AUTH", msg.Event}
	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerAuthMsg)
	}

	return ret, err
}

type ServerCountMsg struct {
	SubscriptionID string
	Count          uint64
	Approximate    *bool
}

func NewServerCountMsg(subID string, count uint64, approx *bool) *ServerCountMsg {
	return &ServerCountMsg{
		SubscriptionID: subID,
		Count:          count,
		Approximate:    approx,
	}
}

func (*ServerCountMsg) ServerMsg() {}

var ErrMarshalServerCountMsg = errors.New("failed to marshal server count msg")

func (msg *ServerCountMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerCountMsg
	}

	type payload struct {
		Count       uint64 `json:"count"`
		Approximate *bool  `json:"approximate,omitempty"`
	}

	v := [3]interface{}{
		"COUNT",
		msg.SubscriptionID,
		payload{Count: msg.Count, Approximate: msg.Approximate},
	}
	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerCountMsg)
	}

	return ret, err
}

type EventType int

const (
	EventTypeUnknown EventType = iota
	EventTypeRegular
	EventTypeReplaceable
	EventTypeEphemeral
	EventTypeParamReplaceable
)

type Event struct {
	ID        string `json:"id"`
	Pubkey    string `json:"pubkey"`
	CreatedAt int64  `json:"created_at"`
	Kind      int64  `json:"kind"`
	Tags      []Tag  `json:"tags"`
	Content   string `json:"content"`
	Sig       string `json:"sig"`
}

var ErrMarshalEvent = errors.New("failed to marshal event")

func (ev *Event) MarshalJSON() ([]byte, error) {
	if ev == nil {
		return nil, ErrMarshalEvent
	}

	type alias Event
	ret, err := json.Marshal(alias(*ev))
	if err != nil {
		err = errors.Join(err, ErrMarshalEvent)
	}
	return ret, err
}

func (ev *Event) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewBuffer(b))
	dec.UseNumber()

	var obj map[string]interface{}
	if err := dec.Decode(&obj); err != nil {
		return fmt.Errorf("not a json object: %w", err)
	}
	if len(obj) != 7 {
		return errors.New("contains some extra fields")
	}

	var ret Event
	var tmp any
	var tmpnum json.Number
	var ok bool
	var err error

	// id
	tmp, ok = obj["id"]
	if !ok {
		return errors.New("id not found")
	}
	ret.ID, ok = tmp.(string)
	if !ok {
		return errors.New("id is not a json string")
	}

	// pubkey
	tmp, ok = obj["pubkey"]
	if !ok {
		return errors.New("pubkey not found")
	}
	ret.Pubkey, ok = tmp.(string)
	if !ok {
		return errors.New("pubkey is not a json string")
	}

	// Created_at
	tmp, ok = obj["created_at"]
	if !ok {
		return errors.New("created_at not found")
	}
	tmpnum, ok = tmp.(json.Number)
	if !ok {
		return errors.New("created_at is not a json number")
	}
	ret.CreatedAt, err = tmpnum.Int64()
	if err != nil {
		return fmt.Errorf("created_at is not an integer: %w", err)
	}

	// kind
	tmp, ok = obj["kind"]
	if !ok {
		return errors.New("kind not found")
	}
	tmpnum, ok = tmp.(json.Number)
	if !ok {
		return errors.New("kind is not a json number")
	}
	ret.Kind, err = tmpnum.Int64()
	if err != nil {
		return fmt.Errorf("kind is not an integer: %w", err)
	}

	// tags
	tmp, ok = obj["tags"]
	if !ok {
		return errors.New("tags not found")
	}
	tmpSli, ok := tmp.([]any)
	if !ok {
		return errors.New("tags is not a json array")
	}
	slisli, ok := anySliceAs[[]any](tmpSli)
	if !ok {
		return errors.New("tags is not a array of json array")
	}
	ret.Tags = make([]Tag, len(slisli))
	for i, sli := range slisli {
		ret.Tags[i], ok = anySliceAs[string](sli)
		if !ok {
			return errors.New("tags is not string arrays of json array")
		}
	}

	// content
	tmp, ok = obj["content"]
	if !ok {
		return errors.New("content not found")
	}
	ret.Content, ok = tmp.(string)
	if !ok {
		return errors.New("content is not a json string")
	}

	// sig
	tmp, ok = obj["sig"]
	if !ok {
		return errors.New("sig not found")
	}
	ret.Sig, ok = tmp.(string)
	if !ok {
		return errors.New("sig is not a json string")
	}

	*ev = ret

	return nil
}

func (ev *Event) EventType() EventType {
	if kind := ev.Kind; kind == 0 || kind == 3 || 10000 <= kind && kind < 20000 {
		return EventTypeReplaceable
	} else if 20000 <= kind && kind < 30000 {
		return EventTypeEphemeral
	} else if 30000 <= kind && kind < 40000 {
		return EventTypeParamReplaceable
	}
	return EventTypeRegular
}

func (ev *Event) Valid() bool {
	return ev != nil &&
		validID(ev.ID) &&
		validPubkey(ev.Pubkey) &&
		validKind(ev.Kind) &&
		ev.Tags != nil &&
		sliceAllFunc(ev.Tags, validTag) &&
		validSig(ev.Sig)
}

var ErrEventSerialize = errors.New("failed to serialize event")

func (ev *Event) Serialize() ([]byte, error) {
	if ev == nil {
		return nil, fmt.Errorf("empty event: %w", ErrEventSerialize)
	}

	v := [6]interface{}{
		0,
		ev.Pubkey,
		ev.CreatedAt,
		ev.Kind,
		ev.Tags,
		ev.Content,
	}

	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrEventSerialize)
	}
	return ret, err
}

func (ev *Event) Verify() (bool, error) {
	if ev == nil {
		return false, errors.New("nil event")
	}

	// Verify ID
	serialized, err := ev.Serialize()
	if err != nil {
		return false, err
	}

	idBin, err := hex.DecodeString(ev.ID)
	if err != nil {
		return false, fmt.Errorf("failed to decode id: %w", err)
	}

	hash := sha256.Sum256(serialized)

	if !bytes.Equal(idBin, hash[:]) {
		return false, nil
	}

	// Verify Sig
	pubkeyBin, err := hex.DecodeString(ev.Pubkey)
	if err != nil {
		return false, fmt.Errorf("failed to decode pubkey: %w", err)
	}

	pubkey, err := schnorr.ParsePubKey(pubkeyBin)
	if err != nil {
		return false, fmt.Errorf("failed to parse pubkey: %w", err)
	}

	sigBin, err := hex.DecodeString(ev.Sig)
	if err != nil {
		return false, fmt.Errorf("failed to decode sig: %w", err)
	}

	sig, err := schnorr.ParseSignature(sigBin)
	if err != nil {
		return false, fmt.Errorf("failed to parse sig: %w", err)
	}

	return sig.Verify(idBin, pubkey), nil
}

func (ev *Event) CreatedAtTime() time.Time {
	if ev == nil {
		return time.Unix(0, 0)
	}
	return time.Unix(ev.CreatedAt, 0)
}

type Tag []string

type EventInvalidIDError struct {
	Correct, Actual string
}

func (e *EventInvalidIDError) Error() string {
	return fmt.Sprintf("correct event id is %q but %q", e.Correct, e.Actual)
}

type EventInvalidSigError struct {
	Correct, Actual string
}

func (e *EventInvalidSigError) Error() string {
	return fmt.Sprintf("correct event sig is %q but %q", e.Correct, e.Actual)
}

func validID(id string) bool { return len(id) == 64 && validHexString(id) }

func validPubkey(pubkey string) bool { return len(pubkey) == 64 && validHexString(pubkey) }

func validKind(kind int64) bool { return 0 <= kind || kind <= 65535 }

func validTag(tag Tag) bool { return len(tag) >= 1 && tag[0] != "" }

func validNaddr(naddr string) (ok bool) {
	elems := strings.Split(naddr, ":")
	if len(elems) != 3 {
		return
	}

	kind, err := strconv.ParseInt(elems[0], 10, 64)
	if err != nil {
		return
	}
	if !validKind(kind) {
		return
	}

	if !validPubkey(elems[1]) {
		return
	}

	ok = true
	return
}

func validSig(sig string) bool { return len(sig) == 128 && validHexString(sig) }
