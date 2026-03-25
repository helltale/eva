/**
 * Queues and plays TTS audio chunks (browser-decodable: wav/mp3/ogg).
 * Adapters stay separate from UI logic per EVA transport rules.
 */
export class PlaybackManager {
  private ctx: AudioContext | null = null;
  private chain: Promise<void> = Promise.resolve();
  private currentSource: AudioBufferSourceNode | null = null;

  private async ctxOrCreate(): Promise<AudioContext> {
    if (!this.ctx) {
      this.ctx = new AudioContext();
    }
    if (this.ctx.state === "suspended") {
      await this.ctx.resume().catch(() => undefined);
    }
    return this.ctx;
  }

  /** Decode base64 payload and enqueue playback in order. */
  enqueueBase64(b64: string, _encoding: string): void {
    let bin: Uint8Array;
    try {
      bin = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
    } catch {
      return;
    }
    const copy = bin.buffer.slice(bin.byteOffset, bin.byteOffset + bin.byteLength);
    this.chain = this.chain.then(() => this.playArrayBuffer(copy));
  }

  private async playArrayBuffer(buf: ArrayBuffer): Promise<void> {
    const ctx = await this.ctxOrCreate();
    let audio: AudioBuffer;
    try {
      audio = await ctx.decodeAudioData(buf.slice(0));
    } catch {
      return;
    }
    await new Promise<void>((resolve) => {
      const src = ctx.createBufferSource();
      src.buffer = audio;
      src.connect(ctx.destination);
      src.onended = () => resolve();
      this.currentSource = src;
      src.start();
    });
    this.currentSource = null;
  }

  stop() {
    try {
      this.currentSource?.stop();
    } catch {
      /* already stopped */
    }
    this.currentSource = null;
    this.chain = Promise.resolve();
  }

  dispose() {
    this.stop();
    void this.ctx?.close();
    this.ctx = null;
  }
}
