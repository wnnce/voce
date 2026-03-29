package google_asr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	speech "cloud.google.com/go/speech/apiv2"
	"cloud.google.com/go/speech/apiv2/speechpb"
	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/plugins/base/asr"
	"github.com/wnnce/voce/internal/schema"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
)

//nolint:lll // struct tags are intentionally long for jsonschema
type Config struct {
	ApiKey    string `json:"api_key" jsonschema:"title=API Key,description=Google Cloud Service Account or API Key,example=AIzaSyD..."`
	ProjectID string `json:"project_id" jsonschema:"title=Project ID,description=The unique identifier for your GCP project,example=my-speech-project"`
	Region    string `json:"region" jsonschema:"title=regRegion,description=Region for Chirp 3 deployment,default=us"`
	SpeechEnd int    `json:"speech_end" jsonschema:"title=Speech End Timeout (ms),description=Duration of silence before treating speech as finished,default=500,minimum=100,maximum=5000"`
}

func (c *Config) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(c)
}

func (c *Config) Decode(data []byte) error {
	return sonic.Unmarshal(data, c)
}

type Plugin struct {
	asr.BasePlugin
	config    *Config
	client    *speech.Client
	stream    speechpb.Speech_StreamingRecognizeClient
	connected atomic.Bool
	cancel    context.CancelFunc
}

func NewPlugin(cfg *Config) engine.Plugin {
	plg := &Plugin{
		config: cfg,
	}
	plg.Provider = plg
	return plg
}

func (p *Plugin) Start(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s-speech.googleapis.com", p.config.Region)
	if p.client == nil {
		client, err := speech.NewClient(ctx, option.WithEndpoint(endpoint), option.WithAPIKey(p.config.ApiKey))
		if err != nil {
			return err
		}
		p.client = client
	}
	srmCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	stream, err := p.client.StreamingRecognize(srmCtx)
	if err != nil {
		return err
	}
	recognitionConfig := &speechpb.RecognitionConfig{
		Model:          "chirp-3",
		DecodingConfig: &speechpb.RecognitionConfig_AutoDecodingConfig{},
	}
	if err = stream.Send(&speechpb.StreamingRecognizeRequest{
		Recognizer: fmt.Sprintf("projects/%s/locations/%s/recognizers/_", p.config.ProjectID, p.config.Region),
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: recognitionConfig,
				StreamingFeatures: &speechpb.StreamingRecognitionFeatures{
					InterimResults:            true,
					EnableVoiceActivityEvents: true,
					VoiceActivityTimeout: &speechpb.StreamingRecognitionFeatures_VoiceActivityTimeout{
						SpeechEndTimeout: durationpb.New(time.Duration(p.config.SpeechEnd) * time.Millisecond),
					},
				},
			},
		},
	}); err != nil {
		cancel()
		return err
	}
	p.connected.Store(true)
	p.stream = stream
	go p.readLoop()
	return nil
}

func (p *Plugin) SendAudioData(data []byte, _ bool) error {
	if !p.connected.Load() {
		return errors.New("google stream not connected")
	}
	err := p.stream.Send(&speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_Audio{
			Audio: data,
		},
	})
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func (p *Plugin) Stop() {
	if !p.connected.Load() {
		return
	}
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *Plugin) Shutdown() {
	p.Stop()
	if p.client != nil {
		_ = p.client.Close()
	}
}

func (p *Plugin) Connected() bool {
	return p.connected.Load()
}

func (p *Plugin) readLoop() {
	defer p.connected.Store(false)
	for {
		recv, err := p.stream.Recv()
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			break
		}
		if err != nil {
			slog.ErrorContext(p.Ctx, "google stream recv failed", "error", err)
			return
		}
		if len(recv.Results) == 0 {
			continue
		}
		transcription := &asr.UserTranscription{
			Final:        recv.Results[0].IsFinal,
			LanguageCode: recv.Results[0].LanguageCode,
		}
		if len(recv.Results[0].Alternatives) > 0 {
			transcription.Text = recv.Results[0].Alternatives[0].Transcript
		}
		p.HandleTranscription(transcription)
	}
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name:        "google_asr",
		Description: "Google Cloud Speech to text, Chirp 3",
		Outputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadASRResult, "text", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadASRResult, "role", engine.TypeString, true).
			Build(),
	}); err != nil {
		panic(err)
	}
}
