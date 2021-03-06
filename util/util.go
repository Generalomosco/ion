package util

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"net"
	"strconv"
	"strings"

	"time"

	"github.com/pion/ion/log"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/stun"
	"github.com/pion/webrtc/v2"
)

var (
	localIPPrefix = [...]string{"192.168", "10.0", "169.254", "172.16"}
)

func IsLocalIP(ip string) bool {
	for i := 0; i < len(localIPPrefix); i++ {
		if strings.HasPrefix(ip, localIPPrefix[i]) {
			return true
		}
	}
	return false
}

func GetIntefaceIP() string {
	addrs, _ := net.InterfaceAddrs()

	// get internet ip first
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			if !IsLocalIP(ipnet.IP.String()) {
				return ipnet.IP.String()
			}
		}
	}

	// get internat ip
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}

	return ""
}

func GetIPFromSTUN(stunURL string) string {
	// Creating a "connection" to STUN server.
	c, err := stun.Dial("udp", stunURL)
	if err != nil {
		log.Errorf("stun dial err %v", err)
		return ""
	}

	var ip string
	// Building binding request with random transaction id.
	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	// Sending request to STUN server, waiting for response message.
	ch := make(chan string)
	if err := c.Do(message, func(res stun.Event) {
		if res.Error != nil {
			log.Errorf("stun res err %v", err)
			close(ch)
			return
		}
		// Decoding XOR-MAPPED-ADDRESS attribute from message.
		var xorAddr stun.XORMappedAddress
		if err := xorAddr.GetFrom(res.Message); err != nil {
			log.Errorf("stun messge err %v", err)
			return
		}
		ip = xorAddr.IP.String()
	}); err != nil {
		log.Errorf("stun do err %v", err)
		close(ch)
		return ""
	}

	return ip
}

func MarshalStr(args ...interface{}) string {
	m := Map(args)
	if byt, err := json.Marshal(m); err != nil {
		log.Errorf(err.Error())
		return ""
	} else {
		return string(byt)
	}
}

func MarshalStrMap(m map[string]string) string {
	if byt, err := json.Marshal(m); err != nil {
		log.Errorf(err.Error())
		return ""
	} else {
		return string(byt)
	}
}

func Marshal(m map[string]interface{}) string {
	if byt, err := json.Marshal(m); err != nil {
		log.Errorf(err.Error())
		return ""
	} else {
		return string(byt)
	}
}

func Unmarshal(str string) map[string]interface{} {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(str), &data); err != nil {
		log.Errorf(err.Error())
		return data
	}
	return data
}

func Recover(flag string) {
	if err := recover(); err != nil {
		log.Errorf("[%s] recover err => %v", flag, err)
	}
}

// get value from map
func Val(msg map[string]interface{}, key string) string {
	if msg == nil {
		return ""
	}
	val := msg[key]
	if val == nil {
		return ""
	}
	switch val.(type) {
	case string:
		return val.(string)
	default:
		return ""
	}
}

// make kv to map, args should be multiple of 2
func Map(args ...interface{}) map[string]interface{} {
	if len(args)%2 != 0 {
		return nil
	}
	msg := make(map[string]interface{})
	for i := 0; i < len(args)/2; i++ {
		msg[args[2*i].(string)] = args[2*i+1]
	}
	return msg
}

func GetIDFromRTP(pkt *rtp.Packet) string {
	if !pkt.Header.Extension || len(pkt.Header.ExtensionPayload) < 36 {
		log.Warnf("pkt invalid extension")
		return ""
	}
	return string(bytes.TrimRight(pkt.Header.ExtensionPayload, "\x00"))
}

func SetIDToRTP(pkt *rtp.Packet, id string) *rtp.Packet {
	pkt.Header.Extension = true

	//the payload must be in 32-bit words and bigger than extPayload
	if len(pkt.Header.ExtensionPayload)%4 != 0 || len(pkt.Header.ExtensionPayload) < len(id) {
		n := 4 * (len(id)/4 + 1)
		pkt.Header.ExtensionPayload = make([]byte, n)
	}
	copy(pkt.Header.ExtensionPayload, id)
	return pkt
}

func GetIP(addr string) string {
	if strings.Contains(addr, ":") {
		return strings.Split(addr, ":")[0]
	}
	return ""
}

func GetPort(addr string) string {
	if strings.Contains(addr, ":") {
		return strings.Split(addr, ":")[1]
	}
	return ""
}

