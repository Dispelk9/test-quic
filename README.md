Using this code folder with mp-quic latency of vuva.

Change the quic-go folder in mpquic-latency/mp-quicgo/src/github/lucas-clemente/quic-go



Using on local machine:

run traffic-gen.go

for server:

	go run traffic-gen.go -mode server -p quic -a localhost:3030 -v

for client:
	
	go run traffic-gen.go -mode client -p quic -a localhost:3030 -cc vegas 
	
-cc : 
	
	cubic, vegas or olia.
	
See the output of client terminal
