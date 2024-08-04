package main

import (
	"fmt"
	"golang.org/x/sys/unix"
	"log"
	"net"
	"net/netip"
)

type UDPCon struct {
	con *net.UDPConn
}

func NewUDPCon(con *net.UDPConn) (*UDPCon, error) {
	raw, err := con.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw socket: %v", err)
	}

	if err := raw.Control(func(fd uintptr) {
		if err := unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_PKTINFO, 1); err != nil {
			log.Println("failed to configure IP_PKTINFO:", err)
		}

		if err := unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_RECVPKTINFO, 1); err != nil {
			log.Println("failed to configure IPV6_RECVPKTINFO:", err)
		}
	}); err != nil {
		return nil, fmt.Errorf("failed to control raw socket: %v", err)
	}

	return &UDPCon{con: con}, nil
}

type UDPIn struct {
	Data []byte
	Src  *net.UDPAddr
	Dst  netip.Addr
}

func (c *UDPCon) Read(buffer []byte, oob []byte) (*UDPIn, error) {
	n, oobn, _, addr, err := c.con.ReadMsgUDP(buffer, oob)
	if err != nil {
		return nil, err
	}

	buffer = buffer[:n]
	oob = oob[:oobn]

	var ip netip.Addr

	for len(oob) > 0 {
		hdr, body, remainder, err := unix.ParseOneSocketControlMessage(oob)
		if err != nil {
			return nil, err
		}

		oob = remainder

		if hdr.Level == unix.IPPROTO_IP && hdr.Type == unix.IP_PKTINFO && len(body) >= 12 {
			ip = netip.AddrFrom4(*(*[4]byte)(body[8:12]))
		} else if hdr.Level == unix.IPPROTO_IPV6 && hdr.Type == unix.IPV6_PKTINFO && len(body) >= 16 {
			ip = netip.AddrFrom16(*(*[16]byte)(body[:16])).Unmap()
		}
	}

	return &UDPIn{
		Data: buffer,
		Src:  addr,
		Dst:  ip,
	}, nil
}

func (c *UDPCon) Write(buffer []byte, addr *net.UDPAddr) error {
	_, err := c.con.WriteToUDP(buffer, addr)

	return err
}
