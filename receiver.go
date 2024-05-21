package main

import (
	"context"
	"crypto/tls"
	"log"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/mengelbart/roq"
	"github.com/quic-go/quic-go"
)

type receiver struct {
	pipeline  *gst.Pipeline
	session   *roq.Session
	ctx       context.Context
	cancelCtx context.CancelFunc
}

func newReceiver(ctx context.Context, addr string) (*receiver, error) {
	session, err := createRoQClient(ctx, addr)
	if err != nil {
		return nil, err
	}
	f, err := session.NewReceiveFlow(0)
	if err != nil {
		return nil, err
	}
	pipeline, err := createReceivePipeline(f)
	if err != nil {
		return nil, err
	}
	ctx, cancelCtx := context.WithCancel(context.Background())
	r := &receiver{
		pipeline:  pipeline,
		session:   session,
		ctx:       ctx,
		cancelCtx: cancelCtx,
	}
	return r, nil
}

func (r *receiver) start() error {
	bus := r.pipeline.GetPipelineBus()
	go func() {
		<-r.ctx.Done()
		r.pipeline.SendEvent(gst.NewEOSEvent())
	}()
	r.pipeline.SetState(gst.StatePlaying)
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

func (r *receiver) Close() error {
	r.cancelCtx()
	return nil
}

func createRoQClient(ctx context.Context, addr string) (*roq.Session, error) {
	conn, err := quic.DialAddr(ctx, addr, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"roq-10"},
	}, &quic.Config{EnableDatagrams: true})
	if err != nil {
		return nil, err
	}
	return roq.NewSession(roq.NewQUICGoConnection(conn), true)
}

func createReceivePipeline(flow *roq.ReceiveFlow) (*gst.Pipeline, error) {
	gst.Init(nil)
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, err
	}
	elements, err := gst.NewElementMany("rtpjitterbuffer", "rtpvp8depay", "vp8dec", "videoconvert", "clocksync", "autovideosink")
	if err != nil {
		return nil, err
	}
	src, err := app.NewAppSrc()
	if err != nil {
		return nil, err
	}
	src.SetCaps(gst.NewCapsFromString("application/x-rtp,media=(string)video,clock-rate=(int)90000,encoding-name=(string)VP8,payload=(int)96"))
	pipeline.AddMany(append([]*gst.Element{src.Element}, elements...)...)
	gst.ElementLinkMany(append([]*gst.Element{src.Element}, elements...)...)

	buf := make([]byte, 64_000)
	src.SetCallbacks(&app.SourceCallbacks{
		NeedDataFunc: func(self *app.Source, length uint) {
			n, err := flow.Read(buf)
			if err != nil {
				src.EndStream()
				return
			}
			log.Printf("read %v bytes\n", n)
			buffer := gst.NewBufferWithSize(int64(n))
			buffer.Map(gst.MapWrite).WriteData(buf[:n])
			buffer.Unmap()

			self.PushBuffer(buffer)
		},
	})
	return pipeline, nil
}
