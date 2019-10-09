package congestion

import (
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
)

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
}

// NewVegasSender help other packeges access this struct
func NewVegasSender(clock Clock, rttStats *RTTStats, reno bool, initialCongestionWindow, initialMaxCongestionWindow protocol.PacketNumber) SendAlgorithmVegas {
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
	}
}

// OnPacketSent for vegas
func (v *VegasSender) OnPacketSent(sentTime time.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) bool {
	// fmt.Println("Get in this function")
	// if !isRetransmittable {

	// 	return false
	// }
	// v.congestionWindow = v.vegas.CwndVegasduringCA(protocol.PacketNumber(bytesInFlight))

	// v.hybridSlowStart.OnPacketSent(packetNumber)
	// return true
	// Only update bytesInFlight for data packets.
	if !isRetransmittable {

		return false
	}
	if v.InRecovery() {
		// PRR is used when in recovery.
		Evaluate3 = false
		Evaluate2 = false
		Evaluate1 = v.InRecovery()
		v.prr.OnPacketSent(bytes)

	} else if v.InSlowStart() == true {
		//fmt.Println("InSlowStart")
		Evaluate1 = false
		Evaluate2 = v.InSlowStart()
		Evaluate3 = false
	}

	v.largestSentPacketNumber = packetNumber
	//c.Printschedule(c.congestionWindow)
	v.hybridSlowStart.OnPacketSent(packetNumber)
	return true
}

// Called when we receive an ack. Normal TCP tracks how many packets one ack
// represents, but quic has a separate ack for each packet.

// OnPacketAcked for vegas
func (v *VegasSender) OnPacketAcked(ackedPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	v.largestAckedPacketNumber = utils.MaxPacketNumber(ackedPacketNumber, v.largestAckedPacketNumber)
	if v.InRecovery() {
		// PRR is used when in recovery.
		if !v.noPRR {
			v.prr.OnPacketAcked(ackedBytes)
		}
		Evaluate3 = false
		Evaluate2 = false
		Evaluate1 = v.InRecovery()
		return
	}
	v.maybeIncreaseCwndVegas(ackedPacketNumber, ackedBytes, bytesInFlight)
	if v.InSlowStart() {
		v.hybridSlowStart.OnPacketAcked(ackedPacketNumber)
	}
}

// OnPacketLost for vegas
func (v *VegasSender) OnPacketLost(packetNumber protocol.PacketNumber, lostBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
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
	// TODO(chromium): Separate out all of slow start into a separate class.
	if v.slowStartLargeReduction && v.InSlowStart() {
		v.congestionWindow = v.congestionWindow - 1

	} else {
		//v.congestionWindow = v.vegas.CongestionWindowAfterPacketLoss(v.congestionWindow)

	}
	// Enforce a minimum congestion window.
	if v.congestionWindow < v.minCongestionWindow {
		v.congestionWindow = v.minCongestionWindow

	}
	v.slowstartThreshold = v.congestionWindow
	v.largestSentAtLastCutback = v.largestSentPacketNumber
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
	// TODO should it be done also for OLIA?
}

// SetSlowStartLargeReduction for vegas
func (v *VegasSender) SetSlowStartLargeReduction(enabled bool) {
	v.slowStartLargeReduction = enabled
}

// ExitSlowstart for vegas
func (v *VegasSender) ExitSlowstart() {
	v.slowstartThreshold = v.congestionWindow
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

// Called when we receive an ack. Normal TCP tracks how many packets one ack
// represents, but quic has a separate ack for each packet.
func (v *VegasSender) maybeIncreaseCwndVegas(ackedPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {

	// Do not increase the congestion window unless the sender is close to using
	// the current window.
	// if !v.isCwndLimited(bytesInFlight) {
	// 	v.vegas.OnApplicationLimited()

	// 	return
	// }
	if v.congestionWindow >= v.maxTCPCongestionWindow {
		return
	}
	if v.InSlowStart() {
		// TCP slow start, exponential growth, increase by one for each ACK.
		v.congestionWindow++
		// if c.congestionWindow > 50 {
		// 	c.congestionWindow = 35
		// }
		return
	}
	if v.reno {
		// Classic Reno congestion avoidance.
		v.congestionWindowCount++
		// Divide by num_connections to smoothly increase the CWND at a faster
		// rate than conventional Reno.
		if protocol.PacketNumber(v.congestionWindowCount*protocol.ByteCount(v.numConnections)) >= v.congestionWindow {
			v.congestionWindow++
			// if c.congestionWindow > 50 {
			// 	c.congestionWindow = 35
			// }
			v.congestionWindowCount = 0
		}
	} else {
		v.congestionWindow = utils.MinPacketNumber(v.maxTCPCongestionWindow, v.vegas.CwndVegasduringCA(v.congestionWindow))
	}
}
