package main

import (
	"fmt"
	"net/http"

	_ "net/http/pprof"

	"github.com/pion/ion/biz"
	"github.com/pion/ion/conf"
	"github.com/pion/ion/discovery"
	"github.com/pion/ion/log"
	"github.com/pion/ion/rtc"
	"github.com/pion/ion/signal"
)

var (
	ionID = fmt.Sprintf("%s:%d", conf.Global.Addr, conf.Rtp.Port)
)

func init() {
	log.Init(conf.Log.Level)
	biz.Init(ionID, conf.Amqp.URL)
	rtc.Init(conf.Rtp.Port, conf.WebRTC.ICE)
	signal.Init(conf.Signal.Host, conf.Signal.Port, conf.Signal.Cert, conf.Signal.Key, biz.BizEntry)
	discovery.Init(conf.Global.Addr, conf.Rtp.Port, conf.Etcd.Addrs)
}

func close() {
	biz.Close()
}

func main() {
	defer close()
	if conf.Global.Pprof != "" {
		go func() {
			log.Infof("Start pprof on %s", conf.Global.Pprof)
			http.ListenAndServe(conf.Global.Pprof, nil)
		}()
	}

	select {}
}
