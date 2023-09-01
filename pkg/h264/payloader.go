package h264

import (
	"encoding/binary"
	"fmt"
)

type Payloader struct {
	SPS []byte
	PPS []byte
}

func NewPayloader() *Payloader {
	return &Payloader{}
}

const (
	NALU_TYPE_P     = 1
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

	//0x78 = 01111000
	//0 maps to the F bit, 11 maps to NRI, 11000 maps to type 24
	//0, 11, 11000 = 0, 3, 24, false, highest priority, stap-a nal type id
	STAP_A_HEADER = 0x78
	NAL_REF_IDC   = 0x60

	FUA_HEADER_SIZE = 2
)

//findNal finds the start code prefix of a nal unit and returns the start index and length of the prefix
//-1 indicates that the nal is at the beginning of the data
//example
//0 0 0 1 - prefixLength = 4
//0 1 2 - zero count as we iterate through the data
//0 + 2 - 3 = -1, 3 + 1 = 4 - nal at the beginning of the data, represented by -1, 4 for the prefix length
func findNal(data []byte, start int) (prefixStart, prefixLength int) {
	zeros := 0

	for i, b := range data[start:] {
		if b == 0 {
			zeros++
			continue
		} else if b == 1 {
			if zeros >= 2 { //make sure we have at least 2 zeros, otherwise it's just a random 1
				return start + i - zeros, zeros + 1
			}
		}

		//reset the counter to start counting zeros again for finding the next nal
		zeros = 0
	}
	return -1, -1
}

//extractNalUnits extracts all nal units from the data, works with both single and multiple nal units
//as well as with prefix 0 0 1 and 0 0 0 1 or a mix of both
func extractNalUnits(data []byte, extractNal func([]byte)) {
	nalStart, prefixLength := findNal(data, 0)

	//single nal unit or nal at the start of the data
	if nalStart == -1 {
		extractNal(data)
	} else {
		for nalStart != -1 {
			prevNalStart := nalStart + prefixLength
			//find the next nal unit
			nalStart, prefixLength = findNal(data, prevNalStart)
			if nalStart != -1 {
				extractNal(data[prevNalStart:nalStart])
			} else {
				//nal unit is at the end of the data
				extractNal(data[prevNalStart:])
			}
		}
	}
}

func (p *Payloader) Payload(mtu uint16, data []byte) [][]byte {
	var payloads [][]byte

	extractNalUnits(data, func(nal []byte) {
		if len(nal) == 0 {
			return
		}

		naltype := nal[0] & 0x1F
		//this is the NRI (nal reference index) and is the priority of the nal unit, possible values are 0, 1, 2, 3 and are used to determine the priority of the nal
		nalRefIdc := nal[0] & 0x60

		if naltype == NALU_TYPE_SPS {
			p.SPS = nal
			return
		}

		if naltype == NALU_TYPE_PPS {
			p.PPS = nal
			return
		}

		if naltype == NALU_TYPE_AUD || naltype == NALU_TYPE_FILL {
			return
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

			return
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
			return
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
	})

	return payloads
}

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
	case NALU_TYPE_P:
		fmt.Println("found P")
	case NALU_TYPE_IDR:
		fmt.Println("found I")
	case NALU_TYPE_SEI:
		fmt.Println("found SEI")
	case NALU_TYPE_DPA:
		fmt.Println("found DPA")
	case NALU_TYPE_DPB:
		fmt.Println("found DPB")
	case NALU_TYPE_DPC:
		fmt.Println("found DPC")
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
