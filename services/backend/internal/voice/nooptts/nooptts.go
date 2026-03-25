package nooptts

import (
	"encoding/base64"

	"eva/services/backend/internal/audio/wav"
)

// NoopWAVBytes returns a tiny valid WAV (short tone) for Web Audio decode checks in the browser.
func NoopWAVBytes() []byte {
	const sampleRate = 22050
	pcm := wav.ShortTonePCM(sampleRate, 523.25, 120, 0.06)
	return wav.EncodePCM16Mono(pcm, sampleRate)
}

// WSChunk returns one tts.chunk payload (full WAV as base64). Clients decode each chunk; one chunk = one utterance for noop.
func WSChunk() (sequence int, audioEncoding, dataBase64 string) {
	return 0, "audio/wav", base64.StdEncoding.EncodeToString(NoopWAVBytes())
}
