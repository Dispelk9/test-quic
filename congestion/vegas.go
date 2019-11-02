package congestion

import (
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
)

const (
	// Valpha defines alpha of Vegas
	Valpha int64 = 2
	// Vbeta defines beta of Vegas
	Vbeta int64 = 4
	// Vgamma defines gamma of Vegas
	Vgamma int64 = 1
	// numConnections
	numConnections = 2
	// Maxcwnd works as the max parameter so that the packets don't drop
	Maxcwnd = 30
)

// Try Based RTT
var brtt float64 = 0.005

// checking first
var checking int = 1

// Delta var
var Delta float64

// sshthresh for vegassender
var ss protocol.PacketNumber

// Vegas implements the vegas algorithm from TCP
type Vegas struct {
	// HybrindSlowStart works with slow start function
	//HybridSlowStart HybridSlowStart
	//clock shows time
	clock Clock
	// Number of connections to simulate.
	numConnections int
	// Time when this cycle started, after last loss event.
	epoch time.Time
	// Time when sender went into application-limited period. Zero if not in
	// application-limited period.
	appLimitedStartTime time.Time
	// Time when we updated last_congestion_window.
	lastUpdateTime time.Time
	// Last congestion window (in packets) used.
	lastCongestionWindow protocol.PacketNumber
	// Max congestion window (in packets) used just before last loss event.
	// Note: to improve fairness to other streams an additional back off is
	// applied to this value if the new value is below our latest value.
	lastMaxCongestionWindow protocol.PacketNumber
	// Number of acked packets since the cycle started (epoch).
	ackedPacketsCount protocol.PacketNumber
	// TCP Vegas equivalent congestion window in packets.
	estimatedTCPcongestionWindow protocol.PacketNumber
	// Origin point of cubic function.
	originPointCongestionWindow protocol.PacketNumber
	// Time to origin point of cubic function in 2^10 fractions of a second.
	timeToOriginPoint uint32
	// Last congestion window in packets computed by vegas function.
	lastTargetCongestionWindow protocol.PacketNumber
	// Based RTT
	BasedRtt time.Duration
	// Observed RTT
	ObRtt time.Duration
	// MaxRTT Last max RTT
	MaxRTT time.Duration
}

// Reset is called after a timeout to reset the vegas state
func (v *Vegas) Reset() {
	v.epoch = time.Time{}
	v.appLimitedStartTime = time.Time{}
	v.lastUpdateTime = time.Time{}
	v.lastCongestionWindow = 0
	v.lastMaxCongestionWindow = 0
	v.ackedPacketsCount = 0
	v.estimatedTCPcongestionWindow = 0
	v.originPointCongestionWindow = 0
	v.lastTargetCongestionWindow = 0
	v.BasedRtt = 0
	v.ObRtt = 0
}

//NewVegas returns a new Vegas instance
func NewVegas(ackedBytes protocol.ByteCount) *Vegas {
	v := &Vegas{}
	return v
}

// OnApplicationLimited help program not increase cwnd when not get ack values
func (v *Vegas) OnApplicationLimited() {
	if shiftQuicCubicEpochWhenAppLimited {
		if v.appLimitedStartTime.IsZero() {
			v.appLimitedStartTime = v.clock.Now()
		}
	} else {
		// Epoch should not increase
		v.epoch = time.Time{}
	}
}

// Difference Calculate the Diff based on
// Fairness Comparisons Between TCP Reno and TCP Vegas for Future Deployment of TCP Vegas
func (v *Vegas) Difference(Basedrtt time.Duration, ObservedRtt time.Duration, currentCongestionWindow protocol.PacketNumber) float64 {
	var Diff float64

	if Basedrtt == 0 {
		Basedrtt = lrtt //20 * 1000000 // change into millisecond 20
	}
	if ObservedRtt == 0 {
		ObservedRtt = lrtt //20 * 1000000 // change into millisecond, optimal values 20
	}
	if currentCongestionWindow == 0 {
		currentCongestionWindow = protocol.InitialCongestionWindow
	}
	// Expected Rate
	var Expr = float64(currentCongestionWindow) / float64(Basedrtt)
	// Actual Rate
	var Actr = float64(currentCongestionWindow) / float64(ObservedRtt)
	if Actr > Expr {
		v.BasedRtt = lrtt
	}
	// Diff equal expected cwnd/basedrtt minus actual cwnd/observedrtt
	Diff = Expr - Actr
	//fmt.Println(Expr, Actr, time.Now().UnixNano())
	Delta = Diff
	return Diff
}

