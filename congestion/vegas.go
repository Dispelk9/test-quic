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

// Vegas implements the vegas algorithm from TCP
type Vegas struct {
	// HybrindSlowStart works with slow start function
	HybridSlowStart HybridSlowStart
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
	v.BasedRtt = initialRTTus
	v.ObRtt = initialRTTus
}

//NewVegas returns a new Vegas instance
func NewVegas(ackedBytes protocol.ByteCount) *Vegas {
	v := &Vegas{}
	return v
}

// Difference Calculate the Difference
func Difference(Basedrtt time.Duration, ObservedRtt time.Duration, currentCongestionWindow protocol.PacketNumber) int64 {
	var Diff int64
	if Basedrtt == 0 {
		fmt.Println("Basedrtt is equal 0")
		fmt.Println(Basedrtt, int64(Basedrtt))
	} else if ObservedRtt == 0 {
		fmt.Println("Observed Rtt is equal 0")
		ObservedRtt = initialRTTus
		fmt.Println(ObservedRtt, int64(ObservedRtt), mrtt, lrtt)
	}
	// Diff equal expected cwnd/basedrtt minus actual cwnd/observedrtt
	Diff = int64(currentCongestionWindow)/int64(Basedrtt) - int64(currentCongestionWindow)/int64(ObservedRtt)
	return Diff
}

// CwndVegasSS computes a new congestion window to use at the beginning or after
// a loss event. Returns the new congestion window in packets.
func (v *Vegas) CwndVegasSS(currentCongestionWindow protocol.PacketNumber) protocol.PacketNumber {
	var TarCwnd protocol.PacketNumber = currentCongestionWindow
	var lrtt1 = lrtt
	if v.BasedRtt == 0 {
		v.BasedRtt = initialRTTus
	}
	var Diff int64 = Difference(v.BasedRtt, lrtt1, currentCongestionWindow)
	if Diff < Valpha/int64(v.BasedRtt) {
		TarCwnd = TarCwnd + 1
		v.BasedRtt = mrtt
		v.ObRtt = lrtt1
	} else if Diff >= Valpha/int64(v.BasedRtt) && Diff <= Vbeta/int64(v.BasedRtt) {
		TarCwnd = currentCongestionWindow
		v.BasedRtt = mrtt
		v.ObRtt = lrtt1
	} else if Diff > Vbeta/int64(v.BasedRtt) {
		TarCwnd = TarCwnd - 1
		v.BasedRtt = mrtt
		v.ObRtt = lrtt1
	}
	fmt.Println(TarCwnd, Diff, lrtt1, v.BasedRtt, "this works in SS")
	return TarCwnd
}

// CwndVegasCA computes a new congestion window to use at the beginning or after
// a loss event. Returns the new congestion window in packets.
func (v *Vegas) CwndVegasCA(currentCongestionWindow protocol.PacketNumber) protocol.PacketNumber {
	var TarCwnd protocol.PacketNumber = currentCongestionWindow
	var lrtt1 = lrtt
	if v.BasedRtt == 0 {
		v.BasedRtt = initialRTTus
	}
	var Diff int64 = Difference(v.BasedRtt, lrtt1, currentCongestionWindow)
	if Diff < Valpha/int64(v.BasedRtt) {
		TarCwnd = TarCwnd + 1
		v.BasedRtt = mrtt
		v.ObRtt = lrtt1
	} else if Diff >= Valpha/int64(v.BasedRtt) && Diff <= Vbeta/int64(v.BasedRtt) {
		TarCwnd = currentCongestionWindow
		v.BasedRtt = mrtt
		v.ObRtt = lrtt1
	} else if Diff > Vbeta/int64(v.BasedRtt) {
		TarCwnd = TarCwnd - 1
		v.BasedRtt = mrtt
		v.ObRtt = lrtt1
	}
	fmt.Println(TarCwnd, Diff, lrtt1, v.BasedRtt, v.ObRtt, mrtt, "this works in CA")
	return TarCwnd
}
