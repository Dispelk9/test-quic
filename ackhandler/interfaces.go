package ackhandler

import (
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/wire"
)

// SentPacketHandler handles ACKs received for outgoing packets
type SentPacketHandler interface {
	// SentPacket may modify the packet
	SentPacket(packet *Packet) error
	ReceivedAck(ackFrame *wire.AckFrame, withPacketNumber protocol.PacketNumber, recvTime time.Time) error

	// Specific to multipath operation
	ReceivedClosePath(f *wire.ClosePathFrame, withPacketNumber protocol.PacketNumber, recvTime time.Time) error
	SetInflightAsLost()

	SendingAllowed() bool
	CongestionFree() bool
	OvershootFree(pathNum int) bool
	GetStopWaitingFrame(force bool) *wire.StopWaitingFrame
	ShouldSendRetransmittablePacket() bool
	DequeuePacketForRetransmission() (packet *Packet)
	GetLeastUnacked() protocol.PacketNumber

	GetAlarmTimeout() time.Time
	OnAlarm()

	DuplicatePacket(packet *Packet)

	GetStatistics() (uint64, uint64, uint64, uint64)
	GetCongestionWindow() uint64
	GetBytesInFlight() uint64
	RemovePacketByNumber(protocol.PacketNumber) bool
}

// ReceivedPacketHandler handles ACKs needed to send for incoming packets
type ReceivedPacketHandler interface {
	ReceivedPacket(packetNumber protocol.PacketNumber, shouldInstigateAck bool, packetLength uint64) error
	SetLowerLimit(protocol.PacketNumber)

	GetAlarmTimeout() time.Time
	GetAckFrame() *wire.AckFrame

	GetClosePathFrame() *wire.ClosePathFrame

	GetStatistics() (uint64, uint64)
}