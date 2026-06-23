package remote

import (
	"context"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pluginv1 "github.com/wnnce/voce/api/plugin/v1"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
	"google.golang.org/grpc"
)

// fakeStream is a channel-based mock for bidirectional gRPC streaming.
type fakeStream struct {
	grpc.ClientStream
	ctx    context.Context
	sendCh chan *pluginv1.RuntimeMessage // messages the plugin sends TO remote
	recvCh chan *pluginv1.RuntimeMessage // messages injected FROM remote
}

func newFakeStream(ctx context.Context) *fakeStream {
	return &fakeStream{
		ctx:    ctx,
		sendCh: make(chan *pluginv1.RuntimeMessage, 64),
		recvCh: make(chan *pluginv1.RuntimeMessage, 64),
	}
}

func (f *fakeStream) Send(m *pluginv1.RuntimeMessage) error {
	select {
	case <-f.ctx.Done():
		return f.ctx.Err()
	case f.sendCh <- m:
		return nil
	}
}

func (f *fakeStream) Recv() (*pluginv1.RuntimeMessage, error) {
	select {
	case <-f.ctx.Done():
		return nil, f.ctx.Err()
	case m, ok := <-f.recvCh:
		if !ok {
			return nil, io.EOF
		}
		return m, nil
	}
}

func (f *fakeStream) Context() context.Context {
	return f.ctx
}

// fakePluginClient returns a pre-configured fakeStream on RunInstance.
type fakePluginClient struct {
	pluginv1.RemotePluginServiceClient
	stream *fakeStream
}

func (f *fakePluginClient) RunInstance(ctx context.Context, _ ...grpc.CallOption) (pluginv1.RemotePluginService_RunInstanceClient, error) {
	f.stream.ctx = ctx
	return f.stream, nil
}

// replyOK reads one message from sendCh and replies with REPORT(OK).
func replyOK(stream *fakeStream) {
	msg := <-stream.sendCh
	stream.recvCh <- &pluginv1.RuntimeMessage{
		Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
		CorrelationId: msg.MessageId,
		Body: &pluginv1.RuntimeMessage_Report{
			Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
		},
	}
}

// startPlugin performs OnStart, waits for readLoop to transition to Streaming, then does OnReady.
func startPlugin(t *testing.T, plugin *Plugin, flow engine.Flow, stream *fakeStream) {
	t.Helper()
	ctx := context.Background()

	go func() {
		msg := <-stream.sendCh
		assert.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE, msg.Type)
		assert.Equal(t, pluginv1.LifecycleType_LIFECYCLE_TYPE_START, msg.GetLifecycle().GetType())
		stream.recvCh <- &pluginv1.RuntimeMessage{
			Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
			CorrelationId: msg.MessageId,
			Body: &pluginv1.RuntimeMessage_Report{
				Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
			},
		}
	}()
	require.NoError(t, plugin.OnStart(ctx, flow))
	require.Equal(t, PluginStateStreaming, plugin.State())

	// Answer the OnReady doCall in a background goroutine.
	go replyOK(stream)
	plugin.OnReady(ctx, flow)
}

