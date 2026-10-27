package core

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/ClipMyHorseTV/livego/av"
	"github.com/ClipMyHorseTV/livego/utils/pool"
)

type ChunkStream struct {
	Format    uint32
	CSID      uint32
	Timestamp uint32
	Length    uint32
	TypeID    uint32
	StreamID  uint32
	timeDelta uint32
	exted     bool
	index     uint32
	remain    uint32
	got       bool
	tmpFromat uint32
	Data      []byte
}

func (chunkStream *ChunkStream) full() bool {
	return chunkStream.got
}

func (chunkStream *ChunkStream) new() {
	chunkStream.got = false
	chunkStream.index = 0
	chunkStream.remain = chunkStream.Length
	chunkStream.Data = pool.Get(int(chunkStream.Length))
}

func (chunkStream *ChunkStream) writeHeader(w *ReadWriter) error {
	if err := chunkStream.writeBasicHeader(w); err != nil {
		return err
	}
	// Chunk Message Header
	ts := chunkStream.Timestamp
	if chunkStream.Format != 3 {
		ts = min(ts, 0xffffff)
		if err := chunkStream.writeMessageHeader(w, ts); err != nil {
			return err
		}
	}
	// Extended Timestamp
	if ts >= 0xffffff {
		if err := w.WriteUintBE(chunkStream.Timestamp, 4); err != nil {
			return err
		}
	}
	return w.WriteError()
}

// writeBasicHeader writes the chunk format and chunk stream ID.
func (chunkStream *ChunkStream) writeBasicHeader(w *ReadWriter) error {
	h := chunkStream.Format << 6
	switch {
	case chunkStream.CSID < 64:
		return w.WriteUintBE(h|chunkStream.CSID, 1)
	case chunkStream.CSID-64 < 256:
		if err := w.WriteUintBE(h, 1); err != nil {
			return err
		}
		return w.WriteUintLE(chunkStream.CSID-64, 1)
	case chunkStream.CSID-64 < 65536:
		if err := w.WriteUintBE(h|1, 1); err != nil {
			return err
		}
		return w.WriteUintLE(chunkStream.CSID-64, 2)
	}
	return nil
}

// writeMessageHeader writes the fields of the chunk message header present in the chunk's format.
func (chunkStream *ChunkStream) writeMessageHeader(w *ReadWriter, ts uint32) error {
	if err := w.WriteUintBE(ts, 3); err != nil {
		return err
	}
	if chunkStream.Format == 2 {
		return nil
	}
	if chunkStream.Length > 0xffffff {
		return fmt.Errorf("length=%d", chunkStream.Length)
	}
	if err := w.WriteUintBE(chunkStream.Length, 3); err != nil {
		return err
	}
	if err := w.WriteUintBE(chunkStream.TypeID, 1); err != nil {
		return err
	}
	if chunkStream.Format == 1 {
		return nil
	}
	return w.WriteUintLE(chunkStream.StreamID, 4)
}

func (chunkStream *ChunkStream) writeChunk(w *ReadWriter, chunkSize int) error {
	switch chunkStream.TypeID {
	case av.TagAudio:
		chunkStream.CSID = 4
	case av.TagVideo, av.TagScriptDataAMF0, av.TagScriptDataAMF3:
		chunkStream.CSID = 6
	}

	if chunkSize <= 0 || int64(chunkSize) > math.MaxUint32 {
		return fmt.Errorf("invalid chunk size %d", chunkSize)
	}
	size := uint32(chunkSize)
	dataLen := len(chunkStream.Data)
	if int64(dataLen) > math.MaxUint32 {
		return fmt.Errorf("chunk data too large: %d bytes", dataLen)
	}
	remaining := uint32(dataLen)

	totalLen := uint32(0)
	numChunks := chunkStream.Length / size
	for i := uint32(0); i <= numChunks; i++ {
		if totalLen == chunkStream.Length {
			break
		}
		if i == 0 {
			chunkStream.Format = uint32(0)
		} else {
			chunkStream.Format = uint32(3)
		}
		if err := chunkStream.writeHeader(w); err != nil {
			return err
		}
		inc := size
		start := i * size
		if remaining-start <= inc {
			inc = remaining - start
		}
		totalLen += inc
		end := start + inc
		buf := chunkStream.Data[start:end]
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}

	return nil
}

