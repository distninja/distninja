package rpc

import (
	"fmt"
	"io"
	"math"
	"net"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/grpc"

	pb "github.com/distninja/distninja/rpc/proto"
)

type server struct {
	pb.UnimplementedServerProtoServer
}

func StartServer(serve string) error {
	port := serve
	if !strings.Contains(port, ":") {
		port = ":" + port
	}

	options := []grpc.ServerOption{grpc.MaxRecvMsgSize(math.MaxInt32), grpc.MaxSendMsgSize(math.MaxInt32)}

	s := grpc.NewServer(options...)
	pb.RegisterServerProtoServer(s, &server{})

	lis, err := net.Listen("tcp", port)
	if err != nil {
		return errors.Wrap(err, "failed to listen on grpc port")
	}

	fmt.Printf("Starting gRPC server on %s...\n", port)

	return s.Serve(lis)
}

func (s *server) SendServer(stream pb.ServerProto_SendServerServer) error {
	for {
		_, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	_ = stream.SendAndClose(&pb.ServerReply{Message: "Received with success"})

	return nil
}
