PB = $(wildcard *.proto)
GO = $(PB:.proto=.pb.go)

all: $(GO)

%.pb.go: %.proto
		protoc --proto_path=$(PWD) --go_out=. $<
		mv ./pb/*.pb.go .
		rm -f ./pb

clean:
		rm -f ./pb/*.pb.go
		rm -f ./pb
		rm *.pb.go
