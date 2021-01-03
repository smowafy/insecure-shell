package main

import (
	"log"
)

const (
	Data byte = iota
	WindowResize
)

// arbitrary for now
const PacketMaxSize int = 32

type Packet struct {
	Type byte
	// number of bytes in the payload
	Size    byte
	Payload []byte
}

type PacketHandler func(ConnectionHandler, Packet)

var HandlerTable = []PacketHandler{DataPacketHandler, WindowResizePacketHandler}

func DataPacketHandler(handler ConnectionHandler, packet Packet) {
	log.Printf("[Data Packet Handler] received payload %v\n", packet.Payload)

	err := handler.WriteToPtmx(packet.Payload)

	if err != nil {
		log.Printf("[Data Packet Handler] exited with error %s\n", err)
		return
	}
}

func WindowResizePacketHandler(handler ConnectionHandler, packet Packet) {
	log.Printf("[Window Resize Packet Handler] received payload %v\n", packet.Payload)

	// 16 * 4 = 64 bits
	if packet.Size != 8 {
		log.Printf("[Window Resize Packet Handler] invalid size %v; dropping\n", packet.Size)
		return
	}

	var rows, cols, x, y uint16

	parsedAttrs := []*uint16{&rows, &cols, &x, &y}

	log.Printf("[Window Resize Packet Handler] parsed: %v\n", parsedAttrs)

	i := 0

	for _, ptr := range parsedAttrs {
		// big endian
		*ptr = uint16((uint16(packet.Payload[(i<<1)]) << 8) + uint16(packet.Payload[(i<<1)+1]))

		log.Printf("[Window Resize Packet Handler] i = %v, ptr = %v\n", i, *ptr)

		i++
	}

	log.Printf("[Window Resize Packet Handler] size received %v, %v, %v, %v\n", rows, cols, x, y)

	err := handler.SetPtmxSize(rows, cols, x, y)

	if err != nil {
		log.Printf("[Window Resize Packet Handler] exited with error %s\n", err)
	}
}