func TestPlugin(t *testing.T) {
	t.Run("IndexUsesSingleLaneByDefaultAndSplitLaneInMultiMode", func(t *testing.T) {
		plugin := NewPlugin(nil, "inst-1", engine.PluginMetadata{Name: "test"})
		assert.Equal(t, 0, plugin.index(pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL))
		assert.Equal(t, 0, plugin.index(pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_PAYLOAD))
		assert.Equal(t, 0, plugin.index(pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE))

		multiPlugin := NewPlugin(nil, "inst-1", engine.PluginMetadata{Name: "test"}, true)
		assert.Equal(t, 0, multiPlugin.index(pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL))
		assert.Equal(t, 1, multiPlugin.index(pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_PAYLOAD))
		assert.Equal(t, 0, multiPlugin.index(pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE))
	})

	t.Run("OnStartTransitionsToStreaming", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})

			require.Equal(t, PluginStateCreated, plugin.State())
			go func() {
				msg := <-stream.sendCh
				assert.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE, msg.Type)
				assert.Equal(t, pluginv1.LifecycleType_LIFECYCLE_TYPE_START, msg.GetLifecycle().GetType())
				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
					CorrelationId: msg.MessageId,
					Body: &pluginv1.RuntimeMessage_Report{
						Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
					},
				}
			}()
			require.NoError(t, plugin.OnStart(context.Background(), &engine.MockFlow{}))

			assert.Equal(t, PluginStateStreaming, plugin.State())

			go replyOK(stream)
			plugin.OnStop()
			synctest.Wait()
		})
	})

	t.Run("OnStart_ReportErrorSetsFailed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})

			go func() {
				msg := <-stream.sendCh
				assert.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE, msg.Type)
				assert.Equal(t, pluginv1.LifecycleType_LIFECYCLE_TYPE_START, msg.GetLifecycle().GetType())
				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
					CorrelationId: msg.MessageId,
					Body: &pluginv1.RuntimeMessage_Report{
						Report: &pluginv1.EventReport{
							Status: pluginv1.ReportStatus_REPORT_STATUS_ERROR,
							Error: &pluginv1.RemoteError{
								Code:    "START_FAILED",
								Message: "start failed",
							},
						},
					},
				}
			}()

			err := plugin.OnStart(context.Background(), &engine.MockFlow{})
			require.Error(t, err)
			assert.Equal(t, PluginStateFailed, plugin.State())
			synctest.Wait()
			assert.Equal(t, PluginStateFailed, plugin.State())
		})
	})

	t.Run("OnReady_SendsLifecycleReady", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})
			flow := &engine.MockFlow{}

			go func() {
				msg := <-stream.sendCh
				assert.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE, msg.Type)
				assert.Equal(t, pluginv1.LifecycleType_LIFECYCLE_TYPE_START, msg.GetLifecycle().GetType())
				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
					CorrelationId: msg.MessageId,
					Body: &pluginv1.RuntimeMessage_Report{
						Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
					},
				}
			}()
			require.NoError(t, plugin.OnStart(context.Background(), flow))

			// Verify the OnReady message type before replying
			go func() {
				msg := <-stream.sendCh
				assert.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE, msg.Type)
				assert.Equal(t, pluginv1.LifecycleType_LIFECYCLE_TYPE_READY, msg.GetLifecycle().GetType())

				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
					CorrelationId: msg.MessageId,
					Body: &pluginv1.RuntimeMessage_Report{
						Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
					},
				}
			}()

			plugin.OnReady(context.Background(), flow)
			assert.Equal(t, PluginStateStreaming, plugin.State())

			go replyOK(stream)
			plugin.OnStop()
			synctest.Wait()
		})
	})

	t.Run("OnSignal_DoCallSuccess", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})
			flow := &engine.MockFlow{}

			startPlugin(t, plugin, flow, stream)

			// Reply to the signal doCall
			go func() {
				msg := <-stream.sendCh
				assert.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL, msg.Type)
				assert.Equal(t, "test_sig", msg.GetSignal().GetName())

				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
					CorrelationId: msg.MessageId,
					Body: &pluginv1.RuntimeMessage_Report{
						Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
					},
				}
			}()

			sig := schema.NewSignal("test_sig")
			_ = sig.Set("key", "val")
			plugin.OnSignal(context.Background(), flow, sig.ReadOnly())

			assert.Equal(t, PluginStateStreaming, plugin.State())

			go replyOK(stream)
			plugin.OnStop()
			synctest.Wait()
		})
	})

	t.Run("OnSignal_LeaseExpired", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})
			flow := &engine.MockFlow{}

			startPlugin(t, plugin, flow, stream)

			// Consume the sent message but never reply — lease will expire
			go func() { <-stream.sendCh }()

			sig := schema.NewSignal("timeout_sig")
			plugin.OnSignal(context.Background(), flow, sig.ReadOnly())

			// doCall should have set state to Failed after 30s lease expired
			assert.Equal(t, PluginStateFailed, plugin.State())

			plugin.OnStop()
			synctest.Wait()
		})
	})

	t.Run("OnSignal_AckRenewsLease", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})
			flow := &engine.MockFlow{}

			startPlugin(t, plugin, flow, stream)

			go func() {
				msg := <-stream.sendCh

				// ACK every 20s, well within the 30s lease
				time.Sleep(20 * time.Second)
				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_ACK,
					CorrelationId: msg.MessageId,
				}
				time.Sleep(20 * time.Second)
				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_ACK,
					CorrelationId: msg.MessageId,
				}
				// Final REPORT after 50s total — well beyond the original 30s lease
				time.Sleep(10 * time.Second)
				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
					CorrelationId: msg.MessageId,
					Body: &pluginv1.RuntimeMessage_Report{
						Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
					},
				}
			}()

			sig := schema.NewSignal("ack_sig")
			plugin.OnSignal(context.Background(), flow, sig.ReadOnly())

			// Even though 50s elapsed, ACKs kept renewing so state should still be Streaming
			assert.Equal(t, PluginStateStreaming, plugin.State())

			go replyOK(stream)
			plugin.OnStop()
			synctest.Wait()
		})
	})

	t.Run("OnPayload_DoCallSuccess", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})
			flow := &engine.MockFlow{}

			startPlugin(t, plugin, flow, stream)

			go func() {
				msg := <-stream.sendCh
				assert.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_PAYLOAD, msg.Type)
				assert.Equal(t, "llm_chunk", msg.GetPayload().GetName())

				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
					CorrelationId: msg.MessageId,
					Body: &pluginv1.RuntimeMessage_Report{
						Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
					},
				}
			}()

			p := schema.NewPayload("llm_chunk")
			_ = p.Set("text", "hello")
			plugin.OnPayload(context.Background(), flow, p.ReadOnly())

			assert.Equal(t, PluginStateStreaming, plugin.State())

			go replyOK(stream)
			plugin.OnStop()
			synctest.Wait()
		})
	})

	t.Run("OnPayload_ContextCanceledSendsCancelAndWaitsForCanceledReport", func(t *testing.T) {
		stream := newFakeStream(context.Background())
		client := &fakePluginClient{stream: stream}
		plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})
		flow := &engine.MockFlow{}

		startPlugin(t, plugin, flow, stream)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			p := schema.NewPayload("asr_result")
			_ = p.Set("text", "hello")
			plugin.OnPayload(ctx, flow, p.ReadOnly())
			close(done)
		}()

		payloadMsg := <-stream.sendCh
		require.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_PAYLOAD, payloadMsg.Type)
		require.NotEmpty(t, payloadMsg.MessageId)

		cancel()

		cancelMsg := <-stream.sendCh
		require.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_CANCEL, cancelMsg.Type)
		assert.Equal(t, payloadMsg.MessageId, cancelMsg.CorrelationId)
		require.NotNil(t, cancelMsg.GetCancel())

		select {
		case <-done:
			t.Fatal("OnPayload returned before canceled report")
		default:
		}

		stream.recvCh <- &pluginv1.RuntimeMessage{
			Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
			CorrelationId: payloadMsg.MessageId,
			Body: &pluginv1.RuntimeMessage_Report{
				Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_CANCELED},
			},
		}

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("OnPayload did not return after canceled report")
		}
		assert.Equal(t, PluginStateStreaming, plugin.State())

		go replyOK(stream)
		plugin.OnStop()
	})

	t.Run("MultiModeAllowsPayloadAndSignalInFlight", func(t *testing.T) {
		stream := newFakeStream(context.Background())
		client := &fakePluginClient{stream: stream}
		plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"}, true)
		flow := &engine.MockFlow{}

		startPlugin(t, plugin, flow, stream)

		payloadDone := make(chan struct{})
		go func() {
			p := schema.NewPayload("asr_result")
			_ = p.Set("text", "hello")
			plugin.OnPayload(context.Background(), flow, p.ReadOnly())
			close(payloadDone)
		}()

		payloadMsg := <-stream.sendCh
		require.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_PAYLOAD, payloadMsg.Type)
		require.NotEmpty(t, payloadMsg.MessageId)

		signalDone := make(chan struct{})
		go func() {
			sig := schema.NewSignal("user_speech_start")
			plugin.OnSignal(context.Background(), flow, sig.ReadOnly())
			close(signalDone)
		}()

		signalMsg := <-stream.sendCh
		require.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL, signalMsg.Type)
		require.NotEmpty(t, signalMsg.MessageId)

		stream.recvCh <- &pluginv1.RuntimeMessage{
			Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
			CorrelationId: signalMsg.MessageId,
			Body: &pluginv1.RuntimeMessage_Report{
				Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
			},
		}

		select {
		case <-signalDone:
		case <-time.After(time.Second):
			t.Fatal("OnSignal did not return after signal report")
		}
		select {
		case <-payloadDone:
			t.Fatal("OnPayload returned before payload report")
		default:
		}

		stream.recvCh <- &pluginv1.RuntimeMessage{
			Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
			CorrelationId: payloadMsg.MessageId,
			Body: &pluginv1.RuntimeMessage_Report{
				Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
			},
		}

		select {
		case <-payloadDone:
		case <-time.After(time.Second):
			t.Fatal("OnPayload did not return after payload report")
		}
		assert.Equal(t, PluginStateStreaming, plugin.State())

		go replyOK(stream)
		plugin.OnStop()
	})

	t.Run("HandleEmitSignal", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})

			var gotSignal schema.Signal
			var gotPort int
			flow := &engine.MockFlow{
				OnSignalHook: func(port int, s schema.Signal) {
					gotSignal = s
					gotPort = port
				},
			}

			startPlugin(t, plugin, flow, stream)

			// Inject an EMIT_SIGNAL from remote side
			stream.recvCh <- &pluginv1.RuntimeMessage{
				Type: pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_EMIT_SIGNAL,
				Body: &pluginv1.RuntimeMessage_EmitSignal{
					EmitSignal: &pluginv1.EmitSignal{
						Signal: &pluginv1.SignalEvent{
							Name:       "user_speech_start",
							Properties: []byte(`{"confidence": 0.95}`),
						},
						Port: 2,
					},
				},
			}

			synctest.Wait()

			require.NotNil(t, gotSignal)
			assert.Equal(t, "user_speech_start", gotSignal.Name())
			assert.Equal(t, 2, gotPort)
			assert.InEpsilon(t, 0.95, schema.GetAs[float64](gotSignal, "confidence"), 0.001)

			go replyOK(stream)
			plugin.OnStop()
			synctest.Wait()
		})
	})

	t.Run("HandleEmitPayload", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})

			var gotPayload schema.Payload
			flow := &engine.MockFlow{
				OnPayloadHook: func(port int, p schema.Payload) {
					gotPayload = p
				},
			}

			startPlugin(t, plugin, flow, stream)

			stream.recvCh <- &pluginv1.RuntimeMessage{
				Type: pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD,
				Body: &pluginv1.RuntimeMessage_EmitPayload{
					EmitPayload: &pluginv1.EmitPayload{
						Payload: &pluginv1.PayloadEvent{
							Name:       "asr_result",
							Properties: []byte(`{"text": "hello world"}`),
						},
					},
				},
			}

			synctest.Wait()

			require.NotNil(t, gotPayload)
			assert.Equal(t, "asr_result", gotPayload.Name())
			assert.Equal(t, "hello world", schema.GetAs[string](gotPayload, "text"))

			go replyOK(stream)
			plugin.OnStop()
			synctest.Wait()
		})
	})

	t.Run("OnStop_SetsStopped", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})
			flow := &engine.MockFlow{}

			startPlugin(t, plugin, flow, stream)
			assert.Equal(t, PluginStateStreaming, plugin.State())

			// Answer the STOP lifecycle doCall and verify its content
			go func() {
				msg := <-stream.sendCh
				assert.Equal(t, pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_LIFECYCLE, msg.Type)
				assert.Equal(t, pluginv1.LifecycleType_LIFECYCLE_TYPE_STOP, msg.GetLifecycle().GetType())

				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
					CorrelationId: msg.MessageId,
					Body: &pluginv1.RuntimeMessage_Report{
						Report: &pluginv1.EventReport{Status: pluginv1.ReportStatus_REPORT_STATUS_OK},
					},
				}
			}()

			plugin.OnStop()
			synctest.Wait()

			assert.Equal(t, PluginStateStopped, plugin.State())
		})
	})

	t.Run("StreamClose_SetsStopped", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})
			flow := &engine.MockFlow{}

			startPlugin(t, plugin, flow, stream)

			// Close the recv channel to simulate stream EOF
			close(stream.recvCh)
			synctest.Wait()

			assert.Equal(t, PluginStateStopped, plugin.State())
		})
	})

	t.Run("ReportError_SetsFailed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := newFakeStream(context.Background())
			client := &fakePluginClient{stream: stream}
			plugin := NewPlugin(client, "inst-1", engine.PluginMetadata{Name: "test"})
			flow := &engine.MockFlow{}

			startPlugin(t, plugin, flow, stream)

			// Answer with error REPORT
			go func() {
				msg := <-stream.sendCh
				stream.recvCh <- &pluginv1.RuntimeMessage{
					Type:          pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_REPORT,
					CorrelationId: msg.MessageId,
					Body: &pluginv1.RuntimeMessage_Report{
						Report: &pluginv1.EventReport{
							Status: pluginv1.ReportStatus_REPORT_STATUS_ERROR,
							Error: &pluginv1.RemoteError{
								Code:    "INTERNAL",
								Message: "something broke",
							},
						},
					},
				}
			}()

			sig := schema.NewSignal("fail_sig")
			plugin.OnSignal(context.Background(), flow, sig.ReadOnly())

			assert.Equal(t, PluginStateFailed, plugin.State())

			plugin.OnStop()
			synctest.Wait()
		})
	})
}
