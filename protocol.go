package main

import (
	"fmt"
	"time"

	"go.bug.st/serial"
)

const (
	TileCPU  byte = 0x53
	TileGPU  byte = 0x36
	TileMEM  byte = 0x49
	TileDSK  byte = 0x4F
	TileDATE byte = 0x6B
	TileNET  byte = 0x27
	TileVOL  byte = 0x10
	TileBAT  byte = 0x1A
)

// seqFor: SEQ fisso per ciascun tile in steady-state (durante unlock si usa il SEQ ricevuto).
var seqFor = map[byte]byte{
	TileCPU:  '2',
	TileGPU:  '3',
	TileMEM:  '4',
	TileDSK:  '5',
	TileDATE: '6',
	TileNET:  '7',
	TileVOL:  '9',
	TileBAT:  '2',
}

var trailer = []byte{0xCC, 0x33, 0xC3, 0x3C}

func openSerial(cfg *Config) (serial.Port, error) {
	time.Sleep(time.Duration(float64(time.Second) * cfg.Serial.StartDelay))

	mode := &serial.Mode{
		BaudRate: cfg.Serial.Baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
		InitialStatusBits: &serial.ModemOutputBits{
			DTR: cfg.Serial.DSRDTR,
			RTS: cfg.Serial.RTSCTS,
		},
	}

	port, err := serial.Open(cfg.Serial.Port, mode)
	if err != nil {
		return nil, fmt.Errorf("serial.Open(%s): %w", cfg.Serial.Port, err)
	}

	if err := port.SetReadTimeout(time.Second); err != nil {
		port.Close()
		return nil, fmt.Errorf("SetReadTimeout: %w", err)
	}

	port.ResetInputBuffer()
	port.ResetOutputBuffer()
	return port, nil
}

func readByte(port serial.Port) (byte, bool) {
	buf := [1]byte{}
	n, err := port.Read(buf[:])
	if err != nil || n == 0 {
		return 0, false
	}
	return buf[0], true
}

// readENQ legge un frame ENQ: AA 05 <seq> CC 33 C3 3C
func readENQ(port serial.Port) (byte, bool) {
	b, ok := readByte(port)
	if !ok || b != 0xAA {
		return 0, false
	}
	b, ok = readByte(port)
	if !ok || b != 0x05 {
		return 0, false
	}
	seq, ok := readByte(port)
	if !ok {
		return 0, false
	}
	for _, expected := range trailer {
		b, ok = readByte(port)
		if !ok || b != expected {
			return 0, false
		}
	}
	return seq, true
}

// buildReply costruisce il frame: AA <tileID> 00 <seq> <payload> CC 33 C3 3C
func buildReply(tileID, seq byte, payload string) []byte {
	frame := []byte{0xAA, tileID, 0x00, seq}
	frame = append(frame, []byte(payload)...)
	frame = append(frame, trailer...)
	return frame
}

func sendTile(port serial.Port, tileID byte, payload string, seq byte) error {
	frame := buildReply(tileID, seq, payload)
	if _, err := port.Write(frame); err != nil {
		return err
	}
	port.Drain()
	time.Sleep(6 * time.Millisecond)
	return nil
}

// isBootSeq: il display invia SEQ ASCII 0-9 o '<' durante la sequenza di boot.
func isBootSeq(b byte) bool {
	return (b >= 0x30 && b <= 0x39) || b == 0x3C
}
