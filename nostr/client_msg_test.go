package nostr

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/high-moctane/mocrelay/utils"
)

func TestParseClientMsg(t *testing.T) {
	type Expect struct {
		MsgType ClientMsgType
		Err     error
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ng: invalid utf8",
			Input: []byte{'[', '"', 0xf0, 0x28, 0x8c, 0xbc, '"', ']'},
			Expect: Expect{
				MsgType: ClientMsgTypeUnknown,
				Err:     ErrInvalidClientMsg,
			},
		},
		{
			Name:  "ng: empty string",
			Input: []byte(""),
			Expect: Expect{
				MsgType: ClientMsgTypeUnknown,
				Err:     ErrInvalidClientMsg,
			},
		},
		{
			Name:  "ng: not a client message",
			Input: []byte(`["INVALID","value"]`),
			Expect: Expect{
				MsgType: ClientMsgTypeUnknown,
				Err:     ErrInvalidClientMsg,
			},
		},
		{
			Name:  "ok: client close message",
			Input: []byte(`["CLOSE","sub_id"]`),
			Expect: Expect{
				MsgType: ClientMsgTypeClose,
				Err:     nil,
			},
		},
		{
			Name:  "ok: client close message with some spaces",
			Input: []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
			Expect: Expect{
				MsgType: ClientMsgTypeClose,
				Err:     nil,
			},
		},
		{
			Name: "ok: client event message",
			Input: []byte(`["EVENT",` +
				`{` +
				`  "kind": 1,` +
				`  "pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
				`  "created_at": 1693157791,` +
				`  "tags": [` +
				`    [` +
				`      "e",` +
				`      "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
				`      "",` +
				`      "root"` +
				`    ],` +
				`    [` +
				`      "p",` +
				`      "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
				`    ]` +
				`  ],` +
				`  "content": "powa",` +
				`  "id": "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
				`  "sig": "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
				`}]`),
			Expect: Expect{
				MsgType: ClientMsgTypeEvent,
				Err:     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientMsg(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.Equal(t, tt.Expect.MsgType, msg.MsgType())
			assert.Equal(t, tt.Input, msg.Raw())
		})
	}
}

func TestParseClientCloseMsg(t *testing.T) {
	type Expect struct {
		SubscriptionID string
		Raw            []byte
		Err            error
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ok: client close message",
			Input: []byte(`["CLOSE","sub_id"]`),
			Expect: Expect{
				SubscriptionID: "sub_id",
				Raw:            []byte(`["CLOSE","sub_id"]`),
				Err:            nil,
			},
		},
		{
			Name:  "ok: client close message with some spaces",
			Input: []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
			Expect: Expect{
				SubscriptionID: "sub_id",
				Raw:            []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
				Err:            nil,
			},
		},
		{
			Name:  "ng: client close message invalid type",
			Input: []byte(`["CLOSE",3000]`),
			Expect: Expect{
				Err: ErrInvalidClientCloseMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientCloseMsg(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.Equal(t, tt.Expect.SubscriptionID, msg.SubscriptionID)
			assert.Equal(t, tt.Expect.Raw, msg.Raw())
		})
	}
}

func TestParseFilter(t *testing.T) {
	type Expect struct {
		Filter Filter
		Err    error
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ok: empty",
			Input: []byte("{}"),
			Expect: Expect{
				Filter: Filter{},
				Err:    nil,
			},
		},
		{
			Name:  "ok: full",
			Input: []byte(`{"ids":["powa"],"authors":["meu"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}`),
			Expect: Expect{
				Filter: Filter{
					IDs:     utils.ToRef([]string{"powa"}),
					Authors: utils.ToRef([]string{"meu"}),
					Kinds:   utils.ToRef([]int64{1, 3}),
					Tags: utils.ToRef(map[string][]string{
						"e": {"moyasu"},
					}),
					Since: utils.ToRef(int64(16)),
					Until: utils.ToRef(int64(184838)),
					Limit: utils.ToRef(int64(143)),
				},
				Err: nil,
			},
		},
		{
			Name:  "ok: partial",
			Input: []byte(`{"ids":["powa"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}`),
			Expect: Expect{
				Filter: Filter{
					IDs:   utils.ToRef([]string{"powa"}),
					Kinds: utils.ToRef([]int64{1, 3}),
					Tags: utils.ToRef(map[string][]string{
						"e": {"moyasu"},
					}),
					Since: utils.ToRef(int64(16)),
					Until: utils.ToRef(int64(184838)),
					Limit: utils.ToRef(int64(143)),
				},
				Err: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			fil, err := ParseFilter(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if fil == nil {
				t.Errorf("expect non-nil filter but got nil")
				return
			}
			assert.EqualExportedValues(t, tt.Expect.Filter, *fil)
			assert.Equal(t, tt.Input, fil.Raw())
		})
	}
}