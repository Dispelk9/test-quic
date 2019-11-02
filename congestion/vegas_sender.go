package congestion

import (
	"fmt"
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
)

// Pktl Count Packet lost
var Pktl int

// Count RTT for vegas
var rttc time.Duration

// cnt the Rtt sample
var cnt uint32

// VegasSender is a Struct
type VegasSender struct {
	hybridSlowStart HybridSlowStart
	prr             PrrSender
	rttStats        *RTTStats
	stats           connectionStats
	vegas           *Vegas
	vegasSenders    map[protocol.PathID]*VegasSender
	noPRR           bool
	reno            bool

	// Track the largest packet that has been sent.
	largestSentPacketNumber protocol.PacketNumber

	// Track the largest packet that has been acked.
	largestAckedPacketNumber protocol.PacketNumber

	// Track the largest packet number outstanding when a CWND cutbacks occurs.
	largestSentAtLastCutback protocol.PacketNumber

	// Congestion window in packets.
	congestionWindow protocol.PacketNumber

	// Slow start congestion window in packets, aka ssthresh.
	slowstartThreshold protocol.PacketNumber

	// Whether the last loss event caused us to exit slowstart.
	// Used for stats collection of slowstartPacketsLost
	lastCutbackExitedSlowstart bool

	// When true, texist slow start with large cutback of congestion window.
	slowStartLargeReduction bool

	// Minimum congestion window in packets.
	minCongestionWindow protocol.PacketNumber

	// Maximum number of outstanding packets for tcp.
	maxTCPCongestionWindow protocol.PacketNumber

	// Number of connections to simulate
	numConnections int

	// ACK counter for the Reno implementation
	congestionWindowCount protocol.ByteCount

	initialCongestionWindow    protocol.PacketNumber
	initialMaxCongestionWindow protocol.PacketNumber

	//Duplicate ACK checking
	DupAck bool
}

// NewVegasSender help other packeges access this struct
func NewVegasSender(clock Clock, rttStats *RTTStats, reno bool, initialCongestionWindow, initialMaxCongestionWindow protocol.PacketNumber, checkDup bool) SendAlgorithmVegas {
	return &VegasSender{
		rttStats:                   rttStats,
		initialCongestionWindow:    initialCongestionWindow,
		initialMaxCongestionWindow: initialMaxCongestionWindow,
		congestionWindow:           initialCongestionWindow,
		minCongestionWindow:        defaultMinimumCongestionWindow,
		slowstartThreshold:         TCPinitthresh,
		maxTCPCongestionWindow:     initialMaxCongestionWindow,
		numConnections:             defaultNumConnections,
		vegas:                      NewVegas(0),
		DupAck:                     checkDup,
		reno:                       reno,
	}
}

// OnPacketSent for vegas
func (v *VegasSender) OnPacketSent(sentTime time.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) bool {
	if !isRetransmittable {
		return false
	}
	if v.InRecovery() {
		v.vegas.lastCongestionWindow = v.congestionWindow
	}

	if v.InRecovery() {
		// PRR is used when in recovery.
		v.prr.OnPacketSent(bytes)
	}

	v.largestSentPacketNumber = packetNumber
	v.hybridSlowStart.OnPacketSent(packetNumber)
	fmt.Println(v.congestionWindow, time.Now().UnixNano(), lrtt)
	return true
}

