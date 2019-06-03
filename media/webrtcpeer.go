package media

import (
	"time"

	"sync"

	"github.com/pion/ion/log"
	"github.com/pion/webrtc/v2"
)

var (
	webrtcEngine *WebRTCEngine
)

func init() {
	webrtcEngine = NewWebRTCEngine()
}

type WebRTCPeer struct {
	ID              string
	PC              *webrtc.PeerConnection
	VideoTrack      *webrtc.Track
	AudioTrack      *webrtc.Track
	Stop            chan int
	Pli             chan int
	GapSeq          chan uint16
	GapSeqCount     map[uint16]int
	GapSeqCountLock sync.RWMutex
}

func NewWebRTCPeer(id string) *WebRTCPeer {
	return &WebRTCPeer{
		ID:          id,
		Stop:        make(chan int),
		Pli:         make(chan int),
		GapSeq:      make(chan uint16, 100),
		GapSeqCount: make(map[uint16]int),
	}
}

func (p *WebRTCPeer) Close() {
	close(p.Stop)
	close(p.Pli)
	close(p.GapSeq)
}

func (p *WebRTCPeer) AnswerSender(offer webrtc.SessionDescription) (answer webrtc.SessionDescription, err error) {
	log.Infof("WebRTCPeer.AnswerSender")
	// return webrtcEngine.CreateReceiver(offer, &p.PC, &p.VideoTrack, &p.AudioTrack, p.stop, p.pli)
	return webrtcEngine.CreateReceiver(offer, p)
}

func (p *WebRTCPeer) AnswerReceiver(offer webrtc.SessionDescription, addVideoTrack **webrtc.Track, addAudioTrack **webrtc.Track) (answer webrtc.SessionDescription, err error) {
	log.Infof("WebRTCPeer.AnswerReceiver")
	// return webrtcEngine.CreateSender(offer, &p.PC, addVideoTrack, addAudioTrack, p.Stop)
	return webrtcEngine.CreateSender(offer, p, addVideoTrack, addAudioTrack)
}

func (p *WebRTCPeer) SendPLI() {
	go func() {
		defer func() {
			// recover from panic caused by writing to a closed channel
			if r := recover(); r != nil {
				log.Errorf("%v", r)
				return
			}
		}()
		ticker := time.NewTicker(time.Second * 5)
		// i := 0
		for {
			select {
			case <-ticker.C:
				p.Pli <- 1
				// if i > 3 {
				// return
				// }
				// i++
			case <-p.Stop:
				return
			}
		}
	}()
}

func (p *WebRTCPeer) GapSeqInc(gapSeq uint16) {
	p.GapSeqCountLock.RLock()
	defer p.GapSeqCountLock.RUnlock()
	p.GapSeqCount[gapSeq]++
}

func (p *WebRTCPeer) GetGapSeqCount(gapSeq uint16) int {
	p.GapSeqCountLock.RLock()
	defer p.GapSeqCountLock.RUnlock()
	return p.GapSeqCount[gapSeq]
}

func (p *WebRTCPeer) DelGapSeq(gapSeq uint16) {
	p.GapSeqCountLock.Lock()
	defer p.GapSeqCountLock.Unlock()
	// delete all member before gapSeq
	for i := gapSeq; i > gapSeq-uint16(len(p.GapSeqCount)); i-- {
		if _, ok := p.GapSeqCount[i]; !ok {
			break
		}
		delete(p.GapSeqCount, gapSeq)
	}
}
