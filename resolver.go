package main

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/wpalmer/gozone"
	"net"
	"os"
	"reflect"
	"strings"
)

var Hosts = map[string][]string{}

type QueueMessage struct {
	conn *net.UDPConn
	addr *net.UDPAddr
	data []byte
}

//  DNSResourceRecord
//  0  1  2  3  4  5  6  7  8  9  0  1  2  3  4  5
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                                               |
//  /                                               /
//  /                      NAME                     /
//  |                                               |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                      TYPE                     |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                     CLASS                     |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                      TTL                      |
//  |                                               |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                   RDLENGTH                    |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--|
//  /                     RDATA                     /
//  /                                               /
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+

func getDnsAnswer(ip string, question string) layers.DNSResourceRecord {
	return layers.DNSResourceRecord{
		Name:  []byte(question),
		Type:  layers.DNSTypeA,
		Class: layers.DNSClassIN,
		TTL:   DefaultTTL,
		IP:    net.ParseIP(ip),
	}
}

// DNS is specified in RFC 1034 / RFC 1035
// +---------------------+
// |        Header       |
// +---------------------+
// |       Question      | the question for the name server
// +---------------------+
// |        Answer       | RRs answering the question
// +---------------------+
// |      Authority      | RRs pointing toward an authority
// +---------------------+
// |      Additional     | RRs holding additional information
// +---------------------+
//
//  DNS Header
//  0  1  2  3  4  5  6  7  8  9  0  1  2  3  4  5
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                      ID                       |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |QR|   Opcode  |AA|TC|RD|RA|   Z    |   RCODE   |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                    QDCOUNT                    |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                    ANCOUNT                    |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                    NSCOUNT                    |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//  |                    ARCOUNT                    |
//  +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+

func getDnsResultPacket(question layers.DNSQuestion, answerIps []string, id uint16) layers.DNS {
	var answers []layers.DNSResourceRecord
	for _, ip := range answerIps {
		answers = append(answers, getDnsAnswer(ip, string(question.Name)))
	}

	var responseCode layers.DNSResponseCode
	if len(answers) != 0 {
		responseCode = layers.DNSResponseCodeNoErr
	} else {
		responseCode = layers.DNSResponseCodeNXDomain
	}

	dnsResult := layers.DNS{
		ID:           id,
		ResponseCode: responseCode,

		QR:     true,
		OpCode: 0,
		AA:     false,
		TC:     false,
		RD:     false,
		RA:     false,
		Z:      0,

		QDCount: 1,
		ANCount: uint16(len(answers)),
		NSCount: 1,
		ARCount: 0,

		Questions: []layers.DNSQuestion{question},
		Answers:   answers,
	}
	return dnsResult
}

func sendUDPPacket(conn *net.UDPConn, addr *net.UDPAddr, packet layers.DNS) {
	buf := gopacket.NewSerializeBuffer()
	packet.SerializeTo(buf, gopacket.SerializeOptions{})
	conn.WriteToUDP(buf.Bytes(), addr)
}

func resolve(workerId int, conn *net.UDPConn, addr *net.UDPAddr, data []byte) {
	packet := gopacket.NewPacket(data, layers.LayerTypeDNS, gopacket.Default)
	if dnsLayer := packet.Layer(layers.LayerTypeDNS); dnsLayer != nil {
		dns, _ := dnsLayer.(*layers.DNS)
		question := dns.Questions[0]
		if question.Type == layers.DNSTypeA {
			hostname := string(question.Name)
			resultIps, _ := Hosts[getDnsKey(question.Type.String(), hostname)]
			dnsResultPacket := getDnsResultPacket(question, resultIps, dns.ID)
			fmt.Println("DNS Resolved", workerId, string(question.Name), resultIps)
			sendUDPPacket(conn, addr, dnsResultPacket)
		}
	}
}

func getDnsKey(zoneType string, domain string) string {
	return zoneType + "::" + strings.TrimSuffix(domain, ".")
}

func loadZoneFile(filePath string) {
	stream, _ := os.Open(filePath)
	var record gozone.Record
	scanner := gozone.NewScanner(stream)

	for {
		err := scanner.Next(&record)
		if err != nil {
			break
		}

		if record.Type == gozone.RecordType_A {
			dnsKey := getDnsKey(record.Type.String(), record.DomainName)
			Hosts[dnsKey] = append(Hosts[dnsKey], record.Data[0])
		}
	}

}

func startResolverWorker(workerId int, queue chan QueueMessage) {
	fmt.Println("Worker started with id", workerId)
	for {
		message := <-queue
		resolve(workerId, message.conn, message.addr, message.data)
	}
}

func sendToAny(ob QueueMessage, chs []chan QueueMessage) int {
	var set []reflect.SelectCase
	for _, ch := range chs {
		set = append(set, reflect.SelectCase{
			Dir:  reflect.SelectSend,
			Chan: reflect.ValueOf(ch),
			Send: reflect.ValueOf(ob),
		})
	}
	to, _, _ := reflect.Select(set)
	return to
}

func StartServer(zoneFilePath string, workerCount int, queueSize int) {
	loadZoneFile(zoneFilePath)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: 5353,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Printf("server listening %s\n", conn.LocalAddr().String())

	fmt.Println(workerCount)
	// Start workers
	var workerQueues []chan QueueMessage
	for i := 0; i < workerCount; i++ {
		queue := make(chan QueueMessage, queueSize)
		workerQueues = append(workerQueues, queue)
		go startResolverWorker(i, queue)
	}

	// Start Serving
	for {
		message := make([]byte, MaxMessageSize)
		rlen, from, err := conn.ReadFromUDP(message[:])
		if err != nil {
			panic(err)
		}

		queueMessage := QueueMessage{conn: conn, addr: from, data: message[:rlen]}
		sendToAny(queueMessage, workerQueues)
	}
}
