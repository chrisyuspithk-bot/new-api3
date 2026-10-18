package openai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wsPair returns the server-side conn (what the handler holds) and the peer.
func wsPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	up := websocket.Upgrader{}
	ch := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		require.NoError(t, err)
		ch <- c
	}))
	t.Cleanup(srv.Close)
	peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = peer.Close() })
	return <-ch, peer
}

func responseDone(total int) string {
	return `{"type":"response.done","response":{"usage":{"total_tokens":` + itoa(total) + `,"input_tokens":` + itoa(total) + `,"output_tokens":0,"input_token_details":{"text_tokens":` + itoa(total) + `},"output_token_details":{}}}}`
}

func itoa(v int) string {
	b := []byte{}
	if v == 0 {
		return "0"
	}
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

// A chunk whose charge fails must be counted in the final total exactly once
// (not re-added at the end), so the final settle bills the true usage.
func TestRealtimeFailedChunkCountedOnceInFinalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var mu sync.Mutex
	var charged []int
	orig := preWssConsume
	t.Cleanup(func() { preWssConsume = orig })
	preWssConsume = func(_ *gin.Context, _ *relaycommon.RelayInfo, u *dto.RealtimeUsage) error {
		mu.Lock()
		defer mu.Unlock()
		charged = append(charged, u.TotalTokens)
		if u.TotalTokens == 40 {
			return errors.New("reseller quota insufficient")
		}
		return nil
	}

	clientSrv, clientPeer := wsPair(t)
	targetSrv, targetPeer := wsPair(t)
	_ = clientPeer
	info := &relaycommon.RelayInfo{ClientWs: clientSrv, TargetWs: targetSrv}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)

	go func() {
		_ = targetPeer.WriteMessage(websocket.TextMessage, []byte(responseDone(30)))
		time.Sleep(50 * time.Millisecond)
		_ = targetPeer.WriteMessage(websocket.TextMessage, []byte(responseDone(40)))
	}()
	done := make(chan *dto.RealtimeUsage, 1)
	go func() {
		_, u := OpenaiRealtimeHandler(c, info)
		done <- u
	}()
	var sum *dto.RealtimeUsage
	select {
	case sum = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not finish")
	}
	assert.Equal(t, 70, sum.TotalTokens, "failed chunk counted once: 30 + 40, not 110")
	mu.Lock()
	assert.Equal(t, []int{30, 40}, charged, "each chunk attempted once; failed chunk not re-charged at close")
	mu.Unlock()
}
