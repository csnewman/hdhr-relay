package main

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"time"
)

type Remote struct {
	ID         uint32
	Type       uint32
	TunerCount uint8
	Lineup     string
	Base       string
	Auth       string
}

func Discover(addr string) (*Remote, error) {
	log.Println("Discovering upstream", addr)

	remote, err := net.ResolveUDPAddr("udp4", addr+":"+strconv.Itoa(DiscoveryPort))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve remote address: %v", err)
	}

	log.Println("upstream", remote)

	local, err := net.ResolveUDPAddr("udp4", ":")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local address: %v", err)
	}

	listen, err := net.ListenUDP("udp4", local)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	defer listen.Close()

	con, err := NewUDPCon(listen)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %v", err)
	}

	w := NewWriter()
	w.WriteU8(DeviceType)
	w.WriteVarLen(4)
	w.WriteU32(TypeTuner)

	packet := Frame(DiscoveryRequest, w.Data.Bytes())

	if err := con.Write(packet, remote); err != nil {
		return nil, err
	}

	recv := make([]byte, 4096)

	con.con.SetReadDeadline(time.Now().Add(time.Second))

	in, err := con.Read(recv, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %v", err)
	}

	ty, data, err := UnFrame(in.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to unframe: %v", err)
	}

	if ty != DiscoveryResponse {
		return nil, fmt.Errorf("unexpected response: %v", err)
	}

	result := &Remote{}

	r := NewReader(data)

	for r.HasMore() {
		tag, err := r.ReadU8()
		if err != nil {
			return nil, fmt.Errorf("failed to read tag: %v", err)
		}

		l, err := r.ReadVarLen()
		if err != nil {
			return nil, fmt.Errorf("failed to read len: %v", err)
		}

		start := r.Pos

		switch tag {
		case DeviceType:
			ty, err := r.ReadU32()
			if err != nil {
				return nil, fmt.Errorf("failed to read tag data: %v", err)
			}

			result.Type = ty

		case DeviceID:
			ty, err := r.ReadU32()
			if err != nil {
				return nil, fmt.Errorf("failed to read tag data: %v", err)
			}

			result.ID = ty

		case TunerCount:
			c, err := r.ReadU8()
			if err != nil {
				return nil, fmt.Errorf("failed to read tag data: %v", err)
			}

			result.TunerCount = c

		case LineupURL:
			a, err := r.ReadStr(int(l))
			if err != nil {
				return nil, fmt.Errorf("failed to read tag data: %v", err)
			}

			result.Lineup = a

		case BaseURL:
			a, err := r.ReadStr(int(l))
			if err != nil {
				return nil, fmt.Errorf("failed to read tag data: %v", err)
			}

			result.Base = a

		case DeviceAuth:
			a, err := r.ReadStr(int(l))
			if err != nil {
				return nil, fmt.Errorf("failed to read tag data: %v", err)
			}

			result.Auth = a

		default:
			log.Println("unknown tag from upstream", tag)
		}

		r.Pos = start + int(l)
	}

	return result, nil
}
