package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
)

func main() {
	server := flag.Bool("server", false, "Run as server and send media (true) or run as client and receive media (false)")
	addr := flag.String("addr", "localhost:8080", "address")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	done := make(chan struct{}, 1)

	go func() {
		if err := run(ctx, *server, *addr); err != nil {
			log.Fatal(err)
		}
	}()

	<-done
}

func run(ctx context.Context, server bool, addr string) error {
	gst.Init(nil)
	defer gst.Deinit()

	if server {
		s, err := newSender(ctx, addr)
		if err != nil {
			return err
		}
		s.start()
		<-ctx.Done()
		return s.Close()
	}
	r, err := newReceiver(ctx, addr)
	if err != nil {
		return err
	}
	r.start()
	<-ctx.Done()
	return r.Close()
}

func handleMessage(msg *gst.Message) error {
	switch msg.Type() {
	case gst.MessageEOS:
		return app.ErrEOS
	case gst.MessageError:
		return msg.ParseError()
	}
	return nil
}
