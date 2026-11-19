package voice

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// EncodeWAV16kMono wraps raw 16-bit little-endian mono PCM (at sampleRate) in a
// standard 44-byte RIFF/WAVE header.
func EncodeWAV16kMono(pcm []byte, sampleRate int) []byte {
	if sampleRate <= 0 {
		sampleRate = SampleRate
	}
	dataLen := uint32(len(pcm))
	var buf bytes.Buffer
	buf.Grow(44 + len(pcm))
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36)+dataLen)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))           // PCM fmt chunk size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))            // AudioFormat = 1 (PCM)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))            // NumChannels = 1
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))   // SampleRate
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2)) // ByteRate
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))            // BlockAlign
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))           // BitsPerSample
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataLen)
	buf.Write(pcm)
	return buf.Bytes()
}

// DecodeWAVTo16kMono extracts 16 kHz mono s16le PCM bytes from a RIFF/WAVE byte slice.
// It handles arbitrary subchunks (LIST, INFO, JUNK), stereo-to-mono downmixing,
// and linear resampling to SampleRate (16000 Hz) if the input rate differs.
func DecodeWAVTo16kMono(wav []byte) ([]byte, error) {
	if len(wav) < 44 || string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, fmt.Errorf("некорректный заголовок WAV")
	}
	channels := 1
	sampleRate := SampleRate
	bitsPerSample := 16
	var rawData []byte

	pos := 12
	for pos+8 <= len(wav) {
		chunkID := string(wav[pos : pos+4])
		chunkLen := int(binary.LittleEndian.Uint32(wav[pos+4 : pos+8]))
		pos += 8
		if chunkLen < 0 || pos+chunkLen > len(wav) {
			if chunkID == "data" && pos < len(wav) {
				rawData = wav[pos:]
			}
			break
		}
		switch chunkID {
		case "fmt ":
			if chunkLen >= 16 {
				channels = int(binary.LittleEndian.Uint16(wav[pos+2 : pos+4]))
				sampleRate = int(binary.LittleEndian.Uint32(wav[pos+4 : pos+8]))
				bitsPerSample = int(binary.LittleEndian.Uint16(wav[pos+14 : pos+16]))
			}
		case "data":
			rawData = wav[pos : pos+chunkLen]
		}
		pos += chunkLen
		if chunkLen%2 == 1 {
			pos++ // word alignment pad
		}
	}
	if len(rawData) == 0 {
		return nil, fmt.Errorf("пустой data-блок в WAV")
	}
	if channels < 1 {
		channels = 1
	}
	if sampleRate <= 0 {
		sampleRate = SampleRate
	}
	bytesPerSample := bitsPerSample / 8
	if bytesPerSample < 1 {
		bytesPerSample = 2
	}
	frameSize := channels * bytesPerSample
	numFrames := len(rawData) / frameSize
	if numFrames == 0 {
		return nil, nil
	}

	mono := make([]int16, numFrames)
	for i := 0; i < numFrames; i++ {
		var sum int32
		base := i * frameSize
		for c := 0; c < channels; c++ {
			off := base + c*bytesPerSample
			var s int16
			switch bytesPerSample {
			case 1:
				s = int16(int(rawData[off])-128) << 8
			case 2:
				s = int16(binary.LittleEndian.Uint16(rawData[off : off+2]))
			case 3:
				s = int16(binary.LittleEndian.Uint16(rawData[off+1 : off+3]))
			default:
				s = int16(binary.LittleEndian.Uint16(rawData[off+bytesPerSample-2 : off+bytesPerSample]))
			}
			sum += int32(s)
		}
		mono[i] = int16(sum / int32(channels))
	}

	if sampleRate != SampleRate {
		outFrames := int(int64(numFrames) * int64(SampleRate) / int64(sampleRate))
		if outFrames < 1 {
			outFrames = 1
		}
		resampled := make([]int16, outFrames)
		ratio := float64(sampleRate) / float64(SampleRate)
		for i := 0; i < outFrames; i++ {
			srcIdx := float64(i) * ratio
			idx0 := int(srcIdx)
			if idx0 >= numFrames {
				idx0 = numFrames - 1
			}
			idx1 := idx0 + 1
			if idx1 >= numFrames {
				idx1 = numFrames - 1
			}
			frac := srcIdx - float64(idx0)
			v := (1.0-frac)*float64(mono[idx0]) + frac*float64(mono[idx1])
			resampled[i] = int16(v)
		}
		mono = resampled
	}

	out := make([]byte, len(mono)*2)
	for i, s := range mono {
		binary.LittleEndian.PutUint16(out[i*2:i*2+2], uint16(s))
	}
	return out, nil
}
