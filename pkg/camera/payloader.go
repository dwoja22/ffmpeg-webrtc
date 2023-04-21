package camera

type Payloader struct {
}

func (p Payloader) Payload(mtu uint16, payload []byte) [][]byte {
	var payloads [][]byte

	//split payload into mtu sized chunks and append them to the payloads array
	for len(payload) > 0 {
		if len(payload) <= int(mtu) {
			payloads = append(payloads, payload)
			break
		}

		payloads = append(payloads, payload[:mtu])
		payload = payload[mtu:]
	}

	return payloads
}
