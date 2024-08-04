package main

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
)

const (
	DiscoveryPort     = 65001
	ContentPort       = 5004
	WebPort           = 80
	DiscoveryRequest  = 0x2
	DiscoveryResponse = 0x3
	DeviceType        = 0x1
	DeviceID          = 0x2
	TunerCount        = 0x10
	LineupURL         = 0x27
	BaseURL           = 0x2a
	DeviceAuth        = 0x2b
	MultiType         = 0x2d
	TypeTuner         = 0x1
	TypeWildcard      = 0xFFFFFFFF
	TypeStorage       = 0x5
)

func main() {
	log.Println("HDHR Relay")

	if len(os.Args) != 3 {
		log.Println("Usage: hdhr-relay <target> <self>")

		return
	}

	target := os.Args[1]
	self := os.Args[2]

	log.Println("Target:", target)
	log.Println("Self:", self)

	ctx := context.Background()

	r := &Relay{
		Target: target,
		Self:   self,
	}

	if err := r.Run(ctx); err != nil {
		log.Println("Error:", err)

		os.Exit(1)
	}
}

type Relay struct {
	Target          string
	Self            string
	discoverySocket *UDPCon
}

func (r *Relay) Run(ctx context.Context) error {
	dUDP, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: DiscoveryPort,
	})
	if err != nil {
		return fmt.Errorf("failed to listen on discovery port: %v", err)
	}

	r.discoverySocket, err = NewUDPCon(dUDP)
	if err != nil {
		return fmt.Errorf("failed to create UDP socket: %v", err)
	}

	g, gctx := errgroup.WithContext(ctx)

	proxyBase := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   r.Target,
	})

	httpBase := &http.Server{
		Addr: "0.0.0.0:" + strconv.Itoa(WebPort),
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/discover.json" &&
				request.URL.Path != "/lineup.json" &&
				request.URL.Path != "/lineup_status.json" {
				log.Println("blocking request", request.URL.Path)

				return
			}

			log.Println("proxying base", request.URL)

			proxyBase.ServeHTTP(writer, request)
		}),
	}

	proxyContent := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   r.Target + ":" + strconv.Itoa(ContentPort),
	})

	httpContent := &http.Server{
		Addr: "0.0.0.0:" + strconv.Itoa(ContentPort),
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			log.Println("proxying content", request.URL)

			proxyContent.ServeHTTP(writer, request)
		}),
	}

	g.Go(func() error {
		return r.handleDiscovery(gctx)
	})

	g.Go(httpBase.ListenAndServe)
	g.Go(httpContent.ListenAndServe)

	return g.Wait()
}

func (r *Relay) handleDiscovery(ctx context.Context) error {
	buffer := make([]byte, 4096)
	oob := make([]byte, 4096)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		in, err := r.discoverySocket.Read(buffer, oob)
		if err != nil {
			log.Println("Error: reading from UDP address:", err)

			continue
		}

		ty, data, err := UnFrame(in.Data)
		if err != nil {
			log.Println("Error: parsing packet from", in.Src, ":", err)

			continue
		}

		rdata := NewReader(data)

		switch ty {
		case DiscoveryRequest:
			if err := r.discoveryRequest(in, rdata); err != nil {
				log.Println("Error: discovery request:", err)
			}

		default:
			log.Println("Error: unknown packet type:", ty)
		}
	}
}

func (r *Relay) discoveryRequest(in *UDPIn, data *Reader) error {
	ty, err := data.ReadU8()
	if err != nil {
		return err
	}

	wantTuner := false

	switch ty {
	case DeviceType:
		vl, err := data.ReadVarLen()
		if err != nil {
			return err
		}

		if vl != 4 {
			return fmt.Errorf("invalid length: %v", vl)
		}

		dty, err := data.ReadU32()
		if err != nil {
			return err
		}

		wantTuner = dty == TypeTuner || dty == TypeWildcard

	case MultiType:
		vl, err := data.ReadVarLen()
		if err != nil {
			return err
		}

		if vl%4 != 0 {
			return fmt.Errorf("invalid length: %v", vl)
		}

		for i := 0; i < int(vl); i += 4 {
			dty, err := data.ReadU32()
			if err != nil {
				return err
			}

			if dty == TypeTuner || dty == TypeWildcard {
				wantTuner = true
			}
		}
	default:
		return fmt.Errorf("unknown discovery type: %v", ty)
	}

	if !wantTuner {
		log.Println("ignoring discovery request")

		return nil
	}

	log.Println("Responding to tuner request from", in.Src, "to", in.Dst)

	upstream, err := Discover(r.Target)
	if err != nil {
		log.Println("Failed to query upstream:", err)

		upstream = &Remote{}
		upstream.Type = TypeTuner
		upstream.ID = 1234
		upstream.TunerCount = 1
	}

	log.Println("Fetched discovery packet from upstream")

	w := NewWriter()
	w.WriteU8(DeviceType)
	w.WriteVarLen(4)
	w.WriteU32(upstream.Type)

	w.WriteU8(DeviceID)
	w.WriteVarLen(4)
	w.WriteU32(upstream.ID)

	auth := []byte(upstream.Auth)

	if len(auth) > 0 {
		w.WriteU8(DeviceAuth)
		w.WriteVarLen(uint16(len(auth)))
		w.WriteBlob(auth)
	}

	newBase := "http://" + r.Self + ":" + strconv.Itoa(WebPort)

	base := []byte(newBase)

	w.WriteU8(BaseURL)
	w.WriteVarLen(uint16(len(base)))
	w.WriteBlob(base)

	w.WriteU8(TunerCount)
	w.WriteVarLen(1)
	w.WriteU8(upstream.TunerCount)

	lineup := []byte(newBase + "/lineup.json")
	w.WriteU8(LineupURL)
	w.WriteVarLen(uint16(len(lineup)))
	w.WriteBlob(lineup)

	packet := Frame(DiscoveryResponse, w.Data.Bytes())

	if err := r.discoverySocket.Write(packet, in.Src); err != nil {
		return err
	}

	return nil
}