// OnPacketAcked for vegas. Called when we receive an ack,, maybe occur in SS symbol(1) or CA symbol(0)
func (v *VegasSender) OnPacketAcked(ackedPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	v.largestAckedPacketNumber = utils.MaxPacketNumber(ackedPacketNumber, v.largestAckedPacketNumber)
	if v.InRecovery() {
		// PRR is used when in recovery
		v.prr.OnPacketAcked(ackedBytes)
		return
	}
	//v.vegas.UpdateAckedSinceLastLoss(ackedBytes)
	v.maybeIncreaseCwnd(ackedPacketNumber, ackedBytes, bytesInFlight)
	if v.InSlowStart() {
		v.hybridSlowStart.OnPacketAcked(ackedPacketNumber)
	}
	//fmt.Println(v.congestionWindow, time.Now().UnixNano())

}
func (v *VegasSender) maybeIncreaseCwnd(ackedPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	// Do not increase the congestion window unless the sender is close to using
	// the current window.
	if !v.isCwndLimited(bytesInFlight) {
		return
	}
	if v.congestionWindow >= v.maxTCPCongestionWindow {
		return
	}
	// if v.congestionWindow < v.slowstartThreshold {
	// 	v.hybridSlowStart.Restart()
	// }
	// checking the RTT, it get new RTT with each new sample from RTT_Stats
	if rttc == 0 {
		rttc = lrtt
	}
	// Only update the congestion window when count (cnt) is bigger than 3, with
	// each new RTT, count increases.
	if lrtt != rttc {
		cnt = cnt + 1
	}
	if cnt >= 3 {
		if v.InSlowStart() {
			v.congestionWindow++
			//fmt.Println("In SLowstart")
		} else {
			//fmt.Println("In CA")
			v.congestionWindow = v.vegas.CwndVegasduringCA(v.congestionWindow, bytesInFlight, v.slowstartThreshold)
		}
		cnt = 0
	} else {
		// We don't have enough RTT samples to do the Vegas
		// calculation, so we'll behave like Reno.

		if v.InSlowStart() {
			v.congestionWindow++
		} else {
			// Classic Reno congestion avoidance.
			v.congestionWindowCount++
			// Divide by num_connections to smoothly increase the CWND at a faster
			// rate than conventional Reno.
			if protocol.PacketNumber(v.congestionWindowCount*protocol.ByteCount(v.numConnections)) >= v.congestionWindow {
				v.congestionWindow++

				v.congestionWindowCount = 0
			}
			v.slowstartThreshold = ss
		}
	}
	//fmt.Println(cnt)
}

// OnPacketLost for vegas works when a packet is missing , maybe occur in SS or CA
func (v *VegasSender) OnPacketLost(packetNumber protocol.PacketNumber, lostBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	// Count Packet lost
	Pktl = Pktl + 1
	if Pktl > 3 {
		if packetNumber <= v.largestSentAtLastCutback {
			if v.lastCutbackExitedSlowstart {
				v.stats.slowstartPacketsLost++
				v.stats.slowstartBytesLost += lostBytes
				if v.slowStartLargeReduction {
					if v.stats.slowstartPacketsLost == 1 || (v.stats.slowstartBytesLost/protocol.DefaultTCPMSS) > (v.stats.slowstartBytesLost-lostBytes)/protocol.DefaultTCPMSS {
						// Reduce congestion window by 1 for every mss of bytes lost.
						v.congestionWindow = utils.MaxPacketNumber(v.congestionWindow-1, v.minCongestionWindow)
					}
					v.slowstartThreshold = v.congestionWindow
				}
			}
			return
		}
		v.lastCutbackExitedSlowstart = v.InSlowStart()
		if v.InSlowStart() {
			v.stats.slowstartPacketsLost++
		}

		v.prr.OnPacketLost(bytesInFlight)
		//v.vegas.OnPacketLost()

		// TODO(chromium): Separate out all of slow start into a separate class.
		if v.slowStartLargeReduction && v.InSlowStart() {
			v.congestionWindow = v.congestionWindow - 1
		} else {
			v.congestionWindow = v.vegas.CwndVegascheckAPL(v.congestionWindow, Pktl, bytesInFlight)
		}
		// Enforce a minimum congestion window.
		if v.congestionWindow < v.minCongestionWindow {
			v.congestionWindow = v.minCongestionWindow
		}
		v.slowstartThreshold = v.congestionWindow
		v.largestSentAtLastCutback = v.largestSentPacketNumber
		// reset packet count from congestion avoidance mode. We start
		// counting again when we're out of recovery.
		v.congestionWindowCount = 0
		Pktl = 0
	}
}

// MaybeExitSlowStart for vegas
func (v *VegasSender) MaybeExitSlowStart() {
	if v.InSlowStart() && v.hybridSlowStart.ShouldExitSlowStart(v.rttStats.LatestRTT(), v.rttStats.MinRTT(), v.GetCongestionWindow()/protocol.DefaultTCPMSS) {
		v.ExitSlowstart()
	}
}

