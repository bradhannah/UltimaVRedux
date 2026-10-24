package audio

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

// Tests name these authored cues; game code reads them from the library's routing.
const (
	AmbientFire     AmbientSoundType = "ambient/AmbientFire"
	AmbientFountain AmbientSoundType = "ambient/AmbientFountain"
	AmbientBard     AmbientSoundType = "ambient/AmbientBard"
)

// conformanceDevice is only a controlled transport for the live SDK-backed
// adapter. Playback, gain, windowing, ownership and retirement stay in the SDK.
type conformanceDevice struct {
	source         io.ReadSeeker
	readMutex      sync.Mutex
	playing        atomic.Bool
	position       int64
	volume         float64
	barrierEntered chan struct{}
	barriers       atomic.Int32
}

func (device *conformanceDevice) Play()           { device.playing.Store(true) }
func (device *conformanceDevice) Pause()          { device.playing.Store(false) }
func (device *conformanceDevice) IsPlaying() bool { return device.playing.Load() }
func (device *conformanceDevice) SetVolume(gain float64) {
	device.readMutex.Lock()
	defer device.readMutex.Unlock()
	device.volume = gain
}
func (device *conformanceDevice) PauseAndStopReading() {
	device.Pause()
	if device.barriers.Add(1) == 1 {
		close(device.barrierEntered)
	}
	device.readMutex.Lock()
	defer device.readMutex.Unlock()
}
func (device *conformanceDevice) SetPosition(position time.Duration) error {
	device.readMutex.Lock()
	defer device.readMutex.Unlock()
	offset := int64(position/time.Millisecond) * EditorSampleRate / 1000 * decodedPCMBytesPerFrame
	actual, err := device.source.Seek(offset, io.SeekStart)
	if err == nil {
		device.position = actual
	}
	return err
}
func (device *conformanceDevice) Position() time.Duration {
	device.readMutex.Lock()
	defer device.readMutex.Unlock()
	return time.Duration(device.position * int64(time.Second) / (EditorSampleRate * decodedPCMBytesPerFrame))
}

const EditorSampleRate = 44100
const decodedPCMBytesPerFrame = 4

func unknownRecordingProvenance() RecordingProvenance {
	return RecordingProvenance{OriginalFilename: "unknown", SourceType: "unknown", SourceURL: "unknown",
		SourceCreator: "unknown", License: "unknown", ImportedFrom: "unknown", Processing: []authoring.ProcessingStep{}}
}

// Measure production footfall bursts rather than just repeating window metadata.
func audibleBursts(pcm []byte, sampleRate int) int {
	windowBytes := sampleRate / 100 * 4
	silent, bursts := 10, 0
	for offset := 0; offset+windowBytes <= len(pcm); offset += windowBytes {
		peak := int32(0)
		for index := offset; index < offset+windowBytes; index += 2 {
			sample := int32(int16(binary.LittleEndian.Uint16(pcm[index:])))
			if sample < 0 {
				sample = -sample
			}
			peak = max(peak, sample)
		}
		if peak > 1036 {
			if silent >= 10 {
				bursts++
			}
			silent = 0
		} else {
			silent++
		}
	}
	return bursts
}

func editorTestWAV() []byte {
	pcm := make([]byte, EditorSampleRate*decodedPCMBytesPerFrame)
	for frame := EditorSampleRate / 50; frame < EditorSampleRate*3/50; frame++ {
		binary.LittleEndian.PutUint16(pcm[frame*4+2:], 16000)
	}
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36+len(pcm)))
	wav.WriteString("WAVEfmt ")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(16))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(1))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(2))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(EditorSampleRate))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(EditorSampleRate*4))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(4))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(16))
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(len(pcm)))
	wav.Write(pcm)
	return wav.Bytes()
}
