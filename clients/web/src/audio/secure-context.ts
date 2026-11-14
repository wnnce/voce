/**
 * getUserMedia and enumerateDevices only exist in a secure context: HTTPS, or
 * http on localhost/127.0.0.1. Over plain http on a LAN hostname
 * `navigator.mediaDevices` is simply undefined, which is easy to misread as
 * "the browser does not support audio".
 */
export function isSecureAudioContext(): boolean {
  if (typeof window === 'undefined') return true;
  return window.isSecureContext !== false;
}
