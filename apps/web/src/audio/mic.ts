export type MicChunk = {
  sequence: number;
  audioEncoding: string;
  data: string; // base64
};

/**
 * Captures microphone via MediaRecorder; invokes onChunk with base64 webm/opus chunks.
 * Returns stop function. Requires secure context (HTTPS or localhost).
 */
export async function startMicStreaming(
  onChunk: (c: MicChunk) => void,
  opts: { mimeType?: string } = {}
): Promise<() => void> {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  const mime =
    opts.mimeType ??
    (MediaRecorder.isTypeSupported("audio/webm;codecs=opus") ? "audio/webm;codecs=opus" : "audio/webm");
  const rec = new MediaRecorder(stream, { mimeType: mime });
  let seq = 0;
  rec.ondataavailable = async (ev) => {
    if (!ev.data.size) return;
    const buf = await ev.data.arrayBuffer();
    const u8 = new Uint8Array(buf);
    let bin = "";
    const step = 0x8000;
    for (let i = 0; i < u8.length; i += step) {
      bin += String.fromCharCode.apply(null, u8.subarray(i, i + step) as unknown as number[]);
    }
    const b64 = btoa(bin);
    onChunk({ sequence: seq++, audioEncoding: mime, data: b64 });
  };
  rec.start(250);
  return () => {
    try {
      rec.stop();
    } catch {
      /* ignore */
    }
    stream.getTracks().forEach((t) => t.stop());
  };
}
