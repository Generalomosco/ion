package media

import (
	"io"

	"github.com/pion/ion/conf"
	"github.com/pion/ion/log"
	"github.com/pion/ion/media/samplebuilder"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v2"
)

var defaultPeerCfg = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.stunprotocol.org:3478"},
		},
	},
}

const (
	// The amount of RTP packets it takes to hold one full video frame
	// The MTU of ~1400 meant that one video buffer had to be split across 7 packets
	averageRtpPacketsPerKeyFrame = 100
)

type WebRTCEngine struct {
	// PeerConnection config
	cfg webrtc.Configuration

	// Media engine
	mediaEngine webrtc.MediaEngine

	// API object
	api *webrtc.API
}

func NewWebRTCEngine() *WebRTCEngine {
	urls := conf.SFU.Ices

	w := &WebRTCEngine{
		mediaEngine: webrtc.MediaEngine{},
		cfg: webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: urls,
				},
			},
		},
	}
	w.mediaEngine.RegisterCodec(webrtc.NewRTPVP8Codec(webrtc.DefaultPayloadTypeVP8, 90000))
	w.mediaEngine.RegisterCodec(webrtc.NewRTPOpusCodec(webrtc.DefaultPayloadTypeOpus, 48000))
	w.api = webrtc.NewAPI(webrtc.WithMediaEngine(w.mediaEngine))
	return w
}

func (s WebRTCEngine) CreateSender(offer webrtc.SessionDescription, p *WebRTCPeer, addVideoTrack, addAudioTrack **webrtc.Track) (answer webrtc.SessionDescription, err error) {
	p.PC, err = s.api.NewPeerConnection(s.cfg)
	log.Infof("WebRTCEngine.CreateSender pc=%p", p.PC)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	//track is ready
	if *addVideoTrack != nil && *addAudioTrack != nil {
		p.PC.AddTrack(*addVideoTrack)
		p.PC.AddTrack(*addAudioTrack)
		err = p.PC.SetRemoteDescription(offer)
		if err != nil {
			return webrtc.SessionDescription{}, err
		}
	}

	answer, err = p.PC.CreateAnswer(nil)
	err = p.PC.SetLocalDescription(answer)
	log.Infof("WebRTCEngine.CreateSender ok")
	return answer, err
}

func (s WebRTCEngine) CreateReceiver(offer webrtc.SessionDescription, p *WebRTCPeer) (answer webrtc.SessionDescription, err error) {
	p.PC, err = s.api.NewPeerConnection(s.cfg)
	log.Infof("WebRTCEngine.CreateReceiver pc=%p", p.PC)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	_, err = p.PC.AddTransceiver(webrtc.RTPCodecTypeVideo)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	_, err = p.PC.AddTransceiver(webrtc.RTPCodecTypeAudio)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	p.PC.OnTrack(func(remoteTrack *webrtc.Track, receiver *webrtc.RTPReceiver) {
		if remoteTrack.PayloadType() == webrtc.DefaultPayloadTypeVP8 ||
			remoteTrack.PayloadType() == webrtc.DefaultPayloadTypeVP9 ||
			remoteTrack.PayloadType() == webrtc.DefaultPayloadTypeH264 {
			p.VideoTrack, err = p.PC.NewTrack(remoteTrack.PayloadType(), remoteTrack.SSRC(), "video", remoteTrack.Label())

			go func() {
				for {
					select {
					case <-p.Pli:
						p.PC.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: remoteTrack.SSRC()}})
					case gapSeq := <-p.GapSeq:
						// TODO
						p.GapSeqInc(gapSeq)
						if p.GetGapSeqCount(gapSeq) >= 1 {
							t := rtcp.TransportLayerNack{MediaSSRC: remoteTrack.SSRC(), Nacks: []rtcp.NackPair{{PacketID: gapSeq}}}
							log.Infof("p.PC.WriteRTCP gapSeq=%d", gapSeq)
							p.PC.WriteRTCP([]rtcp.Packet{&t})
							p.DelGapSeq(gapSeq)
						}

					case <-p.Stop:
						return
					}
				}
			}()
			s.jitterBuffer(remoteTrack, p.VideoTrack, p.Stop, p.GapSeq)
		} else {
			p.AudioTrack, err = p.PC.NewTrack(remoteTrack.PayloadType(), remoteTrack.SSRC(), "audio", remoteTrack.Label())
			s.jitterBuffer(remoteTrack, p.AudioTrack, p.Stop, p.GapSeq)
		}
	})

	err = p.PC.SetRemoteDescription(offer)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	answer, err = p.PC.CreateAnswer(nil)
	err = p.PC.SetLocalDescription(answer)
	log.Infof("WebRTCEngine.CreateReceiver ok")
	return answer, err
}

func (s WebRTCEngine) jitterBuffer(remoteTrack, localTrack *webrtc.Track, stop chan int, gapSeq chan uint16) {
	var pkt rtp.Depacketizer
	if remoteTrack.PayloadType() == webrtc.DefaultPayloadTypeVP8 {
		pkt = &codecs.VP8Packet{}
	} else if remoteTrack.PayloadType() == webrtc.DefaultPayloadTypeOpus {
		pkt = &codecs.OpusPacket{}
	} else {
		log.Errorf("TODO remoteTrack.PayloadType()=%v", remoteTrack.PayloadType())
	}

	builder := samplebuilder.New(averageRtpPacketsPerKeyFrame, pkt)
	for {
		select {
		case <-stop:
			return
		default:
			rtp, err := remoteTrack.ReadRTP()
			if err != nil {
				if err == io.EOF {
					return
				}
				log.Errorf(err.Error())
			}

			builder.Push(rtp, gapSeq)
			for s := builder.Pop(); s != nil; s = builder.Pop() {
				if err := (*localTrack).WriteSample(*s); err != nil && err != io.ErrClosedPipe {
					log.Errorf(err.Error())
				}
			}
		}
	}
}
