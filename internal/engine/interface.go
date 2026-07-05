package engine

import (
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/syncx"
)

type Flow interface {
	Publisher
	SendSignal(value schema.Signal)
	SendSignalToPort(port int, value schema.Signal)

	AskSignal(value schema.Signal) *syncx.Collector[schema.Result]
	AskSignalToPort(port int, value schema.Signal) *syncx.Collector[schema.Result]

	SendPayload(value schema.Payload)
	SendPayloadToPort(port int, value schema.Payload)

	SendAudio(value schema.Audio)
	SendAudioToPort(port int, value schema.Audio)

	SendVideo(value schema.Video)
	SendVideoToPort(port int, value schema.Video)
}

type Publisher interface {
	Publish(mt protocol.PacketType, data []byte)
	PublishFull(mt protocol.PacketType, encode protocol.PacketEncode, data []byte)
}

type SocketWriter interface {
	Write(packet *protocol.Packet)
}
