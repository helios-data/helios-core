package transport

import (
	"google.golang.org/protobuf/proto"

	packet "helios/generated/transport"
)

func CreateTransportPacket(id int32, address string, data []byte) (*packet.TransportPacket, error) {
	pkt := &packet.TransportPacket{
		Id:      id,
		Address: address,
		Data:    data,
	}
	return pkt, nil
}

func MarshalTransportPacket(pkt *packet.TransportPacket) ([]byte, error) {
	data, err := proto.Marshal(pkt)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func UnmarshalTransportPacket(data []byte) (*packet.TransportPacket, error) {
    pkt := &packet.TransportPacket{}
    if err := proto.Unmarshal(data, pkt); err != nil {
        return nil, err
    }
    return pkt, nil
}
