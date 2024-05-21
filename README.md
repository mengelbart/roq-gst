# RTP over QUIC / Gstreamer / Go

This is an example application using [RTP over
QUIC](https://github.com/mengelbart/roq) and
[Gstreamer](https://gstreamer.freedesktop.org/) in [Go](https://go.dev/).

To build the example you need to install Go, Gstreamer, and a C compiler and run
`go build`. After successfully building the application, you can start a server
with `./roq-gst -server`. In a different terminal, run `./rog-gst` to start a
client. You should see a test video being transmitted from the server to the
client.

