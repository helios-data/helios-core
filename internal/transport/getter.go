package transport

import (
	"net"

	packet "helios/generated/transport"
)

func (c *CommClient) GetConn() net.Conn {
	return c.conn
}

func (c *CommClient) SetConn(conn net.Conn) {
	c.conn = conn
}

func (c *CommClient) GetRecentPacket() *packet.TransportPacket {
	return c.recentPacket
}

func (c *CommClient) GetCoreOutChan() chan []byte {
	return c.coreOutChan
}

func (c *CommClient) GetCoreInChan() chan []byte {
	return c.coreInChan
}

func (c *CommClient) GetContInChan() chan []byte {
	return c.contInChan
}
