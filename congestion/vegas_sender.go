package congestion

import (
	"fmt"
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
)

// Pktl Count Packet lost
var Pktl int

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
	}
}

// OnPacketSent for vegas
func (v *VegasSender) OnPacketSent(sentTime time.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) bool {
	// Only update bytesInFlight for data packets.
	var latest = lrtt
	var min = mrtt
	var delay = ackd
	var timeout time.Duration
	var deltavalue float64 = Delta
	// Checking if delay too high?
	if delay > timeout {
		// Timeout , retransmit immediately
		// This part has done in sentpackethandler.go
	}
	// Incase min equal zero, set sample for minRTT
	if min == 0 {
		min = lrtt //Convert to millisecond
	}
	if v.congestionWindow < 28 {
		v.congestionWindow++
	} else {
		v.congestionWindow = v.congestionWindow / 2
	}
	if v.InSlowStart() {
		//fmt.Println(v.congestionWindow, "InSlowstart:", time.Now().UnixNano(), lrtt)
	}
	if v.InRecovery() {
		//fmt.Println(v.congestionWindow, "InRecovery:", time.Now().UnixNano(), lrtt)
	}

	// Based Rtt equal min RTT
	var BasedRtt = min
	// Observed Rtt equal latest RTT
	var Observed = latest

	// The expected throughput
	var Ex float64 = float64(bytesInFlight) / float64(BasedRtt)

	// The actual throughput
	var Act float64 = float64(bytesInFlight) / float64(Observed)

	//Checking for new slow start
	if Ex-Act < deltavalue {
		if v.congestionWindow < 28 { // Make the cwnd not get over the max link bandwidth
			v.congestionWindow++
		} else {
			v.congestionWindow = v.congestionWindow / 2
		}

	} else if Ex-Act > deltavalue {
		// Expected congestion, start CA
		//v.ExitSlowstart()
		v.vegas.CwndVegasduringCA(v.congestionWindow, bytesInFlight)
		//fmt.Println(v.congestionWindow, "InCA:", time.Now().UnixNano(), lrtt)
	}
	//fmt.Println("Expected Throughput:", Ex, "Actual Throughput:", Act)

	return true
}

// OnPacketAcked for vegas. Called when we receive an ack,, maybe occur in SS symbol(1) or CA symbol(0)
func (v *VegasSender) OnPacketAcked(ackedPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	v.congestionWindow = protocol.PacketNumber(bytesInFlight) / 1350
	// If in slow start ,working normal
	if v.InSlowStart() {
		if v.congestionWindow < Maxcwnd {
			v.congestionWindow++
		} else {
			v.congestionWindow = v.congestionWindow / 2
		}

		// If in recovery, do nthing, cwnd stay the same
	} else if v.InRecovery() {
		//fmt.Println("In Recovery")
		// always applies algorithms checking cwnd
	} else {
		// Always increase cwnd till max
		if v.congestionWindow > Maxcwnd {
			v.congestionWindow = v.vegas.CwndVegasduringCA(v.congestionWindow, bytesInFlight)
		} else {
			// checking the RTT
			v.congestionWindow++
		}

	}
	fmt.Println("Timestamp", time.Now().UnixNano(), "LatestRTT", lrtt)
}

// OnPacketLost for vegas works when a packet is missing , maybe occur in SS or CA
func (v *VegasSender) OnPacketLost(packetNumber protocol.PacketNumber, lostBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	// Count number pkt lost
	Pktl = Pktl + 1

	v.lastCutbackExitedSlowstart = v.InSlowStart()
	if v.InSlowStart() {
		v.stats.slowstartPacketsLost++
	} else if v.congestionWindow > Maxcwnd {
		v.congestionWindow = Maxcwnd / 2
	} else if v.InRecovery() {
		fmt.Println("In Recovery")
	} else {
		//if Realtp < Expectedtp {
		// After Packet Loss in CA
		v.congestionWindow = v.vegas.CwndVegascheckAPL(v.congestionWindow, Pktl, bytesInFlight) // Check van de vegas lam gi khi bi drop packets

	}

	v.prr.OnPacketLost(bytesInFlight)
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
