import { useCallback, useEffect, useState } from 'react';
import { isSecureAudioContext } from '@/audio/secure-context';

export interface DeviceOption {
  deviceId: string;
  label: string;
}

/**
 * Device labels stay empty until the user has granted microphone permission, so
 * fall back to a positional name rather than rendering blank options.
 */
function toOptions(devices: MediaDeviceInfo[], fallbackPrefix: string): DeviceOption[] {
  return devices.map((device, index) => ({
    deviceId: device.deviceId,
    label: device.label || `${fallbackPrefix} ${index + 1}`,
  }));
}

export function useDevices() {
  const [inputs, setInputs] = useState<DeviceOption[]>([]);
  const [outputs, setOutputs] = useState<DeviceOption[]>([]);
  const [error, setError] = useState<string>();

  const refresh = useCallback(async () => {
    if (!navigator.mediaDevices?.enumerateDevices) {
      // mediaDevices is undefined on insecure origins, which is by far the most
      // common cause — not an old browser.
      setError(
        isSecureAudioContext()
          ? '当前浏览器不支持设备枚举'
          : '非安全上下文（需 HTTPS 或 localhost），浏览器隐藏了音频设备',
      );
      return;
    }
    try {
      const devices = await navigator.mediaDevices.enumerateDevices();
      setInputs(toOptions(devices.filter((d) => d.kind === 'audioinput'), '输入设备'));
      setOutputs(toOptions(devices.filter((d) => d.kind === 'audiooutput'), '输出设备'));
      setError(undefined);
    } catch (cause) {
      setError((cause as Error).message);
    }
  }, []);

  useEffect(() => {
    // Mount-time enumeration has to populate state; there is no event to hang
    // it off until the user grants permission or plugs a device in.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
    const onChange = () => void refresh();
    navigator.mediaDevices?.addEventListener?.('devicechange', onChange);
    return () => navigator.mediaDevices?.removeEventListener?.('devicechange', onChange);
  }, [refresh]);

  return { inputs, outputs, error, refresh };
}