// extendTimestamp returns ts, or the following 4-byte extended timestamp if ts signals one.
// Read errors are sticky and reported by readChunk.
func (chunkStream *ChunkStream) extendTimestamp(r *ReadWriter, ts uint32) uint32 {
	chunkStream.exted = ts == 0xffffff
	if !chunkStream.exted {
		return ts
	}
	ext, _ := r.ReadUintBE(4)
	return ext
}

func (chunkStream *ChunkStream) readChunk(r *ReadWriter, chunkSize uint32) error {
	if chunkStream.remain != 0 && chunkStream.tmpFromat != 3 {
		return fmt.Errorf("invalid remain = %d", chunkStream.remain)
	}
	switch chunkStream.CSID {
	case 0:
		id, _ := r.ReadUintLE(1)
		chunkStream.CSID = id + 64
	case 1:
		id, _ := r.ReadUintLE(2)
		chunkStream.CSID = id + 64
	}

	switch chunkStream.tmpFromat {
	case 0:
		chunkStream.Format = chunkStream.tmpFromat
		timestamp, _ := r.ReadUintBE(3)
		chunkStream.Length, _ = r.ReadUintBE(3)
		chunkStream.TypeID, _ = r.ReadUintBE(1)
		chunkStream.StreamID, _ = r.ReadUintLE(4)
		chunkStream.Timestamp = chunkStream.extendTimestamp(r, timestamp)
		chunkStream.new()
	case 1:
		chunkStream.Format = chunkStream.tmpFromat
		timeStamp, _ := r.ReadUintBE(3)
		chunkStream.Length, _ = r.ReadUintBE(3)
		chunkStream.TypeID, _ = r.ReadUintBE(1)
		timeStamp = chunkStream.extendTimestamp(r, timeStamp)
		chunkStream.timeDelta = timeStamp
		chunkStream.Timestamp += timeStamp
		chunkStream.new()
	case 2:
		chunkStream.Format = chunkStream.tmpFromat
		timeStamp, _ := r.ReadUintBE(3)
		timeStamp = chunkStream.extendTimestamp(r, timeStamp)
		chunkStream.timeDelta = timeStamp
		chunkStream.Timestamp += timeStamp
		chunkStream.new()
	case 3:
		if err := chunkStream.readContinuationHeader(r); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid format=%d", chunkStream.Format)
	}
	size := min(chunkStream.remain, chunkSize)

	buf := chunkStream.Data[chunkStream.index : chunkStream.index+size]
	if _, err := r.Read(buf); err != nil {
		return err
	}
	chunkStream.index += size
	chunkStream.remain -= size
	if chunkStream.remain == 0 {
		chunkStream.got = true
	}

	return r.readError
}

// readContinuationHeader handles a type 3 chunk header, which either starts a new message with the
// previous header values or continues the current message.
func (chunkStream *ChunkStream) readContinuationHeader(r *ReadWriter) error {
	if chunkStream.remain == 0 {
		switch chunkStream.Format {
		case 0:
			if chunkStream.exted {
				timestamp, _ := r.ReadUintBE(4)
				chunkStream.Timestamp = timestamp
			}
		case 1, 2:
			var timedet uint32
			if chunkStream.exted {
				timedet, _ = r.ReadUintBE(4)
			} else {
				timedet = chunkStream.timeDelta
			}
			chunkStream.Timestamp += timedet
		}
		chunkStream.new()
		return nil
	}

	if !chunkStream.exted {
		return nil
	}
	b, err := r.Peek(4)
	if err != nil {
		return err
	}
	if binary.BigEndian.Uint32(b) == chunkStream.Timestamp {
		if _, err := r.Discard(4); err != nil {
			return err
		}
	}
	return nil
}
