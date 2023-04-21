package camera

import (
	"bytes"
	"fmt"
)

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
)

func (c *Camera) Payloader(mtu uint16, data []byte) [][]byte {
	var chunks [][]byte

	for i := 0; i < len(data); i++ {
		if i+3 >= len(data) {
			fmt.Println("not enough data to check for start code prefix")
			break
		}

		var prefix string

		//determine if the h264 data is using prefix 1 or prefix 2
		if bytes.Equal(data[i:i+len(PREFIX1)], PREFIX1) {
			prefix = "0x00, 0x00, 0x01"
		}

		if bytes.Equal(data[i:i+len(PREFIX2)], PREFIX2) {
			prefix = "0x00, 0x00, 0x00, 0x01"
		}

		nals := extractNalUnits(data, prefix)

		// split NAL units into chunks
		for _, nal := range nals {
			if uint16(len(nal)) <= mtu {
				chunks = append(chunks, nal)
			} else {
				chunks = append(chunks, splitNALUnit(mtu, nal)...)
			}
		}
	}

	return chunks
}

func extractNalUnits(data []byte, prefix string) [][]byte {
	var start int
	var nals [][]byte

	if prefix == "0x00, 0x00, 0x01" && len(data) < 4 || prefix == "0x00, 0x00, 0x00, 0x01" && len(data) < 5 {
		fmt.Println("not enough data to extract nal unit")
		return nil
	}

	var p []byte

	if prefix == "0x00, 0x00, 0x01" {
		p = PREFIX1
	}

	if prefix == "0x00, 0x00, 0x00, 0x01" {
		p = PREFIX2
	}

	for i := 0; i < len(data)-len(p); i++ {
		//found start code prefix of nal unit
		if bytes.Equal(data[i:i+len(p)], p) {
			if i > start {
				//TODO:: check if data[start:i-2] depends on the start code prefix
				nals = append(nals, data[start:i-2])
			}
			start = i
		}
	}

	return nil
}

func splitNALUnit(mtu uint16, nalUnit []byte) [][]byte {
	// function to split a NAL unit into chunks of size mtu
	// chunks must be complete NAL units

	var chunks [][]byte

	// split NAL unit into chunks
	for i := 0; i < len(nalUnit); i += int(mtu) {
		if i+int(mtu) < len(nalUnit) {
			chunks = append(chunks, nalUnit[i:i+int(mtu)])
		} else {
			chunks = append(chunks, nalUnit[i:])
		}
	}

	return chunks
}

func checkNalUnitType(data []byte, prefix string) {
	if prefix == "0x00, 0x00, 0x01" && len(data) < 4 || prefix == "0x00, 0x00, 0x00, 0x01" && len(data) < 5 {
		fmt.Println("not enough data to check for nal unit type")
		return
	}

	var nalType byte
	var nalRefIdc byte

	if prefix == "0x00, 0x00, 0x01" {
		//check for nal unit type
		//format is using prefix 1, so we need to skip 3 bytes to get to the nal unit type
		//the nal unit type is the last 5 bits of the byte, so we need to mask the byte with 0x1F, which is 0001 1111
		//the bitwise AND operator will return the last 5 bits of the byte, nalType will be a value between 0 and 31
		//we are using 0001 1111 because the nal unit type can have a maximum value of 31, 11111 is 31 in binary with 000 masking the first 3 bits
		nalType = data[3] & 0x1F

		//check for nal ref idc
		//the nal ref idc is the last 2 bits of the byte, so we need to mask the byte with 0x03, which is 0000 0011
		//the bitwise AND operator will return the last 2 bits of the byte, nalRefIdc will be a value between 0 and 3
		nalRefIdc = (data[3] >> 5) & 0x03
	}

	if prefix == "0x00, 0x00, 0x00, 0x01" {
		nalType = data[4] & 0x1F
		nalRefIdc = (data[4] >> 5) & 0x03
	}

	isType(nalType, nalRefIdc)
}

func isType(nalType byte, nalRefIdc byte) {
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
