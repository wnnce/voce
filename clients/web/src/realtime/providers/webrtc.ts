import { decodePacket, encodePacket, PacketType, type Packet } from '@/realtime/packet';
import type {
  ConnectionHandlers,
  ConnectionProvider,
  ConnectionTarget,
  RealtimeConnection,
} from '@/realtime/connection';
import { sessionApi, type WebRTCCredentials } from '@/realtime/session';
import { requestMicrophone } from '@/audio/aec';
import { ensureAudioContext } from '@/audio/engine';

/**
 * 等待 RTCPeerConnection 完成本地 ICE 候选者收集。
 * 针对 REST 式 SDP 交换（如 WTN / WHIP），在发送 Offer 前收集完整候选者可免除 trickle 机制。
 */
function waitForIceGatheringComplete(pc: RTCPeerConnection, timeoutMs = 2500): Promise<void> {
  if (pc.iceGatheringState === 'complete') {
    return Promise.resolve();
  }
  return new Promise((resolve) => {
    let timer: ReturnType<typeof setTimeout> | null = null;
    const onStateChange = () => {
      if (pc.iceGatheringState === 'complete') {
        cleanup();
        resolve();
      }
    };
    const cleanup = () => {
      if (timer !== null) {
        clearTimeout(timer);
        timer = null;
      }
      pc.removeEventListener('icegatheringstatechange', onStateChange);
    };
    timer = setTimeout(() => {
      cleanup();
      resolve();
    }, timeoutMs);
    pc.addEventListener('icegatheringstatechange', onStateChange);
  });
}

/**
 * 启动麦克风音频振幅实时监测（RMS），用于驱动上层 UI 的波形与指示灯。
 */
function startMicLevelMonitor(stream: MediaStream, onLevel: (rms: number) => void): () => void {
  let ctx: AudioContext | null = null;
  let timer: ReturnType<typeof setInterval> | null = null;
  try {
    const AudioCtx =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    if (!AudioCtx) return () => {};
    ctx = new AudioCtx();
    const source = ctx.createMediaStreamSource(stream);
    const analyser = ctx.createAnalyser();
    analyser.fftSize = 256;
    source.connect(analyser);

    const dataArray = new Uint8Array(analyser.frequencyBinCount);
    timer = setInterval(() => {
      analyser.getByteTimeDomainData(dataArray);
      let sum = 0;
      for (let i = 0; i < dataArray.length; i++) {
        const val = (dataArray[i] - 128) / 128;
        sum += val * val;
      }
      const rms = Math.sqrt(sum / dataArray.length);
      onLevel(rms);
    }, 100);
  } catch (err) {
    console.warn('[WebRTC] 音频振幅监测初始化异常:', err);
  }

  return () => {
    if (timer !== null) clearInterval(timer);
    if (ctx && ctx.state !== 'closed') {
      ctx.close().catch(() => {});
    }
  };
}

class WebRTCConnection implements RealtimeConnection {
  readonly kind = 'webrtc' as const;

  private pc: RTCPeerConnection | null = null;
  private dataChannel: RTCDataChannel | null = null;
  private localStream: MediaStream | null = null;
  private remoteAudio: HTMLAudioElement | null = null;
  private remoteSource: MediaStreamAudioSourceNode | null = null;
  private stopLevelMonitor: (() => void) | null = null;
  private teardownUrl: string | null = null;
  private teardownToken: string | null = null;
  private closed = false;
  private pendingOpenReject: ((err: Error) => void) | null = null;

