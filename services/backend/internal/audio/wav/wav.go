package wav

import (
	"encoding/binary"
	"math"
)

// EncodePCM16Mono builds a valid RIFF WAV (PCM s16le, mono).
func EncodePCM16Mono(samples []int16, sampleRate int) []byte {
	const hdr = 44
	dataBytes := len(samples) * 2
	out := make([]byte, hdr+dataBytes)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataBytes))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(out[22:24], 1) // mono
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(out[32:34], 2)
	binary.LittleEndian.PutUint16(out[34:36], 16)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataBytes))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[44+i*2:], uint16(s))
	}
	return out
}

// ShortTonePCM generates a short sine burst (pleasant end-of-utterance cue for noop TTS).
func ShortTonePCM(sampleRate int, freqHz float64, durMs int, amplitude float64) []int16 {
	if amplitude <= 0 || amplitude > 1 {
		amplitude = 0.08
	}
	n := sampleRate * durMs / 1000
	if n < 1 {
		n = 1
	}
	out := make([]int16, n)
	amp := 32767.0 * amplitude
	fade := func(i int) float64 {
		// Hann-style edge to avoid clicks
		if n < 4 {
			return 1
		}
		t := float64(i) / float64(n-1)
		return 0.5 * (1 - math.Cos(math.Pi*t))
	}
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		out[i] = int16(amp * math.Sin(2*math.Pi*freqHz*t) * fade(i))
	}
	return out
}
