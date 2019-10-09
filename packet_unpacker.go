package quic

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/wire"
	"github.com/lucas-clemente/quic-go/qerr"
)

type unpackedPacket struct {
	encryptionLevel protocol.EncryptionLevel
	frames          []wire.Frame
}

type quicAEAD interface {
	Open(dst, src []byte, packetNumber protocol.PacketNumber, associatedData []byte) ([]byte, protocol.EncryptionLevel, error)
}

type packetUnpacker struct {
	version protocol.VersionNumber
	aead    quicAEAD
}

func (u *packetUnpacker) Unpack(publicHeaderBinary []byte, hdr *wire.PublicHeader, data []byte) (*unpackedPacket, error) {
	buf := getPacketBuffer()
	defer putPacketBuffer(buf)
	decrypted, encryptionLevel, err := u.aead.Open(buf, data, hdr.PacketNumber, publicHeaderBinary)
	if err != nil {
		// Wrap err in quicError so that public reset is sent by session
		return nil, qerr.Error(qerr.DecryptionFailure, err.Error())
	}
	r := bytes.NewReader(decrypted)

	if r.Len() == 0 {
		return nil, qerr.MissingPayload
	}

	fs := make([]wire.Frame, 0, 2)

	// Read all frames in the packet
	for r.Len() > 0 {
		typeByte, _ := r.ReadByte()
		if typeByte == 0x0 { // PADDING frame
			continue
		}
		r.UnreadByte()

		var frame wire.Frame
		if typeByte&0x80 == 0x80 {
			frame, err = wire.ParseStreamFrame(r, u.version)
			if err != nil {
				err = qerr.Error(qerr.InvalidStreamData, err.Error())
			} else {
				streamID := frame.(*wire.StreamFrame).StreamID
				if streamID != 1 && encryptionLevel <= protocol.EncryptionUnencrypted {
					err = qerr.Error(qerr.UnencryptedStreamData, fmt.Sprintf("received unencrypted stream data on stream %d", streamID))
				}
			}
			// VUVA: log received frame
			streamFrame := frame.(*wire.StreamFrame)
			logfile, err := os.OpenFile("receiver-frame.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

			if err != nil {
				panic("cannot create logfile!!")
			}

			defer logfile.Close()
			if streamFrame.DataLen() > 8 {
				io.WriteString(logfile, fmt.Sprintf("%d %d %d %d %d %d\n", hdr.PathID, hdr.PacketNumber, streamFrame.StreamID, streamFrame.Offset, uint(binary.BigEndian.Uint32(streamFrame.Data[0:4])), uint(time.Now().UnixNano())))

			}
			// END VUVA
		} else if typeByte&0xc0 == 0x40 {
			frame, err = wire.ParseAckFrame(r, u.version)
			if err != nil {
				err = qerr.Error(qerr.InvalidAckData, err.Error())
			}
		} else if typeByte&0xe0 == 0x20 {
			err = errors.New("unimplemented: CONGESTION_FEEDBACK")
		} else {
			switch typeByte {
			case 0x01:
				frame, err = wire.ParseRstStreamFrame(r, u.version)
				if err != nil {
					err = qerr.Error(qerr.InvalidRstStreamData, err.Error())
				}
			case 0x02:
				frame, err = wire.ParseConnectionCloseFrame(r, u.version)
				if err != nil {
					err = qerr.Error(qerr.InvalidConnectionCloseData, err.Error())
				}
			case 0x03:
				frame, err = wire.ParseGoawayFrame(r, u.version)
				if err != nil {
					err = qerr.Error(qerr.InvalidGoawayData, err.Error())
				}
			case 0x04:
				frame, err = wire.ParseWindowUpdateFrame(r, u.version)
				if err != nil {
					err = qerr.Error(qerr.InvalidWindowUpdateData, err.Error())
				}
			case 0x05:
				frame, err = wire.ParseBlockedFrame(r, u.version)
				if err != nil {
					err = qerr.Error(qerr.InvalidBlockedData, err.Error())
				}
			case 0x06:
				frame, err = wire.ParseStopWaitingFrame(r, hdr.PacketNumber, hdr.PacketNumberLen, u.version)
				if err != nil {
					err = qerr.Error(qerr.InvalidStopWaitingData, err.Error())
				}
			case 0x07:
				frame, err = wire.ParsePingFrame(r, u.version)
			case 0x10:
				frame, err = wire.ParseAddAddressFrame(r, u.version)
			case 0x11:
				frame, err = wire.ParseClosePathFrame(r, u.version)
			case 0x12:
				frame, err = wire.ParsePathsFrame(r, u.version)
			default:
				err = qerr.Error(qerr.InvalidFrameData, fmt.Sprintf("unknown type byte 0x%x", typeByte))
			}
		}
		if err != nil {
			return nil, err
		}
		if frame != nil {
			fs = append(fs, frame)
		}
	}

	return &unpackedPacket{
		encryptionLevel: encryptionLevel,
		frames:          fs,
	}, nil
}