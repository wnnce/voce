package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
)

func TestNewPoolConnection(t *testing.T) {
	t.Run("rejects a nil engine", func(t *testing.T) {
		conn, err := newPoolConnection(context.Background(), nil, "machine", "127.0.0.1:7001", nil, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrNilNBHTTPEngine)
	})

	t.Run("rejects an invalid address", func(t *testing.T) {
		conn, err := newPoolConnection(context.Background(), &nbhttp.Engine{}, "machine", "\x00", nil, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrInvalidPoolAddress)
	})

	t.Run("initializes a connecting connection", func(t *testing.T) {
		observer := &recordingConnectionObserver{}
		conn, err := newPoolConnection(context.Background(), &nbhttp.Engine{}, "machine", "127.0.0.1:7001", nil, observer)
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.Equal(t, protocol.ConnectionConnecting, conn.State())
		assert.Equal(t, "ws://127.0.0.1:7001/pool", conn.addr.String())
		assert.Same(t, observer, conn.observer)
	})
}

func TestConnectionNotifiesObserver(t *testing.T) {
	observer := &recordingConnectionObserver{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn := &Connection{ctx: ctx, observer: observer}
	conn.state.Store(int32(protocol.ConnectionConnecting))

	conn.OnOpen(nil)
	assert.Equal(t, []*Connection{conn}, observer.opened)

	conn.OnClose(nil, nil)
	assert.Equal(t, []*Connection{conn}, observer.closed)

	conn.Close()
	assert.Equal(t, []*Connection{conn, conn}, observer.closed)
	conn.Close()
	assert.Equal(t, []*Connection{conn, conn}, observer.closed)
}

func TestConnectionIgnoresRepeatedClose(t *testing.T) {
	observer := &recordingConnectionObserver{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn := &Connection{ctx: ctx, observer: observer}
	conn.state.Store(int32(protocol.ConnectionActive))

	conn.OnClose(nil, nil)
	assert.Equal(t, []*Connection{conn}, observer.closed)
	assert.Equal(t, protocol.ConnectionConnecting, conn.State())

	conn.OnClose(nil, nil)
	assert.Equal(t, []*Connection{conn}, observer.closed)

	conn.Close()
	assert.Equal(t, []*Connection{conn, conn}, observer.closed)
	assert.Equal(t, protocol.ConnectionClosed, conn.State())
}

func TestConnectionOnMessage(t *testing.T) {
	key := protocol.SessionKey{1}
	payload := []byte("message")
	data := make([]byte, protocol.SessionKeySize+len(payload))
	copy(data, key[:])
	copy(data[protocol.SessionKeySize:], payload)

	var receivedKey protocol.SessionKey
	var receivedPayload []byte
	conn := &Connection{dispatcher: func(key protocol.SessionKey, data []byte) {
		receivedKey = key
		receivedPayload = append([]byte(nil), data...)
	}}
	conn.OnMessage(nil, websocket.BinaryMessage, data)
	assert.Equal(t, key, receivedKey)
	assert.Equal(t, payload, receivedPayload)

	conn.OnMessage(nil, websocket.TextMessage, data)
	conn.OnMessage(nil, websocket.BinaryMessage, data[:protocol.SessionKeySize-1])
	assert.Equal(t, key, receivedKey)
	assert.Equal(t, payload, receivedPayload)

	(&Connection{}).OnMessage(nil, websocket.BinaryMessage, data)
}

func TestConnectionWriteRequiresActiveSocket(t *testing.T) {
	packet := &protocol.Packet{Type: protocol.TypeText, Payload: []byte("message")}
	for _, state := range []protocol.ConnectionState{
		protocol.ConnectionConnecting,
		protocol.ConnectionClosed,
		protocol.ConnectionActive,
	} {
		t.Run(state.String(), func(t *testing.T) {
			conn := &Connection{}
			conn.state.Store(int32(state))
			require.ErrorIs(t, conn.Write(protocol.SessionKey{}, []byte("message")), ErrConnectionNotActive)
			require.ErrorIs(t, conn.WritePacket(protocol.SessionKey{}, packet), ErrConnectionNotActive)
		})
	}
}

func TestConnectionReconnectLoopStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn := &Connection{ctx: ctx}
	conn.state.Store(int32(protocol.ConnectionConnecting))

	done := make(chan struct{})
	go func() {
		conn.reconnectLoop()
		close(done)
	}()
	require.Eventually(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
}

type recordingConnectionObserver struct {
	opened []*Connection
	closed []*Connection
}

func (o *recordingConnectionObserver) OnConnectionOpen(conn *Connection) {
	o.opened = append(o.opened, conn)
}

func (o *recordingConnectionObserver) OnConnectionClose(conn *Connection) {
	o.closed = append(o.closed, conn)
}
