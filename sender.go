package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log"
	"math/big"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/mengelbart/roq"
	"github.com/quic-go/quic-go"
)

type sender struct {
	pipeline  *gst.Pipeline
	session   *roq.Session
	ctx       context.Context
	cancelCtx context.CancelFunc
}

func newSender(ctx context.Context, addr string) (*sender, error) {
	session, err := createRoQSenderServer(ctx, addr)
	if err != nil {
		return nil, err
	}
	f, err := session.NewSendFlow(0)
	if err != nil {
		return nil, err
	}
	pipeline, err := createSendPipeline(f)
	if err != nil {
		return nil, err
	}
	ctx, cancelCtx := context.WithCancel(context.Background())
	s := &sender{
		pipeline:  pipeline,
		session:   session,
		ctx:       ctx,
		cancelCtx: cancelCtx,
	}
	return s, nil
}

func (s *sender) start() error {
	bus := s.pipeline.GetPipelineBus()
	go func() {
		<-s.ctx.Done()
		s.pipeline.SendEvent(gst.NewEOSEvent())
	}()
	s.pipeline.SetState(gst.StatePlaying)
	for {
		msg := bus.TimedPop(gst.ClockTimeNone)
		if msg == nil {
			break
		}
		if err := handleMessage(msg); err != nil {
			return err
		}
	}
	return nil
}

func (s *sender) Close() error {
	s.cancelCtx()
	return nil
}

func createSendPipeline(flow *roq.SendFlow) (*gst.Pipeline, error) {
	gst.Init(nil)
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, err
	}
	elements, err := gst.NewElementMany("videotestsrc", "clocksync", "vp8enc", "rtpvp8pay")
	if err != nil {
		return nil, err
	}
	err = elements[len(elements)-1].SetProperty("mtu", uint(64000))
	if err != nil {
		return nil, err
	}
	sink, err := app.NewAppSink()
	if err != nil {
		return nil, err
	}
	pipeline.AddMany(append(elements, sink.Element)...)
	gst.ElementLinkMany(append(elements, sink.Element)...)

	sink.SetCallbacks(&app.SinkCallbacks{
		NewSampleFunc: func(sink *app.Sink) gst.FlowReturn {
			sample := sink.PullSample()
			if sample == nil {
				return gst.FlowEOS
			}
			buffer := sample.GetBuffer()
			if buffer == nil {
				return gst.FlowError
			}
			samples := buffer.Map(gst.MapRead).AsUint8Slice()
			defer buffer.Unmap()

			stream, err := flow.NewSendStream(context.Background())
			if err != nil {
				return gst.FlowError
			}
			n, err := stream.WriteRTPBytes(samples)
			if err != nil {
				return gst.FlowError
			}
			log.Printf("buffer len: %v, written: %v\n", len(samples), n)

			err = stream.Close()
			if err != nil {
				return gst.FlowError
			}
			return gst.FlowOK
		},
	})
	return pipeline, nil
}

func createRoQSenderServer(ctx context.Context, addr string) (*roq.Session, error) {
	tlsConfig := generateTLSConfig()
	listener, err := quic.ListenAddr(addr, tlsConfig, &quic.Config{EnableDatagrams: true})
	if err != nil {
		return nil, err
	}
	conn, err := listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	return roq.NewSession(roq.NewQUICGoConnection(conn), true)
}

// Setup a bare-bones TLS config for the server
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{tlsCert},
		NextProtos:         []string{"roq-10"},
	}
}