  async open(target: ConnectionTarget, handlers: ConnectionHandlers): Promise<void> {
    const { sessionId, inputDeviceId, outputDeviceId, captureEnabled = true, webrtcProvider } = target;
    if (!sessionId) {
      return Promise.reject(new Error('WebRTC 连接需要 sessionId'));
    }

    this.closed = false;

    return new Promise((resolve, reject) => {
      let settled = false;

      const finishSuccess = () => {
        if (settled) return;
        settled = true;
        this.pendingOpenReject = null;
        resolve();
      };

      const finishFailure = (err: Error) => {
        if (settled) return;
        settled = true;
        this.pendingOpenReject = null;
        this.close();
        reject(err);
      };

      this.pendingOpenReject = finishFailure;

      void (async () => {
        try {
          handlers.onState('connecting');

          // 1. 获取服务端 WebRTC 连接凭据 (指定 provider 或默认 provider)
          let creds: WebRTCCredentials;
          try {
            creds = await sessionApi.attachWebRTC(sessionId, webrtcProvider);
          } catch (cause) {
            finishFailure(new Error(`获取 WebRTC 凭据失败: ${(cause as Error).message}`));
            return;
          }

          if (this.closed) return;

          // 2. 组装 STUN 服务器列表并创建 RTCPeerConnection
          const iceServers: RTCIceServer[] = [];
          if (creds.stuns && creds.stuns.length > 0) {
            iceServers.push({ urls: creds.stuns });
          } else {
            iceServers.push({ urls: 'stun:stun.volcengine.com:3478' });
          }

          const pc = new RTCPeerConnection({ iceServers });
          this.pc = pc;

          // 3. 创建 Voce 协议 DataChannel 并挂载监听
          const dc = pc.createDataChannel('voce');
          this.setupDataChannel(dc, handlers, finishSuccess);

          pc.ondatachannel = (event) => {
            if (this.closed) return;
            if (event.channel) {
              this.setupDataChannel(event.channel, handlers, finishSuccess);
            }
          };

          // 提前创建播放用的 audio 元素挂载到 DOM 并预热（利用当前用户手势上下文解锁自动播放限制）
          let audio = this.remoteAudio;
          if (!audio) {
            audio = document.createElement('audio');
            audio.autoplay = true;
            audio.hidden = true;
            audio.setAttribute('playsinline', '');
            document.body.appendChild(audio);
            this.remoteAudio = audio;
            void audio.play().catch(() => {});
          }

          // 4. 采集麦克风并加入 PeerConnection（若启用采集）
          let hasAudioTrack = false;
          if (captureEnabled) {
            try {
              const devId =
                inputDeviceId && inputDeviceId !== 'default-input' ? inputDeviceId : undefined;
              const stream = await requestMicrophone({ deviceId: devId });
              this.localStream = stream;
              for (const track of stream.getTracks()) {
                pc.addTrack(track, stream);
                hasAudioTrack = true;
              }
              this.stopLevelMonitor = startMicLevelMonitor(stream, (level) => {
                handlers.onUplinkLevel?.(level);
              });
            } catch (micErr) {
              console.warn('[WebRTCConnection] 无法打开麦克风，降级为仅接收音频:', micErr);
            }
          }

          // 5. 音频通道协商配置
          if (!hasAudioTrack) {
            try {
              pc.addTransceiver('audio', { direction: 'recvonly' });
            } catch (trErr) {
              console.warn('[WebRTCConnection] addTransceiver warning:', trErr);
            }
          } else {
            // 确保本地麦克风轨道所在 Transceiver 为 sendrecv，以便同时接收下行音频
            for (const transceiver of pc.getTransceivers()) {
              if (
                transceiver.sender.track?.kind === 'audio' ||
                transceiver.receiver.track?.kind === 'audio'
              ) {
                transceiver.direction = 'sendrecv';
              }
            }
          }

          // 6. 监听下行音频轨并自动播放
          pc.ontrack = (event) => {
            if (this.closed) return;
            const stream = event.streams[0] || new MediaStream([event.track]);
            let currentAudio = this.remoteAudio;
            if (!currentAudio) {
              currentAudio = document.createElement('audio');
              currentAudio.autoplay = true;
              currentAudio.hidden = true;
              currentAudio.setAttribute('playsinline', '');
              document.body.appendChild(currentAudio);
              this.remoteAudio = currentAudio;
            }
            currentAudio.srcObject = stream;
            const outId =
              outputDeviceId && outputDeviceId !== 'default-output' ? outputDeviceId : undefined;
            if (
              outId &&
              typeof (currentAudio as HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> })
                .setSinkId === 'function'
            ) {
              (currentAudio as HTMLAudioElement & { setSinkId: (id: string) => Promise<void> })
                .setSinkId(outId)
                .catch(() => {});
            }
            currentAudio.play().catch(async (err) => {
              console.warn('[WebRTCConnection] 远端音频直接播放受限，尝试通过 AudioContext 桥接:', err);
              try {
                const ctx = await ensureAudioContext();
                if (!this.remoteSource) {
                  this.remoteSource = ctx.createMediaStreamSource(stream);
                  this.remoteSource.connect(ctx.destination);
                }
              } catch (ctxErr) {
                console.error('[WebRTCConnection] AudioContext 桥接播放亦失败:', ctxErr);
              }
            });
          };

          // 7. 链路状态变更监听
          pc.onconnectionstatechange = () => {
            if (this.closed) return;
            switch (pc.connectionState) {
              case 'connected':
                handlers.onState('active');
                finishSuccess();
                break;
              case 'disconnected':
                handlers.onState('reconnecting', 'WebRTC 链路中断');
                break;
              case 'failed':
                handlers.onState('error', 'WebRTC 媒体链路连接失败');
                finishFailure(new Error('WebRTC 媒体链路连接失败'));
                break;
              case 'closed':
                if (!this.closed) handlers.onState('closed');
                break;
            }
          };

          // 8. 创建本地 Offer 并收集 ICE 候选者
          const offer = await pc.createOffer();
          await pc.setLocalDescription(offer);
          await waitForIceGatheringComplete(pc, 2500);

          if (this.closed) return;

          const offerSdp = pc.localDescription?.sdp;
          if (!offerSdp) {
            finishFailure(new Error('WebRTC Offer SDP 生成失败'));
            return;
          }

          // 9. 与 WebRTC 媒体服务端交换 SDP (支持 Cloudflare Calls 与 WTN)
          let answerSdp = '';
          if (creds.provider === 'cloudflare') {
            const baseUrl = creds.base_url.replace(/\/+$/, '');
            // 1) 在 Cloudflare Calls 创建客户端 session
            const newSessionRes = await fetch(
              `${baseUrl}/${encodeURIComponent(creds.app_id)}/sessions/new`,
              {
                method: 'POST',
                headers: {
                  'Content-Type': 'application/json',
                  ...(creds.token ? { Authorization: `Bearer ${creds.token}` } : {}),
                },
              },
            );
            if (!newSessionRes.ok) {
              const errText = await newSessionRes.text().catch(() => '');
              finishFailure(
                new Error(`Cloudflare Calls 会话创建失败 (${newSessionRes.status}): ${errText}`),
              );
              return;
            }
            const { sessionId: clientSessionId } = (await newSessionRes.json()) as {
              sessionId: string;
            };
            this.teardownUrl = `${baseUrl}/${encodeURIComponent(creds.app_id)}/sessions/${encodeURIComponent(clientSessionId)}/tracks/close`;
            this.teardownToken = creds.token || null;

            // 2) 提交本地 Offer SDP 并订阅远端服务端的音频轨道
            const tracksRes = await fetch(
              `${baseUrl}/${encodeURIComponent(creds.app_id)}/sessions/${encodeURIComponent(clientSessionId)}/tracks/new`,
              {
                method: 'POST',
                headers: {
                  'Content-Type': 'application/json',
                  ...(creds.token ? { Authorization: `Bearer ${creds.token}` } : {}),
                },
                body: JSON.stringify({
                  sessionDescription: { type: 'offer', sdp: offerSdp },
                  tracks: [
                    {
                      location: 'local',
                      mid: '0',
                      trackName: creds.client_track_name || 'client-audio',
                    },
                    {
                      location: 'remote',
                      sessionId: creds.server_session_id,
                      trackName: creds.server_track_name || 'server-audio',
                    },
                  ],
                }),
              },
            );
            if (!tracksRes.ok) {
              const errText = await tracksRes.text().catch(() => '');
              finishFailure(
                new Error(`Cloudflare Calls 协商失败 (${tracksRes.status}): ${errText}`),
              );
              return;
            }
            const tracksData = (await tracksRes.json()) as {
              sessionDescription?: { sdp: string };
            };
            answerSdp = tracksData.sessionDescription?.sdp || '';
          } else {
            // WTN PubSub 协商
            const baseUrl = creds.base_url.replace(/\/+$/, '');
            const endpoint = `${baseUrl}/webrtc/pub-sub/${encodeURIComponent(creds.app_id)}/${encodeURIComponent(creds.pub_stream_id || '')}/${encodeURIComponent(creds.sub_stream_id || '')}`;
            const url = new URL(endpoint);
            url.searchParams.set('SessionID', sessionId);
            url.searchParams.set('PubMuteAudio', captureEnabled ? 'false' : 'true');
            url.searchParams.set('PubMuteVideo', 'true');
            url.searchParams.set('SubMuteAudio', 'false');
            url.searchParams.set('SubMuteVideo', 'true');

            const response = await fetch(url.toString(), {
              method: 'POST',
              headers: {
                'Content-Type': 'application/sdp',
                ...(creds.token ? { Authorization: `Bearer ${creds.token}` } : {}),
              },
              body: offerSdp,
            });

            if (!response.ok) {
              const errText = await response.text().catch(() => '');
              finishFailure(
                new Error(`WTN 协商失败 (${response.status}): ${errText || response.statusText}`),
              );
              return;
            }

            const location = response.headers.get('Location');
            if (location) {
              this.teardownUrl = location;
              this.teardownToken = creds.token || null;
            }

            answerSdp = await response.text();
          }

          if (this.closed) return;

          await pc.setRemoteDescription({
            type: 'answer',
            sdp: answerSdp,
          });

          // 10. 设置最长建联等待计时器
          setTimeout(() => {
            if (this.closed || settled) return;
            if (
              pc.connectionState === 'connected' ||
              (this.dataChannel && this.dataChannel.readyState === 'open')
            ) {
              handlers.onState('active');
              finishSuccess();
            } else if (pc.connectionState === 'failed') {
              finishFailure(new Error('WebRTC 媒体链路连接失败'));
            } else {
              // 容忍慢网络：若 SDP 协商通过且未明确失败，视为已就绪
              handlers.onState('active');
              finishSuccess();
            }
          }, 4000);
        } catch (err) {
          finishFailure(err instanceof Error ? err : new Error(String(err)));
        }
      })();
    });
  }

  publish(packet: Packet): void {
    if (this.closed) return;
    if (packet.type === PacketType.Audio) {
      // WebRTC 原生音频轨处理音频，不经由 DataChannel 发送
      return;
    }
    const dc = this.dataChannel;
    if (!dc || dc.readyState !== 'open') return;
    try {
      dc.send(encodePacket(packet));
    } catch (err) {
      console.warn('[WebRTCConnection] DataChannel 数据发送失败:', err);
    }
  }

  close(): void {
    this.closed = true;

    if (this.pendingOpenReject) {
      this.pendingOpenReject(new Error('连接已手动取消'));
      this.pendingOpenReject = null;
    }

    // 1. 释放远端 WTN PubSub 资源
    if (this.teardownUrl && this.teardownToken) {
      const url = this.teardownUrl;
      const token = this.teardownToken;
      this.teardownUrl = null;
      this.teardownToken = null;
      fetch(url, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {});
    }

    // 2. 停止麦克风音量监测
    if (this.stopLevelMonitor) {
      this.stopLevelMonitor();
      this.stopLevelMonitor = null;
    }

    // 3. 停止本地麦克风轨道
    if (this.localStream) {
      for (const track of this.localStream.getTracks()) {
        track.stop();
      }
      this.localStream = null;
    }

    // 4. 停止远端音频播放并从 DOM 移除
    if (this.remoteAudio) {
      this.remoteAudio.pause();
      this.remoteAudio.srcObject = null;
      this.remoteAudio.remove();
      this.remoteAudio = null;
    }
    if (this.remoteSource) {
      this.remoteSource.disconnect();
      this.remoteSource = null;
    }

    // 5. 释放 DataChannel
    if (this.dataChannel) {
      const dc = this.dataChannel;
      this.dataChannel = null;
      dc.onopen = null;
      dc.onmessage = null;
      dc.onerror = null;
      dc.onclose = null;
      try {
        if (dc.readyState === 'open' || dc.readyState === 'connecting') {
          dc.close();
        }
      } catch (err) {
        void err;
      }
    }

    // 6. 释放 RTCPeerConnection
    if (this.pc) {
      const pc = this.pc;
      this.pc = null;
      pc.onconnectionstatechange = null;
      pc.ontrack = null;
      pc.ondatachannel = null;
      try {
        pc.close();
      } catch (err) {
        void err;
      }
    }
  }

  private setupDataChannel(
    dc: RTCDataChannel,
    handlers: ConnectionHandlers,
    onSuccess?: () => void,
  ): void {
    this.dataChannel = dc;
    dc.binaryType = 'arraybuffer';

    dc.onopen = () => {
      if (this.closed) return;
      handlers.onState('active');
      onSuccess?.();
    };

    dc.onmessage = (event: MessageEvent<ArrayBuffer | string>) => {
      if (this.closed) return;
      if (typeof event.data === 'string') {
        handlers.onMalformed?.();
        return;
      }
      const { packet } = decodePacket(event.data);
      if (!packet) {
        handlers.onMalformed?.();
        return;
      }
      handlers.onMessage(packet);
    };

    dc.onerror = () => {
      if (this.closed) return;
      console.warn('[WebRTCConnection] DataChannel 异常');
    };

    dc.onclose = () => {
      if (this.closed) return;
      console.log('[WebRTCConnection] DataChannel 关闭');
    };
  }
}

export const webrtcProvider: ConnectionProvider = {
  kind: 'webrtc',
  label: 'WebRTC (WTN)',
  description: '原生 Opus 音频轨 + DataChannel，低延迟抗弱网',
  requiresSession: true,
  managesAudio: true,
  create: () => new WebRTCConnection(),
};
