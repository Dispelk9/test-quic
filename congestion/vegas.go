package congestion

import (
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
	// Maxcwnd works as the max parameter so that the packets don't drop
	Maxcwnd = 28
)

// Try Based RTT
var brtt float64 = 0.005

// checking first
var checking int = 1

// Delta var
var Delta float64

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

// Difference Calculate the Diff based on
// Fairness Comparisons Between TCP Reno and TCP Vegas for Future Deployment of TCP Vegas
func (v *Vegas) Difference(Basedrtt time.Duration, ObservedRtt time.Duration, currentCongestionWindow protocol.PacketNumber) float64 {
	var Diff float64

	if Basedrtt == 0 {
		Basedrtt = 5 * 1000000 // change into millisecond
	}
	if ObservedRtt == 0 {
		ObservedRtt = 7 * 1000000 // change into millisecond, optimal values
	}
	if currentCongestionWindow == 0 {
		currentCongestionWindow = protocol.InitialCongestionWindow
	}

	// Diff equal expected cwnd/basedrtt minus actual cwnd/observedrtt
	Diff = float64(currentCongestionWindow)/float64(Basedrtt) - float64(currentCongestionWindow)/float64(ObservedRtt)
	//fmt.Println("Basedrtt: ", Basedrtt, " Cwmd: ", currentCongestionWindow, " ObservedRtt: ", ObservedRtt, "Diff", Diff)
	Delta = Diff
	return Diff
}

// CwndVegasduringCA computes a new congestion window to use at the beginning or after
// a loss event. Returns the new congestion window in packets.
func (v *Vegas) CwndVegasduringCA(currentCongestionWindow protocol.PacketNumber, biF protocol.ByteCount) protocol.PacketNumber {
	//fmt.Println("Time in Avoidance.", time.Now().Nanosecond())
	var TarCwnd protocol.PacketNumber = currentCongestionWindow
	// if TarCwnd > Maxcwnd {
	// 	TarCwnd = Maxcwnd
	// }
	// Latest RTT from rtt_stats
	var lrtt1 = lrtt
	// Latest minRTT from rtt_stats
	var mrtt1 = mrtt
	// Observed RTT from latest RTT
	v.ObRtt = lrtt1
	//Checking if BasedRtt equal 0. Set a parameter for Based RTT
	if v.BasedRtt == 0 {
		v.BasedRtt = lrtt1 //5 * 1000000
	}
	// If Basedrtt bigger that latest min RTT, get value from mrtt1
	if v.BasedRtt > mrtt1 {
		v.BasedRtt = mrtt1
	}
	// Checking if ObRTT is smaller than min RTT, if smaller get new BasedRTT
	if v.ObRtt < v.BasedRtt {
		v.BasedRtt = v.ObRtt
	}
	// Calculate Difference value based on the BasedRTT, ObservedRTT and the congestion window in the mean time
	var Diff float64 = v.Difference(v.BasedRtt, lrtt1, currentCongestionWindow)
	if Diff < float64(Valpha)/float64(v.BasedRtt) {
		TarCwnd = TarCwnd + 3

	} else if Diff >= float64(Valpha)/float64(v.BasedRtt) && float64(Diff) <= float64(Vbeta)/float64(v.BasedRtt) {
		TarCwnd = currentCongestionWindow

	} else if Diff > float64(Vbeta)/float64(v.BasedRtt) {
		TarCwnd = TarCwnd - 3
	}
	//fmt.Println("++++ ACK ++++")
	//fmt.Println("Tarcwnd in ACK:", TarCwnd, "Cwnd:", currentCongestionWindow, "biF:", biF, "BasedRTT:", mrtt1, "ObservedRTT", lrtt)
	//fmt.Println("++++++++++++++++++++++++++++++++++++++++")
	//fmt.Println("LatestRTT", lrtt1, "timestamp", time.Now().Unix())
	v.lastCongestionWindow = TarCwnd
	return TarCwnd
}

// CwndVegascheckAPL check if it is needed to change to CWND or not. Returns the new congestion window in packets.
func (v *Vegas) CwndVegascheckAPL(currentCongestionWindow protocol.PacketNumber, packetlost int, biF protocol.ByteCount) protocol.PacketNumber {
	//fmt.Println("Time get Packetloss:", time.Now().Nanosecond())
	var TarCwnd protocol.PacketNumber
	// if TarCwnd > Maxcwnd {
	// 	TarCwnd = Maxcwnd / 2
	// }
	// Counting the packets lost , if the after 30th packet it still losing, it will decrease Cwnd/2
	// If not, keep the cwnd.

	if checking+29 < packetlost {
		TarCwnd = currentCongestionWindow / 2

	} else {
		TarCwnd = currentCongestionWindow + 3
		//checking = packetlost
	}
	//fmt.Println("++++ APL ++++")
	//fmt.Println("TarCwnd in APL:", TarCwnd, "Cwnd:", currentCongestionWindow, "Packetlost count:", packetlost, "biF:", biF, "BasedRTT:", mrtt, "ObservedRTT", lrtt)
	//fmt.Println("++++++++++++++++++++++++++++++++++++++++")
	return TarCwnd
}
