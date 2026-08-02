package server

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestHub_BroadcastDuringDisconnect_NoPanic reproduces the send-on-closed-
// channel crash. One goroutine repeatedly broadcasts to the session while
// participants disconnect concurrently — the everyday "someone leaves while
// someone else swipes" interleaving.
//
// Before the fix, readPump's defer closed c.send. A broadcast that had already
// snapshotted a client under the lock would then run c.send <- data on that
// just-closed channel and panic; the panic surfaces in the broadcaster
// goroutine (no recover) and crashes the whole process, failing this test.
// After the fix, send is never closed (readPump closes done instead), so the
// broadcast is always safe. Run under -race to also catch the close/send data
// race.
func TestHub_BroadcastDuringDisconnect_NoPanic(t *testing.T) {
	srv := New(Config{SessionTTL: "1h"})
	session := srv.store.Create("Host")

	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/ws?code=" + session.Code

	const nClients = 8
	conns := make([]*websocket.Conn, 0, nClients)

	var readers sync.WaitGroup
	// Each connection needs a dedicated reader goroutine draining writePump's
	// output; ReadMessage also returns the error that unblocks the drain when
	// the connection is closed.
	drain := func(c *websocket.Conn) {
		defer readers.Done()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}

	for i := 0; i < nClients; i++ {
		conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if resp != nil {
			resp.Body.Close()
		}
		if err != nil {
			t.Fatalf("dial client %d: %v", i, err)
		}
		conns = append(conns, conn)
		readers.Add(1)
		go drain(conn)

		joinData, err := newEnvelope("join", JoinPayload{Name: fmt.Sprintf("P%d", i)})
		if err != nil {
			t.Fatalf("build join %d: %v", i, err)
		}
		if err := conn.WriteMessage(websocket.TextMessage, joinData); err != nil {
			t.Fatalf("send join %d: %v", i, err)
		}
	}

	// Wait until every client is registered in session.clients before we start
	// hammering broadcasts, so the only writes to c.participantID (in
	// handleJoin) have all completed first.
	deadline := time.Now().Add(2 * time.Second)
	for {
		session.mu.Lock()
		n := len(session.clients)
		session.mu.Unlock()
		if n == nClients {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d/%d clients registered before timeout", n, nClients)
		}
		time.Sleep(time.Millisecond)
	}

	// Spin a broadcaster that runs continuously across the disconnects.
	stop := make(chan struct{})
	var bcast sync.WaitGroup
	bcast.Add(1)
	go func() {
		defer bcast.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			session.broadcast("progress", ProgressPayload{CardsRemaining: 1})
			session.broadcastSessionState()
		}
	}()

	// Let broadcasts get going, then drop half the participants mid-flight.
	time.Sleep(10 * time.Millisecond)
	for i := 0; i < nClients; i += 2 {
		conns[i].Close()
	}
	// Keep broadcasting past the disconnects to widen the race window.
	time.Sleep(50 * time.Millisecond)

	close(stop)
	bcast.Wait()

	// Clean up the survivors.
	for i := 1; i < nClients; i += 2 {
		conns[i].Close()
	}
	readers.Wait()
}
