package mkv

import (
	"context"
	"fmt"
	pb "github.com/anujga/dstk/pkg/api/proto"
	"github.com/anujga/dstk/pkg/core"
	"github.com/golang/protobuf/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"
)

type MapEntry struct {
	payload     []byte
	partitionId int64
}

type mkvServer struct {
	data       map[string]MapEntry
	partitions map[int64]string
	mu         sync.Mutex

	log  *zap.Logger
	slog *zap.SugaredLogger
}

func MakeServer() (pb.MkvServer, error) {
	s := mkvServer{
		data:       make(map[string]MapEntry),
		partitions: make(map[int64]string),
	}
	var err error
	if s.log, err = zap.NewProduction(); err != nil {
		return nil, err
	}

	s.slog = s.log.Sugar()
	return &s, nil
}

func (s *mkvServer) AddPart(ctx context.Context, args *pb.AddParReq) (*pb.Ex, error) {
	uri := args.GetUri()
	if !strings.HasPrefix(uri, "file://") {
		s.slog.Errorw("Unkown file prefix",
			"allowed", "file://",
			"found", uri,
		)
		return &pb.Ex{Id: pb.Ex_NOT_IMPLEMENTED}, nil
	}

	filename := strings.TrimPrefix(uri, "file://")
	s.slog.Infow("adding partition",
		// Structured context as loosely typed key-value pairs.
		"filename", filename,
	)

	fin, err := ioutil.ReadFile(filename)
	if err != nil {
		s.slog.Errorw("Could not open patition file",
			"uri", uri,
			"err", err)
		return &pb.Ex{Id: pb.Ex_INVALID_ARGUMENT}, err
	}

	p := &pb.MkvPartition{}
	if err := proto.Unmarshal(fin, p); err != nil {
		s.slog.Errorw("Failed to parse partition file:",
			"uri", uri,
			"err", err)

		return &pb.Ex{Id: pb.Ex_INVALID_ARGUMENT}, err
	}

	id := p.GetId()
	s.mu.Lock()

	{
		if _, ok := s.partitions[id]; ok {
			s.slog.Errorw("Partition already exists")
			return &pb.Ex{Id: pb.Ex_INVALID_ARGUMENT, Msg: "Partition already exists"}, err
		}

		//note: partition is already added assuming there cannot be any failure
		//subsequently
		s.partitions[id] = uri
	}

	s.mu.Unlock()

	i := 0
	var e *pb.MkvPartition_Entry
	for i, e = range p.Entries {
		s.mu.Lock()
		s.data[string(e.Key)] = MapEntry{
			partitionId: p.GetId(),
			payload:     e.Value,
		}
		s.mu.Unlock()
	}

	s.slog.Infow("file read successfully", "uri", uri, "count", i+1)
	return core.ExOK, nil
}

func (s *mkvServer) Get(ctx context.Context, args *pb.GetReq) (*pb.GetRes, error) {
	k := string(args.GetKey())
	p := args.GetPartitionId()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.partitions[p]; !ok {
		return &pb.GetRes{Ex: &pb.Ex{Id: pb.Ex_BAD_PARTITION}}, nil
	}

	val, ok := s.data[k]
	if !ok {
		return &pb.GetRes{Ex: &pb.Ex{Id: pb.Ex_NOT_FOUND}}, nil
	}

	return &pb.GetRes{
		Ex:          core.ExOK,
		PartitionId: val.partitionId,
		Payload:     val.payload,
	}, nil
}

func StartServer(port int32, listener *bufconn.Listener) (*grpc.Server, func()) {
	log, err := zap.NewProduction()
	if err != nil {
		println("Failed to open logger %s", err)
		os.Exit(-1)
	}
	slog := log.Sugar()

	var lis net.Listener
	if port == 0 {
		lis = listener
	} else {
		lis, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			slog.Fatalw("failed to listen",
				"port", port,
				"err", err)
		}
	}
	grpcServer := grpc.NewServer()
	s, err := MakeServer()
	if err != nil {
		slog.Fatalw("failed to initialize server object",
			"port", port,
			"err", err)
	}

	pb.RegisterMkvServer(grpcServer, s)

	return grpcServer, func() {
		if err = grpcServer.Serve(lis); err != nil {
			slog.Fatalw("failed to start server",
				"port", port,
				"err", err)
			//todo: dont crash the process, return a promise or channel
			//or create a small interface for shutdown, shutdownNow, didStart ...
			os.Exit(-2)
		}
	}
}