// CwndVegasduringCA computes a new congestion window to use at the beginning or after
// a loss event. Returns the new congestion window in packets.
func (v *Vegas) CwndVegasduringCA(currentCongestionWindow protocol.PacketNumber, biF protocol.ByteCount, ssthresh protocol.PacketNumber) protocol.PacketNumber {

	// Latest RTT from rtt_stats
	var lrtt1 = lrtt
	// Latest minRTT from rtt_stats
	var mrtt1 = mrtt
	// Observed RTT from latest RTT
	v.ObRtt = lrtt1

	//Checking if BasedRtt equal 0. Set a parameter for Based RTT
	if v.BasedRtt == 0 {
		v.BasedRtt = 20 //* 1000000
	}
	// If Basedrtt bigger that latest min RTT, get value from mrtt1
	if v.BasedRtt > mrtt1 {
		v.BasedRtt = mrtt
	}
	// Checking if ObRTT is smaller than based RTT, if smaller get new BasedRTT
	if v.ObRtt < v.BasedRtt {
		v.BasedRtt = v.ObRtt
	}

	// Calculate the target Cwnd
	var TarCwnd protocol.PacketNumber = currentCongestionWindow * protocol.PacketNumber(v.BasedRtt/mrtt1)

	// Calculate Difference value based on the BasedRTT, ObservedRTT and the congestion window in the mean time
	var Diff float64 = v.Difference(v.BasedRtt, lrtt1, currentCongestionWindow)

	if Diff > float64(Vgamma) && currentCongestionWindow <= ssthresh {
		currentCongestionWindow = utils.MinPacketNumber(ssthresh, TarCwnd+1)
		ss = TarCwnd / 2
	} else if currentCongestionWindow < ssthresh {
		// Slow start
	} else {

		if Diff < float64(Valpha) {
			TarCwnd = TarCwnd + 1

		} else if Diff > float64(Vbeta) {
			TarCwnd = TarCwnd - 1
		} else {
			TarCwnd = currentCongestionWindow
		}
	}

	return TarCwnd * 5
}

// CwndVegascheckAPL check if it is needed to change to CWND or not. Returns the new congestion window in packets.
func (v *Vegas) CwndVegascheckAPL(currentCongestionWindow protocol.PacketNumber, packetlost int, biF protocol.ByteCount) protocol.PacketNumber {
	var TarCwnd protocol.PacketNumber

	if currentCongestionWindow < v.lastMaxCongestionWindow {
		// We never reached the old max, so assume we are competing with another flow.
		v.lastMaxCongestionWindow = protocol.PacketNumber(betaLastMax * float32(currentCongestionWindow))
	} else {
		v.lastMaxCongestionWindow = currentCongestionWindow
	}
	v.epoch = time.Time{} // Reset time.

	if currentCongestionWindow < v.lastMaxCongestionWindow {
		// We never reached the old max, so assume we are competing with another
		// flow. Use our extra back off factor to allow the other flow to go up.
		v.lastMaxCongestionWindow = protocol.PacketNumber(betaLastMax * float32(currentCongestionWindow))
	} else {
		v.lastMaxCongestionWindow = currentCongestionWindow
	}
	v.epoch = time.Time{} // Reset time.
	TarCwnd = protocol.PacketNumber(float32(currentCongestionWindow) * 0.7)

	return TarCwnd * 5
}
