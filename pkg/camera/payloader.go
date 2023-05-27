package camera

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

//TODO:: improve the efficiency of this payloader
type Payloader struct {
	SPS []byte
	PPS []byte
}

func NewPayloader() *Payloader {
	return &Payloader{}
}

//define h264 nal unit types and start code prefixes according to https://tools.ietf.org/html/rfc6184
var PREFIX1 = []byte{0x00, 0x00, 0x01}
var PREFIX2 = []byte{0x00, 0x00, 0x00, 0x01}

const (
	NALU_TYPE_SLICE = 1
	NALU_TYPE_DPA   = 2
	NALU_TYPE_DPB   = 3
	NALU_TYPE_DPC   = 4
	NALU_TYPE_IDR   = 5
	NALU_TYPE_SEI   = 6
	NALU_TYPE_SPS   = 7
	NALU_TYPE_PPS   = 8
	NALU_TYPE_AUD   = 9
	NALU_TYPE_EOSEQ = 10
	NALU_TYPE_EOSTR = 11
	NALU_TYPE_FILL  = 12

	NALU_TYPE_STAPA = 24
	NALU_TYPE_FUA   = 28
	NALU_TYPE_FUB   = 29

	STAP_A_HEADER = 0x78
	NAL_REF_IDC   = 0x60

	FUA_HEADER_SIZE = 2
)

func (p *Payloader) Payload(mtu uint16, data []byte) [][]byte {
	var prefix int
	var payloads [][]byte

	for i := 0; i < len(data); i++ {
		//determine if the h264 data is using prefix 1 or prefix 2
		if bytes.Equal(data[i:i+len(PREFIX1)], PREFIX1) {
			prefix = 3
			break
		}

		if bytes.Equal(data[i:i+len(PREFIX2)], PREFIX2) {
			prefix = 4
			break
		}
	}

	nals := extractNalUnits(data, prefix)

	for _, nal := range nals {
		if len(nal) == 0 {
			continue
		}

		naltype := nal[0] & 0x1F
		//this is the priority of the nal unit, possible values are 0, 32, 64, 96 which are mapped to 0, 1, 2, 3 and are used to determine the order of the nal units in the stream
		nalRefIdc := nal[0] & 0x60

		if naltype == NALU_TYPE_SPS {
			p.SPS = nal
			continue
		}

		if naltype == NALU_TYPE_PPS {
			p.PPS = nal
			continue
		}

		if naltype == NALU_TYPE_AUD || naltype == NALU_TYPE_FILL {
			continue
		}

		if p.SPS != nil && p.PPS != nil {
			spsLen := make([]byte, 2)
			binary.BigEndian.PutUint16(spsLen, uint16(len(p.SPS)))

			ppsLen := make([]byte, 2)
			binary.BigEndian.PutUint16(ppsLen, uint16(len(p.PPS)))

			stapANalu := []byte{STAP_A_HEADER}
			stapANalu = append(stapANalu, spsLen...)
			stapANalu = append(stapANalu, p.SPS...)
			stapANalu = append(stapANalu, ppsLen...)
			stapANalu = append(stapANalu, p.PPS...)

			if len(stapANalu) <= int(mtu) {
				nalOut := make([]byte, len(stapANalu))
				copy(nalOut, stapANalu)
				payloads = append(payloads, nalOut)
			}

			p.SPS = nil
			p.PPS = nil
		}

		if len(nal) <= int(mtu) {
			nalOut := make([]byte, len(nal))
			copy(nalOut, nal)
			payloads = append(payloads, nalOut)
			continue
		}

		//Package as STAP-A, non-interleaved
		//Nal unit is too big to fit into a single RTP packet and needs to be split into multiple FU-A packets
		//FU-A header is 2 bytes long
		maxFragmentSize := int(mtu) - FUA_HEADER_SIZE

		nalData := nal
		nalDataIndex := 1
		nalDataLength := len(nal) - nalDataIndex
		nalDataRemaining := nalDataLength

		if min(maxFragmentSize, nalDataRemaining) <= 0 {
			continue
		}

		for nalDataRemaining > 0 {
			currentFragmentSize := min(maxFragmentSize, nalDataRemaining)
			nalOut := make([]byte, currentFragmentSize+FUA_HEADER_SIZE)

			//set the FU indicator
			nalOut[0] = NALU_TYPE_FUA
			//set the NRI, which is the priority of the nal unit
			nalOut[0] |= nalRefIdc
			//set the type of the nal unit
			nalOut[1] = naltype

			if nalDataRemaining == nalDataLength {
				//set the start bit
				nalOut[1] |= 1 << 7
			} else if nalDataRemaining-currentFragmentSize == 0 {
				//set the end bit
				nalOut[1] |= 1 << 6
			}

			copy(nalOut[FUA_HEADER_SIZE:], nalData[nalDataIndex:nalDataIndex+currentFragmentSize])
			payloads = append(payloads, nalOut)

			nalDataRemaining -= currentFragmentSize
			nalDataIndex += currentFragmentSize
		}
	}

	return payloads
}

