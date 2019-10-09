package congestion

import (
	"fmt"
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
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
)

// Try Based RTT
var brtt float64 = 0.005

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

// // Obcheck check if ObservedRTT too high or not
// func (v *Vegas) Obcheck(ObservedRTT time.Duration) bool {
// 	v.MaxRTT = 1
// 	if ObservedRTT > v.MaxRTT {

// 	}
// 	return true
// }

// Difference Calculate the Difference
func (v *Vegas) Difference(Basedrtt time.Duration, ObservedRtt time.Duration, currentCongestionWindow protocol.PacketNumber) float64 {
	var Diff float64
	//currentCongestionWindow = currentCongestionWindow / 1350

	if Basedrtt == 0 {
		Basedrtt = 5 * 1000000
	}
	if ObservedRtt == 0 {
		ObservedRtt = 7 * 1000000
	}
	if currentCongestionWindow == 0 {
		currentCongestionWindow = 5 * 1350
	}

	// Diff equal expected cwnd/basedrtt minus actual cwnd/observedrtt
	fmt.Println(float64(Basedrtt), float64(ObservedRtt), float64(currentCongestionWindow))
	Diff = float64(currentCongestionWindow)/float64(Basedrtt) - float64(currentCongestionWindow)/float64(ObservedRtt)
	fmt.Println("Basedrtt: ", Basedrtt, " Cwmd: ", currentCongestionWindow, " ObservedRtt: ", ObservedRtt, "Diff", Diff)

	return Diff
}

// CwndVegasduringCA check if it is needed to change to CWND or not. Returns the new congestion window in packets.
func (v *Vegas) CwndVegasduringCA(currentCongestionWindow protocol.PacketNumber) protocol.PacketNumber {
	var TarCwnd protocol.PacketNumber
	//
	// var Ex float64 = float64(currentCongestionWindow) / float64(v.BasedRtt)

	// var Act float64 = float64(currentCongestionWindow) / float64(lrtt)
	// if Ex >= Act {
	TarCwnd = v.CwndVegasCA(currentCongestionWindow)
	// } else {
	// 	// K update Congestion Window dua theo RTT
	// 	TarCwnd = v.lastCongestionWindow
	// }
	//fmt.Println("Expected throughput ", Ex, "Actual throughput", Act, TarCwnd)
	return TarCwnd
}

// CwndVegasCA computes a new congestion window to use at the beginning or after
// a loss event. Returns the new congestion window in packets.
func (v *Vegas) CwndVegasCA(currentCongestionWindow protocol.PacketNumber) protocol.PacketNumber {
	var TarCwnd protocol.PacketNumber = currentCongestionWindow
	var lrtt1 = lrtt
	var mrtt1 = mrtt
	v.ObRtt = lrtt1
	if v.BasedRtt == 0 {
		v.BasedRtt = 5 * 1000000
	} else if v.ObRtt < v.BasedRtt {
		v.BasedRtt = v.ObRtt
	}
	var Diff float64 = v.Difference(v.BasedRtt, lrtt1, currentCongestionWindow)
	if Diff < float64(Valpha)/float64(v.BasedRtt) {
		TarCwnd = TarCwnd + 1350

	} else if Diff >= float64(Valpha)/float64(v.BasedRtt) && float64(Diff) <= float64(Vbeta)/float64(v.BasedRtt) {
		TarCwnd = currentCongestionWindow

	} else if Diff > float64(Vbeta)/float64(v.BasedRtt) {
		TarCwnd = TarCwnd - 1350
	}
	fmt.Println("+++++++++ Congestion Avoidance +++++++++")
	fmt.Println(TarCwnd, Diff, int64(Diff), lrtt1, v.ObRtt, mrtt1, v.BasedRtt)
	fmt.Println("++++++++++++++++++++++++++++++++++++++++")
	v.lastCongestionWindow = TarCwnd
	return TarCwnd
}
