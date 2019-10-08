Using on local machine:

run traffic-gen.go

for server:

	go run traffic-gen.go -mode server -p quic -a localhost:3030 -v

for client:
	
	go run traffic-gen.go -mode client -p quic -a localhost:3030 -cc vegas -v
	
-cc : 
	
	cubic, vegas or olia.
