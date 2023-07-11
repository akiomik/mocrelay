package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/tomasen/realip"
	"golang.org/x/time/rate"
)

const SenderLen = 3

const (
	RateLimitRate  = 20
	RateLimitBurst = 10
	MaxFilterLen   = 50
)

type Relay struct{}

func (relay *Relay) HandleWebsocket(ctx context.Context, req *http.Request, connID string, conn net.Conn, router *Router, cache *Cache) error {
	defer func() {
		if err := recover(); err != nil {
			logStderr.Printf("[%v, %v]: paniced: %v", realip.FromRequest(req), connID, err)
			panic(err)
		}
	}()

	promActiveWebsocket.WithLabelValues(realip.FromRequest(req), connID).Inc()
	defer promActiveWebsocket.WithLabelValues(realip.FromRequest(req), connID).Dec()

	defer router.Delete(connID)

	sender := make(chan ServerMsg, SenderLen)

	errCh := make(chan error, 2)

	wg := new(sync.WaitGroup)
	defer wg.Wait()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errCh <- relay.wsSender(ctx, req, connID, conn, router, sender)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errCh <- relay.wsReceiver(ctx, req, connID, conn, router, cache, sender)
	}()

	err := <-errCh

	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, syscall.ECONNRESET) {
			return nil
		}

		return fmt.Errorf("handle websocket error: %w", err)
	}

	return nil
}

func (relay *Relay) wsReceiver(
	ctx context.Context,
	req *http.Request,
	connID string,
	conn net.Conn,
	router *Router,
	cache *Cache,
	sender chan<- ServerMsg,
) error {
	lim := rate.NewLimiter(RateLimitRate, RateLimitBurst)
	reader := wsutil.NewServerSideReader(conn)

	for {
		if err := lim.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter returns error: %w", err)
		}

		payload, err := relay.wsRead(reader)
		if err != nil {
			return fmt.Errorf("receive error: %w", err)
		}

		if !utf8.Valid(payload) {
			logStderr.Printf("[%v, %v]: payload is not utf8: %v", realip.FromRequest(req), connID, payload)
			continue
		}

		strMsg := string(payload)
		jsonMsg, err := ParseClientMsgJSON(strMsg)
		if err != nil {
			logStderr.Printf("[%v, %v]: received invalid msg: %v", realip.FromRequest(req), connID, err)
			continue
		}

		DoAccessLog(realip.FromRequest(req), connID, AccessLogRecv, strMsg)
		promWSRecvCounter.WithLabelValues(realip.FromRequest(req), connID, jsonMsg).Inc()

		switch msg := jsonMsg.(type) {
		case *ClientReqMsgJSON:
			if err := relay.serveClientReqMsgJSON(connID, router, cache, sender, msg); err != nil {
				logStderr.Printf("[%v, %v]: failed to serve client req msg %v", realip.FromRequest(req), connID, err)
				continue
			}

		case *ClientCloseMsgJSON:
			if err := relay.serveClientCloseMsgJSON(connID, router, msg); err != nil {
				logStderr.Printf("[%v, %v]: failed to serve client close msg %v", realip.FromRequest(req), connID, err)
				continue
			}

		case *ClientEventMsgJSON:
			if err := relay.serveClientEventMsgJSON(router, cache, msg); err != nil {
				logStderr.Printf("[%v, %v]: failed to serve client event msg %v", realip.FromRequest(req), connID, err)
				continue
			}
		}
	}
}

func (*Relay) wsRead(wsr *wsutil.Reader) ([]byte, error) {
	limit := *MaxClientMesLen + 1

	hdr, err := wsr.NextFrame()
	if err != nil {
		return nil, fmt.Errorf("failed to get next frame: %w", err)
	}
	if hdr.OpCode == ws.OpClose {
		return nil, io.EOF
	}

	r := io.LimitReader(wsr, int64(limit))
	res, err := io.ReadAll(r)
	if len(res) == limit {
		return res, fmt.Errorf("websocket message is too long (len=%v): %s", len(res), res)
	}
	return res, err
}

func (*Relay) serveClientReqMsgJSON(
	connID string,
	router *Router,
	cache *Cache,
	sender chan<- ServerMsg,
	msg *ClientReqMsgJSON,
) error {
	filters := NewFiltersFromFilterJSONs(msg.FilterJSONs)

	if len(filters) > MaxFilterLen+2 {
		return fmt.Errorf("filter is too long: %v", msg)
	}

	for _, event := range cache.FindAll(filters) {
		sender <- &ServerEventMsg{msg.SubscriptionID, event.EventJSON}
	}
	sender <- &ServerEOSEMsg{msg.SubscriptionID}

	// TODO(high-moctane) handle error, impl is not good
	if err := router.Subscribe(connID, msg.SubscriptionID, filters, sender); err != nil {
		return nil
	}
	return nil
}

func (*Relay) serveClientCloseMsgJSON(connID string, router *Router, msg *ClientCloseMsgJSON) error {
	if err := router.Close(connID, msg.SubscriptionID); err != nil {
		return fmt.Errorf("cannot close conn %v", msg.SubscriptionID)
	}
	return nil
}

func (*Relay) serveClientEventMsgJSON(router *Router, cache *Cache, msg *ClientEventMsgJSON) error {
	ok, err := msg.EventJSON.Verify()
	if err != nil {
		return fmt.Errorf("failed to verify event json: %v", msg)

	}
	if !ok {
		return fmt.Errorf("invalid signature: %v", msg)
	}

	promEventCounter.WithLabelValues(msg.EventJSON).Inc()

	event := &Event{msg.EventJSON, time.Now()}

	if !event.ValidCreatedAt() {
		return fmt.Errorf("invalid created_at: %v", event.CreatedAtToTime())
	}

	cache.Save(event)

	if err := router.Publish(event); err != nil {
		return fmt.Errorf("failed to publish event: %v", event)
	}
	return nil
}

func (*Relay) wsSender(
	ctx context.Context,
	req *http.Request,
	connID string,
	conn net.Conn,
	router *Router,
	sender <-chan ServerMsg,
) (err error) {
	defer func() {
		if _, e := conn.Write(ws.CompiledCloseNormalClosure); e != nil {
			if errors.Is(e, net.ErrClosed) {
				return
			}
			err = fmt.Errorf("failed to send close frame: %w", e)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil

		case msg := <-sender:
			promWSSendCounter.WithLabelValues(realip.FromRequest(req), connID, msg).Inc()

			jsonMsg, err := msg.MarshalJSON()
			if err != nil {
				logStderr.Printf("[%v, %v]: failed to marshal server msg: %v", realip.FromRequest(req), connID, msg)
			}

			if err := wsutil.WriteServerText(conn, jsonMsg); err != nil {
				if errors.Is(err, net.ErrClosed) {
					return nil
				}
				return fmt.Errorf("failed to write server text: %w", err)
			}

			DoAccessLog(realip.FromRequest(req), connID, AccessLogSend, string(jsonMsg))
		}
	}
}