func extractNalUnits(data []byte, prefix int) [][]byte {
	var start int
	var nals [][]byte

	//check if we have enough data to check for start code prefix
	if prefix == 3 && len(data) < 4 || prefix == 4 && len(data) < 5 {
		return nil
	}

	var p []byte

	if prefix == 3 {
		p = PREFIX1
	}

	if prefix == 4 {
		p = PREFIX2
	}

	for i := 0; i < len(data); i++ {
		if i+prefix >= len(data) {
			break
		}

		//found start code prefix of nal unit
		if bytes.Equal(data[i:i+prefix], p) {
			if i > start {
				nals = append(nals, data[start+prefix:i])
			}

			start = i
		}
	}

	//check if we have any remaining data
	if start+prefix < len(data) {
		nals = append(nals, data[start+prefix:])
	}

	return nals
}

//TODO:: does not work as expected, ffprobe output and mine are different, fix this
//function checks what type of nal units the data contains
func nalType(data []byte) {
	//check for nal unit type
	//the nal unit type is the last 5 bits of the byte, so we need to mask the byte with 0x1F, which is 0001 1111
	//the bitwise AND operator will return the last 5 bits of the byte, nalType will be a value between 0 and 31
	//we are using 0001 1111 because the nal unit type can have a maximum value of 31, 11111 is 31 in binary with 000 masking the first 3 bits

	//check for nal ref idc
	//the nal ref idc is the last 2 bits of the byte, so we need to mask the byte with 0x03, which is 0000 0011
	//the bitwise AND operator will return the last 2 bits of the byte, nalRefIdc will be a value between 0 and 3

	nalRefIdc := (data[0] >> 5) & 0x03
	nalType := data[0] & 0x1F

	switch nalType {
	case NALU_TYPE_SPS:
		fmt.Println("found SPS")
	case NALU_TYPE_PPS:
		fmt.Println("found PPS")
	case NALU_TYPE_IDR:
		fmt.Println("found IDR")
	case NALU_TYPE_SEI:
		fmt.Println("found SEI")
	case NALU_TYPE_DPA:
		fmt.Println("found DPA")
	case NALU_TYPE_DPB:
		fmt.Println("found DPB")
	case NALU_TYPE_DPC:
		fmt.Println("found DPC")
	case NALU_TYPE_SLICE:
		fmt.Println("found slice")
		if nalRefIdc == 0 {
			fmt.Println("slice is (B frame)")
		} else if nalRefIdc == 1 {
			fmt.Println("slice is (P frame)")
		} else if nalRefIdc == 2 {
			fmt.Println("slice is (I frame)")
		} else {
			fmt.Println("unknown nal ref idc")
		}
	case NALU_TYPE_AUD:
		fmt.Println("found AUD")
	case NALU_TYPE_EOSEQ:
		fmt.Println("found EOSEQ")
	case NALU_TYPE_EOSTR:
		fmt.Println("found EOSTR")
	case NALU_TYPE_FILL:
		fmt.Println("found FILL")
	default:
		fmt.Println("found unknown nal unit type with value: ", nalType, " and nal ref idc: ", nalRefIdc)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