// GetCongestionWindow for vegas
func (v *VegasSender) GetCongestionWindow() protocol.ByteCount {
	return protocol.ByteCount(v.congestionWindow) * protocol.DefaultTCPMSS
}

// GetSlowStartThreshold for vegas
func (v *VegasSender) GetSlowStartThreshold() protocol.ByteCount {
	return protocol.ByteCount(v.slowstartThreshold) * protocol.DefaultTCPMSS
}

// OnRetransmissionTimeout for vegas
func (v *VegasSender) OnRetransmissionTimeout(packetsRetransmitted bool) {
	v.largestSentAtLastCutback = 0
	if !packetsRetransmitted {
		return
	}
	v.hybridSlowStart.Restart()
	v.vegas.Reset()
	v.slowstartThreshold = v.congestionWindow / 2
	v.congestionWindow = v.minCongestionWindow
}

// OnConnectionMigration for vegas
func (v *VegasSender) OnConnectionMigration() {
	v.hybridSlowStart.Restart()
	v.prr = PrrSender{}
	v.largestSentPacketNumber = 0
	v.largestAckedPacketNumber = 0
	v.largestSentAtLastCutback = 0
	v.lastCutbackExitedSlowstart = false
	v.vegas.Reset()
	v.congestionWindowCount = 0
	v.congestionWindow = v.initialCongestionWindow
	v.slowstartThreshold = v.initialMaxCongestionWindow
	v.maxTCPCongestionWindow = v.initialMaxCongestionWindow
}

// InRecovery for vegas
func (v *VegasSender) InRecovery() bool {
	return v.largestAckedPacketNumber <= v.largestSentAtLastCutback && v.largestAckedPacketNumber != 0
}

// InSlowStart for vegas
func (v *VegasSender) InSlowStart() bool {
	return v.GetCongestionWindow() < v.GetSlowStartThreshold()
}

// RetransmissionDelay gives the RTO retransmission time
func (v *VegasSender) RetransmissionDelay() time.Duration {
	if v.rttStats.SmoothedRTT() == 0 {
		return 0
	}
	return v.rttStats.SmoothedRTT() + v.rttStats.MeanDeviation()*4
}

// SmoothedRTT for vegas
func (v *VegasSender) SmoothedRTT() time.Duration {
	return v.rttStats.SmoothedRTT()
}

// SetNumEmulatedConnections for vegas
func (v *VegasSender) SetNumEmulatedConnections(n int) {
	v.numConnections = utils.Max(n, 1)
}

// SetSlowStartLargeReduction for vegas
func (v *VegasSender) SetSlowStartLargeReduction(enabled bool) {
	v.slowStartLargeReduction = enabled
}

// ExitSlowstart for vegas
func (v *VegasSender) ExitSlowstart() {
	v.slowstartThreshold = v.congestionWindow
	//fmt.Println("Exit SS")
}

// TimeUntilSend help something
func (v *VegasSender) TimeUntilSend(now time.Time, bytesInFlight protocol.ByteCount) time.Duration {
	if v.InRecovery() {
		// PRR is used when in recovery.
		return v.prr.TimeUntilSend(v.GetCongestionWindow(), bytesInFlight, v.GetSlowStartThreshold())
	}
	if v.GetCongestionWindow() > bytesInFlight {
		return 0
	}
	return utils.InfDuration
}

// isCwndLimited checking if cwnd is limited by program or not
func (v *VegasSender) isCwndLimited(bytesInFlight protocol.ByteCount) bool {

	congestionWindow := v.GetCongestionWindow()
	if bytesInFlight >= congestionWindow {

		return true
	}
	availableBytes := congestionWindow - bytesInFlight
	slowStartLimited := v.InSlowStart() && bytesInFlight > congestionWindow/2

	return slowStartLimited || availableBytes <= maxBurstBytes
}

func (v *VegasSender) RenoBeta() float32 {
	// kNConnectionBeta is the backoff factor after loss for our N-connection
	// emulation, which emulates the effective backoff of an ensemble of N
	// TCP-Reno connections on a single loss event. The effective multiplier is
	// computed as:
	return (float32(v.numConnections) - 1. + renoBeta) / float32(v.numConnections)
}