func GetLostSN(begin, bitmap uint16) []uint16 {
	if bitmap == 0 {
		return []uint16{begin}
	}
	var sns []uint16
	for i := uint16(0); i < 15; i++ {
		if (bitmap >> i & 0x01) == 1 {
			sns = append(sns, begin+i)
		}
	}
	return sns
}

func IsVP8KeyFrame(pkt *rtp.Packet) bool {
	if pkt != nil && pkt.PayloadType == webrtc.DefaultPayloadTypeVP8 {
		vp8 := &codecs.VP8Packet{}
		vp8.Unmarshal(pkt.Payload)
		// start of a frame, there is a payload header  when S == 1
		if vp8.S == 1 && vp8.Payload[0]&0x01 == 0 {
			//key frame
			// log.Infof("vp8.Payload[0]=%b pkt=%v", vp8.Payload[0], pkt)
			return true
		}
	}
	return false
}

// keyFrame : only get nackpair from keyframe buffer
func NackPair(buffer [65536]*rtp.Packet, begin, end uint16, keyFrame bool) (*rtcp.NackPair, int) {

	var lostPkt int

	//size is < 16
	if end-begin > 16 {
		return nil, lostPkt
	}

	//only check key frame if keyFrame=true
	var keyBegin, keyEnd uint16
	if keyFrame {
		//find key frame begin pkt
		for i := begin; i < end; i++ {
			if IsVP8KeyFrame(buffer[i]) {
				keyBegin = i
				break
			}
		}

		//find key frame end pkt
		if keyBegin != 0 {
			for i := keyBegin; i < end; i++ {
				if !IsVP8KeyFrame(buffer[i]) {
					keyEnd = i
					break
				}
			}
		}
	}

	//Bitmask of following lost packets (BLP)
	blp := uint16(0)
	lost := uint16(0)

	if keyFrame {
		if keyBegin != 0 {
			begin = keyBegin
		}
		if keyEnd != 0 {
			end = keyEnd
		}
	}

	//find first lost pkt
	for i := begin; i < end; i++ {
		if buffer[i] == nil {
			lost = i
			lostPkt++
			break
		}
	}

	//no packet lost
	if lost == 0 {
		return nil, lostPkt
	}

	//calc blp
	for i := lost; i < end; i++ {
		//calc from next lost packet
		if i > lost && buffer[i] == nil {
			blp = blp | (1 << (i - lost - 1))
			lostPkt++
		}
	}
	log.Debugf("util.NackPair begin=%v end=%v buffer=%v\n", begin, end, buffer[begin:end])
	return &rtcp.NackPair{PacketID: lost, LostPackets: rtcp.PacketBitmap(blp)}, lostPkt
}

func GetMills() int64 {
	return time.Now().UnixNano() / 1e6
}

func IsVideo(pt uint8) bool {
	if pt == webrtc.DefaultPayloadTypeVP8 ||
		pt == webrtc.DefaultPayloadTypeVP9 ||
		pt == webrtc.DefaultPayloadTypeH264 {
		return true
	}
	return false
}

func ReadAbsSendTime(pkt *rtp.Packet) (uint32, bool) {
	if !pkt.Extension && len(pkt.ExtensionPayload) != 3 {
		log.Errorf("ReadAbsSendTime pkt.Extension=%v len(pkt.Extension)=%d profile=%v", pkt.Extension, len(pkt.ExtensionPayload), pkt.ExtensionProfile)
		return 0, false
	}
	return uint32(pkt.ExtensionPayload[2]) | uint32(pkt.ExtensionPayload[1])<<8 | uint32(pkt.ExtensionPayload[0])<<16, true
}

func StrToUint8(str string) uint8 {
	i, err := strconv.ParseUint(str, 10, 8)
	log.Infof("StrToUint8 str=%v i=%v err=%v", str, i, err)
	return uint8(i)
}

func StrToUint32(str string) uint32 {
	i, err := strconv.ParseUint(str, 10, 32)
	log.Infof("StrToUint32 str=%v i=%v err=%v", str, i, err)
	return uint32(i)
}

func randInt(min int, max int) int {
	return min + rand.Intn(max-min)
}

func RandStr(l int) string {
	bytes := make([]byte, l)
	for i := 0; i < l; i++ {
		bytes[i] = byte(randInt(65, 90))
	}
	return string(bytes)
}
