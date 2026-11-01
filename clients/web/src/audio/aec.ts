import { isSecureAudioContext } from '@/audio/secure-context';

/**
 * Microphone access.
 *
 * Echo cancellation is the browser's job — it owns the loopback reference and
 * the time alignment. We only ask for it; the page never gates or filters the
 * uplink itself.
 *
 * The constraints use `ideal` rather than bare booleans: bare booleans are
 * treated as required by some implementations and rejected outright, and a
 * rejected constraint can drop the whole track. `ideal` keeps the request
 * best-effort so the microphone always opens.
 */

export interface MicrophoneRequest {
  deviceId?: string;
}

export async function requestMicrophone(request: MicrophoneRequest): Promise<MediaStream> {
  if (!navigator.mediaDevices?.getUserMedia) {
    throw new Error(
      isSecureAudioContext()
        ? '浏览器不支持音频采集'
        : '非安全上下文，无法打开麦克风：请用 https 或 localhost 访问',
    );
  }

  return navigator.mediaDevices.getUserMedia({
    audio: {
      deviceId: request.deviceId ? { exact: request.deviceId } : undefined,
      echoCancellation: { ideal: true },
      noiseSuppression: { ideal: true },
      autoGainControl: { ideal: true },
      channelCount: { ideal: 1 },
    },
  });
}
