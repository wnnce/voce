package media

import (
	"context"
	"fmt"

	"github.com/pion/interceptor"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// ICEConfig describes one STUN/TURN server configuration.
type ICEConfig struct {
	URLs       []string
	Username   string
	Credential string
}

// ToPionICEServers converts ICEConfigs to Pion ICEServers.
func ToPionICEServers(configs []ICEConfig) []pionwebrtc.ICEServer {
	if len(configs) == 0 {
		return nil
	}
	servers := make([]pionwebrtc.ICEServer, 0, len(configs))
	for _, config := range configs {
		servers = append(servers, pionwebrtc.ICEServer{
			URLs:       append([]string(nil), config.URLs...),
			Username:   config.Username,
			Credential: config.Credential,
		})
	}
	return servers
}

// Factory initializes and owns the Pion MediaEngine and API.
type Factory struct {
	api *pionwebrtc.API
}

// NewFactory creates a Factory with Opus audio codec and default interceptors.
func NewFactory() (*Factory, error) {
	m := &pionwebrtc.MediaEngine{}
	if err := m.RegisterCodec(pionwebrtc.RTPCodecParameters{
		RTPCodecCapability: pionwebrtc.RTPCodecCapability{
			MimeType:     pionwebrtc.MimeTypeOpus,
			ClockRate:    48000,
			Channels:     2,
			SDPFmtpLine:  OpusSDPFmtpLine,
			RTCPFeedback: nil,
		},
		PayloadType: 111,
	}, pionwebrtc.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("media: register media codec: %w", err)
	}

	registry := &interceptor.Registry{}
	if err := pionwebrtc.RegisterDefaultInterceptors(m, registry); err != nil {
		return nil, fmt.Errorf("media: register default interceptors: %w", err)
	}

	return &Factory{
		api: pionwebrtc.NewAPI(
			pionwebrtc.WithMediaEngine(m),
			pionwebrtc.WithInterceptorRegistry(registry),
		),
	}, nil
}

// NewSession creates a new WebRTC media connection session.
func (f *Factory) NewSession(ice []ICEConfig) (*Session, error) {
	if f == nil || f.api == nil {
		return nil, fmt.Errorf("media: factory is nil")
	}

	pc, err := f.api.NewPeerConnection(pionwebrtc.Configuration{
		ICEServers: ToPionICEServers(ice),
	})
	if err != nil {
		return nil, fmt.Errorf("media: new peer connection: %w", err)
	}

	closeOnError := func(err error) (*Session, error) {
		_ = pc.Close()
		return nil, err
	}

	audioTrack, err := pionwebrtc.NewTrackLocalStaticSample(pionwebrtc.RTPCodecCapability{
		MimeType:  pionwebrtc.MimeTypeOpus,
		ClockRate: 48000,
		Channels:  2,
	}, "audio", "voce-server")
	if err != nil {
		return closeOnError(fmt.Errorf("media: create audio track: %w", err))
	}
	if _, err = pc.AddTrack(audioTrack); err != nil {
		return closeOnError(fmt.Errorf("media: add audio track: %w", err))
	}
	dataChannel, err := pc.CreateDataChannel("voce", nil)
	if err != nil {
		return closeOnError(fmt.Errorf("media: create datachannel: %w", err))
	}

	return &Session{
		pc:          pc,
		audioTrack:  audioTrack,
		dataChannel: dataChannel,
	}, nil
}

// Session owns the Pion PeerConnection, local audio track, and DataChannel.
// It is completely independent of Voce workflows or signaling providers.
type Session struct {
	pc          *pionwebrtc.PeerConnection
	audioTrack  *pionwebrtc.TrackLocalStaticSample
	dataChannel *pionwebrtc.DataChannel
}

func (s *Session) PC() *pionwebrtc.PeerConnection {
	return s.pc
}

func (s *Session) AudioTrack() *pionwebrtc.TrackLocalStaticSample {
	return s.audioTrack
}

func (s *Session) DataChannel() *pionwebrtc.DataChannel {
	return s.dataChannel
}

// CreateOffer gathers ICE candidates and generates the local SDP offer.
func (s *Session) CreateOffer(ctx context.Context) (string, error) {
	if s == nil || s.pc == nil {
		return "", fmt.Errorf("media: session is nil")
	}
	gatherComplete := pionwebrtc.GatheringCompletePromise(s.pc)
	offer, err := s.pc.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("media: create offer: %w", err)
	}
	if err = s.pc.SetLocalDescription(offer); err != nil {
		return "", fmt.Errorf("media: set local description: %w", err)
	}
	select {
	case <-gatherComplete:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	local := s.pc.LocalDescription()
	if local == nil || local.SDP == "" {
		return "", fmt.Errorf("media: local description is empty")
	}
	return local.SDP, nil
}

// SetAnswer installs the remote SDP answer from the signaling provider.
func (s *Session) SetAnswer(sdp string) error {
	if s == nil || s.pc == nil {
		return fmt.Errorf("media: session is nil")
	}
	if sdp == "" {
		return fmt.Errorf("media: remote answer is empty")
	}
	if err := s.pc.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  sdp,
	}); err != nil {
		return fmt.Errorf("media: set remote description: %w", err)
	}
	return nil
}

// Close terminates the underlying PeerConnection.
func (s *Session) Close() {
	if s != nil && s.pc != nil {
		_ = s.pc.Close()
	}
}
