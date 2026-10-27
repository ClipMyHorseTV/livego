package h264

import (
	"bytes"
	"errors"
	"io"
)

const (
	iFrame byte = 0
	pFrame byte = 1
	bFrame byte = 2
)

const (
	naluTypeNotDefine byte = 0
	naluTypeSlice     byte = 1  // slice_layer_without_partioning_rbsp() sliceheader
	naluTypeDPA       byte = 2  // slice_data_partition_a_layer_rbsp( ), slice_header
	naluTypeDPB       byte = 3  // slice_data_partition_b_layer_rbsp( )
	naluTypeDPC       byte = 4  // slice_data_partition_c_layer_rbsp( )
	naluTypeIDR       byte = 5  // slice_layer_without_partitioning_rbsp( ),sliceheader
	naluTypeSEI       byte = 6  // sei_rbsp( )
	naluTypeSPS       byte = 7  // seq_parameter_set_rbsp( )
	naluTypePPS       byte = 8  // pic_parameter_set_rbsp( )
	naluTypeAUD       byte = 9  // access_unit_delimiter_rbsp( )
	naluTypeEOSeq     byte = 10 // end_of_seq_rbsp( )
	naluTypeEOStream  byte = 11 // end_of_stream_rbsp( )
	naluTypeFiller    byte = 12 // filler_data_rbsp( )
)

const (
	naluBytesLen int = 4
	maxSpsPpsLen int = 2 * 1024
)

var (
	errDecConfigTooShort = errors.New("avc decoder config record too short")
	errSPSData           = errors.New("sps data error")
	errPPSHeader         = errors.New("pps header error")
	errPPSData           = errors.New("pps data error")
	errNaluHeaderInvalid = errors.New("nalu header invalid")
	errVideoDataInvalid  = errors.New("video data not match")
	errDataSizeNotMatch  = errors.New("data size not match")
	errNaluBodyLen       = errors.New("nalu body len error")
)

var (
	startCode = []byte{0x00, 0x00, 0x00, 0x01}
	naluAud   = []byte{0x00, 0x00, 0x00, 0x01, 0x09, 0xf0}
)

type Parser struct {
	frameType    byte
	specificInfo []byte
	pps          *bytes.Buffer
}

func NewParser() *Parser {
	return &Parser{
		pps: bytes.NewBuffer(make([]byte, maxSpsPpsLen)),
	}
}

// return value 1:sps, value2 :pps
func (parser *Parser) parseSpecificInfo(src []byte) error {
	if len(src) < 9 {
		return errDecConfigTooShort
	}
	sps := []byte{}
	pps := []byte{}

	// AVCDecoderConfigurationRecord: configVersion (8 bits), avcProfileIndication (8 bits),
	// profileCompatibility (8 bits), avcLevelIndication (8 bits), reserved (6 bits),
	// lengthSizeMinusOne (2 bits), reserved (3 bits), numOfSequenceParameterSets (5 bits).
	// Only the SPS and PPS payloads are needed here.

	// get sps
	spsLen := int(src[6])<<8 | int(src[7])

	if len(src[8:]) < spsLen || spsLen <= 0 {
		return errSPSData
	}
	sps = append(sps, startCode...)
	sps = append(sps, src[8:(8+spsLen)]...)

	// get pps (tmpBuf[0] is numOfPictureParameterSets)
	tmpBuf := src[(8 + spsLen):]
	if len(tmpBuf) < 4 {
		return errPPSHeader
	}
	ppsLen := int(tmpBuf[1])<<8 | int(tmpBuf[2])
	if len(tmpBuf[3:]) < ppsLen || ppsLen <= 0 {
		return errPPSData
	}

	pps = append(pps, startCode...)
	pps = append(pps, tmpBuf[3:]...)

	parser.specificInfo = append(parser.specificInfo, sps...)
	parser.specificInfo = append(parser.specificInfo, pps...)

	return nil
}

func (parser *Parser) isNaluHeader(src []byte) bool {
	if len(src) < naluBytesLen {
		return false
	}
	return src[0] == 0x00 &&
		src[1] == 0x00 &&
		src[2] == 0x00 &&
		src[3] == 0x01
}

func (parser *Parser) naluSize(src []byte) (int, error) {
	if len(src) < naluBytesLen {
		return 0, errors.New("nalusizedata invalid")
	}
	buf := src[:naluBytesLen]
	size := int(0)
	for i := range buf {
		size = size<<8 + int(buf[i])
	}
	return size, nil
}

func (parser *Parser) getAnnexbH264(src []byte, w io.Writer) error {
	dataSize := len(src)
	if dataSize < naluBytesLen {
		return errVideoDataInvalid
	}
	parser.pps.Reset()
	_, err := w.Write(naluAud)
	if err != nil {
		return err
	}

	index := 0
	var state annexbState

	for dataSize > 0 {
		nalLen, err := parser.naluSize(src[index:])
		if err != nil {
			return errDataSizeNotMatch
		}
		index += naluBytesLen
		dataSize -= naluBytesLen
		if dataSize < nalLen || len(src[index:]) < nalLen || nalLen <= 0 {
			return errNaluBodyLen
		}
		if err := parser.writeNalu(w, src[index:index+nalLen], &state); err != nil {
			return err
		}
		index += nalLen
		dataSize -= nalLen
	}
	return nil
}

// annexbState tracks SPS/PPS handling across the NAL units of one AVC packet.
type annexbState struct {
	hasSpsPps      bool
	hasWriteSpsPps bool
}

// writeNalu writes a single NAL unit in Annex B format. SPS and PPS units are buffered
// and written in front of the first IDR slice.
func (parser *Parser) writeNalu(w io.Writer, nalu []byte, state *annexbState) error {
	switch nalu[0] & 0x1f {
	case naluTypeIDR:
		if !state.hasWriteSpsPps {
			state.hasWriteSpsPps = true
			spsPps := parser.specificInfo
			if state.hasSpsPps {
				spsPps = parser.pps.Bytes()
			}
			if _, err := w.Write(spsPps); err != nil {
				return err
			}
		}
		return writeWithStartCode(w, nalu)
	case naluTypeSlice, naluTypeSEI:
		return writeWithStartCode(w, nalu)
	case naluTypeSPS, naluTypePPS:
		state.hasSpsPps = true
		return writeWithStartCode(parser.pps, nalu)
	}
	return nil
}

func writeWithStartCode(w io.Writer, nalu []byte) error {
	if _, err := w.Write(startCode); err != nil {
		return err
	}
	_, err := w.Write(nalu)
	return err
}

func (parser *Parser) Parse(b []byte, isSeq bool, w io.Writer) (err error) {
	switch isSeq {
	case true:
		err = parser.parseSpecificInfo(b)
	case false:
		// is annexb
		if parser.isNaluHeader(b) {
			_, err = w.Write(b)
		} else {
			err = parser.getAnnexbH264(b, w)
		}
	}
	return err
}
