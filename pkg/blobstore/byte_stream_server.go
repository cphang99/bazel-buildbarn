package blobstore

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"golang.org/x/net/context"

	"google.golang.org/genproto/googleapis/bytestream"
	remoteexecution "google.golang.org/genproto/googleapis/devtools/remoteexecution/v1test"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	readChunkSize = 1 << 16
)

// parseResourceNameRead parses resource name strings in one of the following two forms:
//
// - blobs/${hash}/${size}
// - ${instance}/blobs/${hash}/${size}
//
// In the process, the hash, size and instance are extracted.
func parseResourceNameRead(resourceName string) (string, *remoteexecution.Digest) {
	fields := strings.FieldsFunc(resourceName, func(r rune) bool { return r == '/' })
	l := len(fields)
	if (l != 3 && l != 4) || fields[l-3] != "blobs" {
		return "", nil
	}
	size, err := strconv.ParseInt(fields[l-1], 10, 64)
	if err != nil {
		return "", nil
	}
	instance := ""
	if l == 4 {
		instance = fields[0]
	}
	return instance, &remoteexecution.Digest{
		Hash:      fields[l-2],
		SizeBytes: size,
	}
}

// parseResourceNameWrite parses resource name strings in one of the following two forms:
//
// - uploads/${uuid}/blobs/${hash}/${size}
// - ${instance}/uploads/${uuid}/blobs/${hash}/${size}
//
// In the process, the hash, size and instance are extracted.
func parseResourceNameWrite(resourceName string) (string, *remoteexecution.Digest) {
	fields := strings.FieldsFunc(resourceName, func(r rune) bool { return r == '/' })
	l := len(fields)
	if (l != 5 && l != 6) || fields[l-5] != "uploads" || fields[l-3] != "blobs" {
		return "", nil
	}
	size, err := strconv.ParseInt(fields[l-1], 10, 64)
	if err != nil {
		return "", nil
	}
	instance := ""
	if l == 6 {
		instance = fields[0]
	}
	return instance, &remoteexecution.Digest{
		Hash:      fields[l-2],
		SizeBytes: size,
	}
}

type byteStreamServer struct {
	blobAccess BlobAccess
}

func NewByteStreamServer(blobAccess BlobAccess) bytestream.ByteStreamServer {
	return &byteStreamServer{
		blobAccess: blobAccess,
	}
}

func (s *byteStreamServer) Read(in *bytestream.ReadRequest, out bytestream.ByteStream_ReadServer) error {
	if in.ReadOffset != 0 || in.ReadLimit != 0 {
		return status.Error(codes.Unimplemented, "This service does not support downloading directory trees")
	}

	instance, digest := parseResourceNameRead(in.ResourceName)
	if digest == nil {
		return errors.New("Unsupported resource naming scheme")
	}
	r := s.blobAccess.Get(out.Context(), instance, digest)
	defer r.Close()

	for {
		var readBuf [readChunkSize]byte
		n, err := r.Read(readBuf[:])
		if err != nil && err != io.EOF {
			return err
		}
		if n > 0 {
			if err := out.Send(&bytestream.ReadResponse{Data: readBuf[:n]}); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

type byteStreamWriteServerReader struct {
	stream      bytestream.ByteStream_WriteServer
	writeOffset int64
	data        []byte
}

func (r *byteStreamWriteServerReader) Read(p []byte) (int, error) {
	n := 0
	for {
		// Copy data from previously read partial chunk.
		c := copy(p, r.data)
		p = p[c:]
		r.data = r.data[c:]
		n += c
		if len(p) == 0 {
			return n, nil
		}

		// Read next chunk.
		request, err := r.stream.Recv()
		if err != nil {
			return n, err
		}
		if request.WriteOffset != r.writeOffset {
			return n, fmt.Errorf("Attempted to write at offset %d, while %d was expected", request.WriteOffset, r.writeOffset)
		}
		r.writeOffset += int64(len(request.Data))
		r.data = request.Data
	}
}

func (s *byteStreamServer) Write(stream bytestream.ByteStream_WriteServer) error {
	request, err := stream.Recv()
	if err != nil {
		return err
	}
	instance, digest := parseResourceNameWrite(request.ResourceName)
	if digest == nil {
		return errors.New("Unsupported resource naming scheme")
	}
	err = s.blobAccess.Put(stream.Context(), instance, digest, &byteStreamWriteServerReader{
		stream:      stream,
		writeOffset: int64(len(request.Data)),
		data:        request.Data,
	})
	return err
}

func (s *byteStreamServer) QueryWriteStatus(ctx context.Context, in *bytestream.QueryWriteStatusRequest) (*bytestream.QueryWriteStatusResponse, error) {
	log.Print("Attempted to call ByteStream.QueryWriteStatus")
	return nil, errors.New("Fail!")
}
