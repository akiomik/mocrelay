package nostr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServerEOSEMsg_MarshalJSON(t *testing.T) {
	type Expect struct {
		Json []byte
		Err  error
	}

	tests := []struct {
		Name   string
		Input  *ServerEOSEMsg
		Expect Expect
	}{
		{
			Name: "ok: server eose message",
			Input: &ServerEOSEMsg{
				SubscriptionID: "sub_id",
			},
			Expect: Expect{
				Json: []byte(`["EOSE","sub_id"]`),
				Err:  nil,
			},
		},
		{
			Name:  "ng: nil",
			Input: nil,
			Expect: Expect{
				Err: ErrMarshalServerEOSEMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := tt.Input.MarshalJSON()
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			assert.Equal(t, tt.Expect.Json, got)
		})
	}
}

func TestServerEventMsg_MarshalJSON(t *testing.T) {
	type Expect struct {
		Json []byte
		Err  error
	}

	tests := []struct {
		Name   string
		Input  *ServerEventMsg
		Expect Expect
	}{
		{
			Name: "ok: server event message",
			Input: &ServerEventMsg{
				SubscriptionID: "sub_id",
				Event: &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{{
						"e",
						"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						"",
						"root",
					}, {
						"p",
						"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
				},
			},
			Expect: Expect{
				Json: []byte(`["EVENT","sub_id",` +
					`{` +
					`"id":"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
					`"pubkey":"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
					`"created_at":1693157791,` +
					`"kind":1,` +
					`"tags":[` +
					`[` +
					`"e",` +
					`"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
					`"",` +
					`"root"` +
					`],` +
					`[` +
					`"p",` +
					`"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
					`]` +
					`],` +
					`"content":"powa",` +
					`"sig":"795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
					`}]`),
				Err: nil,
			},
		},
		{
			Name: "ok: server event message (raw)",
			Input: &ServerEventMsg{
				SubscriptionID: "sub_id",
				Event: &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{{
						"e",
						"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						"",
						"root",
					}, {
						"p",
						"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
					raw: []byte(`{` +
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
						`  "sig": "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",` +
						`  "powa":"meu"` +
						`}`),
				},
			},
			Expect: Expect{
				Json: []byte(`["EVENT","sub_id",` +
					`{` +
					`"kind":1,` +
					`"pubkey":"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
					`"created_at":1693157791,` +
					`"tags":[` +
					`[` +
					`"e",` +
					`"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
					`"",` +
					`"root"` +
					`],` +
					`[` +
					`"p",` +
					`"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
					`]` +
					`],` +
					`"content":"powa",` +
					`"id":"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
					`"sig":"795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",` +
					`"powa":"meu"` +
					`}]`),
				Err: nil,
			},
		},
		{
			Name:  "ng: nil",
			Input: nil,
			Expect: Expect{
				Err: ErrMarshalServerEventMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := tt.Input.MarshalJSON()
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			assert.Equal(t, tt.Expect.Json, got)
		})
	}
}
